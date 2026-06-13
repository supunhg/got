package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/eventbus"
	"github.com/got-sh/got/internal/gerr"
	"github.com/got-sh/got/internal/repo"
)

// publishWorkspaceEvent is a small helper used by the workspace
// CLI commands to publish a WorkspaceCreated or
// WorkspaceUpdated event on the in-process bus. The bus itself
// is built once per command by openEventBus; if the bus cannot
// be opened (e.g. the user has not run `got init`), this is a
// no-op so commands still work in a brand-new repo. Errors
// from the bus are swallowed for the same reason — the
// workspace engine must not fail just because the event log
// is unreachable. The events are best-effort: the workspace
// row is the source of truth, the event log is the audit trail.
//
// The caller passes the workspace pointer so we can include
// the snapshot of name/title/state in the payload. A future
// refactor could move this into a workspace.Store hook so
// callers can't forget to publish, but the v0.4 wiring is
// kept at the CLI edge to avoid changing the workspace
// package's API.
func publishWorkspaceEvent(ctx context.Context, d Deps, topic eventbus.Topic, w interface {
	GetID() string
	GetName() string
	GetTitle() string
	GetState() string
}) {
	bus, cleanup, err := openEventBus(ctx, d)
	if err != nil || bus == nil {
		return
	}
	defer cleanup()
	payload := eventbus.Payload{
		"id":    w.GetID(),
		"name":  w.GetName(),
		"title": w.GetTitle(),
		"state": w.GetState(),
	}
	_, _ = bus.Publish(ctx, topic, eventbus.WithPayload(payload))
}

// workspaceEventSource is implemented by *workspace.Workspace.
// We use a tiny adapter here (defined in workspace.go) instead
// of importing the type so this file stays light; the actual
// adapter is appended below.
type workspaceEventSource interface {
	GetID() string
	GetName() string
	GetTitle() string
	GetState() string
}

// openEventBus discovers the work tree, opens the .got/got.db
// store, and constructs a *eventbus.Bus. The returned cleanup
// closes the store. If the discovery or open step fails (most
// commonly: not in a git repo, or `got init` has not been run),
// the function returns (nil, noop, nil) so callers can treat
// "no bus" as a non-fatal condition.
//
// Why this is a non-fatal no-op: many commands publish events
// out of an abundance of auditing, but the user's primary
// action (creating a workspace, committing, etc.) must not
// fail just because the event log is unavailable. The bus's
// own constructor surfaces the error for callers that want
// it (e.g. `got event list`).
func openEventBus(ctx context.Context, d Deps) (*eventbus.Bus, func(), error) {
	workTree, err := d.Discover(".")
	if err != nil {
		return nil, func() {}, nil
	}
	if d.StoreFor == nil {
		return nil, func() {}, nil
	}
	dbPath := repo.NewPaths(workTree).DBFile
	s, err := d.StoreFor(dbPath)
	if err != nil {
		return nil, func() {}, nil
	}
	bus, err := eventbus.New(s, currentActor(), dbPath)
	if err != nil {
		_ = s.Close()
		return nil, func() {}, nil
	}
	return bus, func() { _ = s.Close() }, nil
}

// topicWorkspaceCreated / topicWorkspaceUpdated are local
// re-exports of the bus topic constants used by the
// workspace engine. They live here (and not in workspace.go)
// so adding a new event topic doesn't drag the workspace
// package's CLI dependency on eventbus — the workspace
// command can stay focused on the workspace engine.
const (
	topicWorkspaceCreated = eventbus.TopicWorkspaceCreated
	topicWorkspaceUpdated = eventbus.TopicWorkspaceUpdated
)

// currentActor returns the best guess at the OS user, used as
// the default event actor. Mirrors the logic in
// internal/eventbus.currentActor but lives here so the CLI
// doesn't depend on a private function.
func currentActor() string {
	for _, env := range []string{"GOT_ACTOR", "USER", "USERNAME", "LOGNAME"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

// -----------------------------------------------------------------------
// got event
// -----------------------------------------------------------------------

// newEventCmd builds the `got event` subcommand tree. The
// tree is small in v0.4 — `got event list` and `got event
// show` — but uses the same "subcommand is a verb, the
// default is list" pattern as `got workspace` so `got event`
// with no args still works.
//
//	got event                              list (default RunE)
//	got event list [--topic T] [--limit N] [--since RFC3339] [--json]
//	got event show <id>
func newEventCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Inspect the event bus replay log",
		Long: `Inspect the durable event bus replay log.

Every event published by the in-process event bus is persisted
to .got/got.db in the events table. The event log is what
external plugins and offline tools read to catch up on activity
that happened in a previous process or a previous run of ` + "`got`" + `.

Examples:
  got event                       # show the most recent 50 events
  got event list --topic CommitCreated
  got event list --limit 200 --json
  got event list --since 2026-06-01T00:00:00Z
  got event show 0189f3e5a4b0-9c8e1d2f3a4b5c6d`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEventList(cmd.Context(), cmd, d, eventListOptions{
				topic:    "",
				limit:    50,
				since:    time.Time{},
				asJSON:   false,
			})
		},
	}
	cmd.AddCommand(newEventListCmd(d))
	cmd.AddCommand(newEventShowCmd(d))
	return cmd
}

