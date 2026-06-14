// Copyright 2026 The GOT Authors. MIT License.
package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/supunhg/got/internal/events"
)

// GetRemotes lists all configured remotes.
func (a *ExecAdapter) GetRemotes(ctx context.Context) ([]Remote, error) {
	stdout, _, err := a.run(ctx, "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("get remotes: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	seen := make(map[string]*Remote)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: "origin\thttps://github.com/foo/bar.git (fetch)"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		url := strings.TrimSpace(parts[1])
		isPush := len(parts) >= 3 && strings.Contains(parts[len(parts)-1], "push")

		if _, ok := seen[name]; !ok {
			seen[name] = &Remote{Name: name, URL: url}
		} else if isPush {
			seen[name].PushURL = url
		}
	}

	var remotes []Remote
	for _, r := range seen {
		remotes = append(remotes, *r)
	}
	return remotes, nil
}

// AddRemote adds a new remote.
func (a *ExecAdapter) AddRemote(ctx context.Context, name, url string) error {
	_, stderr, err := a.run(ctx, "remote", "add", name, url)
	if err != nil {
		return fmt.Errorf("add remote %q: %w\n%s", name, err, stderr)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventRemoteAdded, events.RemoteAddedPayload{
			Name:    name,
			URL:     url,
			AddedAt: nowMS(),
		})
	}

	return nil
}

// RemoveRemote removes a remote.
func (a *ExecAdapter) RemoveRemote(ctx context.Context, name string) error {
	_, stderr, err := a.run(ctx, "remote", "remove", name)
	if err != nil {
		return fmt.Errorf("remove remote %q: %w\n%s", name, err, stderr)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventRemoteRemoved, events.RemoteRemovedPayload{
			Name:      name,
			RemovedAt: nowMS(),
		})
	}

	return nil
}

// Push pushes a branch to a remote.
func (a *ExecAdapter) Push(ctx context.Context, remote, branch string, force bool) (*PushResult, error) {
	args := []string{"push"}
	if force {
		args = append(args, "--force-with-lease")
	}
	args = append(args, remote, branch)

	stdout, stderr, err := a.run(ctx, args...)
	result := &PushResult{Output: stdout}
	if stderr != "" {
		if result.Output == "" {
			result.Output = stderr
		} else {
			result.Output += "\n" + stderr
		}
	}
	if err != nil {
		return result, fmt.Errorf("push: %w", err)
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventPushCompleted, events.PushCompletedPayload{
			Remote:      remote,
			Branch:      branch,
			Force:       force,
			CompletedAt: nowMS(),
		})
	}

	return result, nil
}

// Pull pulls changes from a remote branch.
func (a *ExecAdapter) Pull(ctx context.Context, remote, branch string) (*PullResult, error) {
	stdout, stderr, err := a.run(ctx, "pull", "--ff-only", remote, branch)
	result := &PullResult{
		Output:      stdout,
		FastForward: true,
	}
	if stderr != "" {
		if result.Output == "" {
			result.Output = stderr
		} else {
			result.Output += "\n" + stderr
		}
	}
	if err != nil {
		// --ff-only may fail; try regular pull.
		stdout2, stderr2, err2 := a.run(ctx, "pull", remote, branch)
		result.Output = stdout2
		if stderr2 != "" {
			if result.Output == "" {
				result.Output = stderr2
			} else {
				result.Output += "\n" + stderr2
			}
		}
		if err2 != nil {
			return result, fmt.Errorf("pull: %w", err2)
		}
		result.FastForward = false
	}

	if a.bus != nil {
		_ = a.bus.Publish(ctx, events.EventPullCompleted, events.PullCompletedPayload{
			Remote:      remote,
			Branch:      branch,
			FastForward: result.FastForward,
			CompletedAt: nowMS(),
		})
	}

	return result, nil
}

// GetGraph returns the commit graph structure for visualisation.
// Uses git log --graph format to get parent-child relationships.
func (a *ExecAdapter) GetGraph(ctx context.Context, branch string, maxCount int) ([]GraphNode, error) {
	args := []string{"log", "--format=%H|%P|%s|%D", "--max-count=" + fmt.Sprintf("%d", maxCount)}
	if branch != "" {
		args = append(args, branch)
	}

	stdout, _, err := a.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("graph: %w", err)
	}

	lines := strings.Split(stdout, "\n")
	var nodes []GraphNode

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 3 {
			continue
		}

		node := GraphNode{
			SHA:     parts[0],
			Message: parts[2],
		}

		// Parse parents.
		parentStr := strings.TrimSpace(parts[1])
		if parentStr != "" {
			node.Parents = strings.Fields(parentStr)
		}

		// Parse decorations (branches, tags).
		if len(parts) >= 4 {
			node.Refs = strings.TrimSpace(parts[3])
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}