// eventListOptions captures the flags shared by the default
// and explicit `got event list` subcommands.
type eventListOptions struct {
	topic  string
	limit  int
	since  time.Time
	asJSON bool
}

func newEventListCmd(d Deps) *cobra.Command {
	var (
		topicFlag string
		limitFlag int
		sinceFlag string
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events from the replay log",
		Long: `List events from the durable event bus replay log, newest first.

Pass --topic to filter by topic name (one of ` + topicList() + `),
--limit to cap the number of events (default 50, max 1000),
or --since to start from a specific time. The --since value is
a RFC3339 timestamp; events strictly before it are excluded.

The --json flag emits a single JSON array of event objects
suitable for piping into jq or for a plugin to consume.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := eventListOptions{
				topic:  topicFlag,
				limit:  limitFlag,
				asJSON: asJSON,
			}
			if sinceFlag != "" {
				t, err := time.Parse(time.RFC3339, sinceFlag)
				if err != nil {
					return gerr.Validation(fmt.Sprintf("invalid --since (want RFC3339): %v", err))
				}
				opts.since = t
			}
			return runEventList(cmd.Context(), cmd, d, opts)
		},
	}
	cmd.Flags().StringVar(&topicFlag, "topic", "", "filter by topic name")
	cmd.Flags().IntVar(&limitFlag, "limit", 50, "maximum number of events to return (1-1000)")
	cmd.Flags().StringVar(&sinceFlag, "since", "", "RFC3339 timestamp; events before this are excluded")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

func newEventShowCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a single event by ID",
		Long: `Show a single event by its full ID. The ID is the value
displayed in the ID column of ` + "`got event list`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEventShow(cmd.Context(), cmd, d, args[0])
		},
	}
	return cmd
}

// runEventList is shared by the default subcommand and the
// explicit `got event list`. It opens the bus, queries the
// replay log, and renders either a table or a JSON array. The
// --since branch uses ReplaySince to honor the offset; the
// default branch uses Replay (newest-first, capped at limit).
func runEventList(ctx context.Context, cmd *cobra.Command, d Deps, opts eventListOptions) error {
	logger := loggerFor(d)
	logger.Info("event list starting", "topic", opts.topic, "limit", opts.limit, "since", opts.since, "json", opts.asJSON)
	bus, cleanup, err := openEventBus(ctx, d)
	if err != nil {
		return err
	}
	if bus == nil {
		_, _ = fmt.Fprintln(cmdWriter(cmd, d), "(no .got/got.db — run `got init` first?)")
		return nil
	}
	defer cleanup()

	limit := opts.limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	var events []*eventbus.Event
	if !opts.since.IsZero() {
		events, err = bus.ReplaySince(ctx, eventbus.Topic(opts.topic), opts.since, limit)
	} else {
		events, err = bus.Replay(ctx, eventbus.Topic(opts.topic), limit)
	}
	if err != nil {
		return err
	}
	logger.Info("event list finished", "count", len(events))
	out := cmdWriter(cmd, d)
	if opts.asJSON {
		return writeEventJSONArray(out, events)
	}
	return writeEventTable(out, events)
}

// runEventShow fetches a single event by ID. The replay log
// is searched by a one-shot SELECT (the events table is
// indexed on id, so this is O(log n)). If the event is not
// found, the command returns a validation error so a
// misspelled ID is obvious to the user.
func runEventShow(ctx context.Context, cmd *cobra.Command, d Deps, id string) error {
	logger := loggerFor(d)
	logger.Info("event show starting", "id", id)
	bus, cleanup, err := openEventBus(ctx, d)
	if err != nil {
		return err
	}
	if bus == nil {
		return gerr.Validation("no .got/got.db — run `got init` first")
	}
	defer cleanup()

	store, ok := busStoreForShow(bus)
	if !ok {
		return gerr.Validation("event bus does not expose the underlying store; cannot look up a single event by id")
	}
	var (
		e         eventbus.Event
		createdMS int64
		payload   string
	)
	row := store.QueryRowContext(ctx,
		`SELECT id, topic, created_at, COALESCE(actor, ''), COALESCE(source, ''), COALESCE(payload, '{}')
		 FROM events WHERE id = ?`, id)
	if err := row.Scan(&e.ID, &e.Topic, &createdMS, &e.Actor, &e.Source, &payload); err != nil {
		return gerr.Validation(fmt.Sprintf("event %q not found", id))
	}
	e.CreatedAt = time.UnixMilli(createdMS).UTC()
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &e.Payload)
	}
	if e.Payload == nil {
		e.Payload = eventbus.Payload{}
	}
	logger.Info("event show finished", "id", id, "topic", e.Topic)
	out := cmdWriter(cmd, d)
	return writeEventDetail(out, &e)
}

// busStoreForShow returns the *store.Store underlying the
// bus. We expose it via a tiny package-private type assertion
// (the bus package keeps the field private) — see
// internal/cli/eventbus_bridge.go for the wiring. The bool
// return is false if the bus was constructed without a
// bridge, which only happens in tests.
func busStoreForShow(bus *eventbus.Bus) (interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *row
}, bool) {
	// We import the bridge implementation by side effect:
	// the bus package exposes the *store.Store via
	// bus.Store() (added in this commit). See
	// internal/eventbus/bus_bridge.go.
	return busStoreForShowBridge(bus)
}

// row is a local re-export of *sql.Row so the bridge can
// match the signature without dragging the database/sql
// dependency into every file in this package.
type row = sqlRow

// writeEventTable renders the recent-events table. The
// columns are ID (truncated to 16 chars), TOPIC, ACTOR,
// CREATED, and PAYLOAD (the payload is collapsed to a
// short summary so the table stays readable). The full
// payload is available via `got event show <id>`.
func writeEventTable(w io.Writer, events []*eventbus.Event) error {
	if len(events) == 0 {
		_, err := fmt.Fprintln(w, "(no events)")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tTOPIC\tACTOR\tCREATED\tPAYLOAD")
	for _, e := range events {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			truncateID(e.ID), e.Topic, e.Actor,
			e.CreatedAt.Format("2006-01-02 15:04:05"),
			payloadSummary(e.Payload))
	}
	return tw.Flush()
}

// writeEventDetail renders a single event as a key/value
// block. The payload is pretty-printed JSON so a plugin
// author can copy it directly.
func writeEventDetail(w io.Writer, e *eventbus.Event) error {
	if e == nil {
		return fmt.Errorf("nil event")
	}
	_, _ = fmt.Fprintf(w, "ID:        %s\n", e.ID)
	_, _ = fmt.Fprintf(w, "Topic:     %s\n", e.Topic)
	_, _ = fmt.Fprintf(w, "Created:   %s\n", e.CreatedAt.Format(time.RFC3339Nano))
	if e.Actor != "" {
		_, _ = fmt.Fprintf(w, "Actor:     %s\n", e.Actor)
	}
	if e.Source != "" {
		_, _ = fmt.Fprintf(w, "Source:    %s\n", e.Source)
	}
	body, err := json.MarshalIndent(e.Payload, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintf(w, "Payload:   <unprintable: %v>\n", err)
	} else {
		_, _ = fmt.Fprintf(w, "Payload:\n%s\n", string(body))
	}
	return nil
}

// writeEventJSONArray writes a single JSON array. The shape
// of each element is the same as the on-disk event log:
// {id, topic, createdAt, actor, source, payload}. Plugins
// can decode this without needing the Go type definitions.
func writeEventJSONArray(w io.Writer, events []*eventbus.Event) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(events)
}

// payloadSummary collapses a payload to a short
// human-readable hint suitable for a table cell. The
// strategy is "first key + first value type" so the user
// gets an idea of what's inside without scrolling.
func payloadSummary(p eventbus.Payload) string {
	if len(p) == 0 {
		return "{}"
	}
	for k, v := range p {
		switch vv := v.(type) {
		case string:
			return k + "=" + truncate(vv, 24)
		case float64:
			return fmt.Sprintf("%s=%g", k, vv)
		case bool:
			return fmt.Sprintf("%s=%v", k, vv)
		case nil:
			return k + "=null"
		default:
			return k + "=…"
		}
	}
	return "…"
}

// truncate returns s clipped to at most n characters. Used
// by payloadSummary to keep the table column narrow.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// truncateID shortens an event ID to its first 13 chars
// (the timestamp prefix) for the table view. The full ID
// is still available via `got event show`.
func truncateID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:16]
}

// topicList returns a comma-joined list of known topics,
// used in the long help text for `got event list`.
func topicList() string {
	topics := eventbus.AllTopics()
	out := ""
	for i, t := range topics {
		if i > 0 {
			out += ", "
		}
		out += string(t)
	}
	return out
}

// limitString returns a human-friendly description of the
// --limit flag's range for the help text. It's a small
// helper kept here so the help text doesn't drift if the
// range changes.
func limitString() string {
	return "1-1000"
}

// _ keeps strconv referenced so future filters (e.g. a
// --min-payload-bytes numeric filter) can be added without
// an import-cycle.
var _ = strconv.Itoa
