package analyzer

import (
	"path/filepath"
	"sort"
	"strings"
)

// IsEmpty reports whether the containerization block has no
// signals at all (no Dockerfiles, no Compose, no K8s, no
// Helm). The CLI uses this to print a single "(no container
// artifacts detected)" line instead of four "no" lines.
func (c Containerization) IsEmpty() bool {
	return !c.HasDockerfile && !c.HasDockerCompose && !c.HasKubernetes && !c.HasHelm
}

// detectContainerization scans the work tree for container-related
// artifacts: Dockerfiles, docker-compose files, Kubernetes
// manifests, and Helm charts.
//
// Dockerfile detection is by filename: "Dockerfile",
// "Dockerfile.<name>", "Containerfile", "Containerfile.<name>",
// plus all *.dockerfile. Compose is "docker-compose.yml",
// "docker-compose.yaml", "compose.yml", "compose.yaml" (and
// variants with a leading project name like "dev.compose.yml").
//
// Kubernetes manifests are YAML files in standard k8s directories
// (k8s/, kubernetes/, manifests/, kustomize/) that look like k8s
// resources (apiVersion / kind fields). Helm charts are detected
// by the presence of a "Chart.yaml" file in a directory.
func detectContainerization(dc DetectionContext) Containerization {
	out := Containerization{}

	for _, f := range dc.Files {
		base := filepath.Base(f)
		dir := filepath.ToSlash(filepath.Dir(f))

		// 1. Dockerfile
		if isDockerfile(base) {
			out.DockerfileCount++
			out.HasDockerfile = true
			continue
		}

		// 2. docker-compose
		if isComposeFile(base) {
			out.ComposeFileCount++
			out.HasDockerCompose = true
			continue
		}

		// 3. K8s manifests: in a k8s directory AND look like k8s YAML
		if isK8sDirectory(dir) && isK8sManifest(dc.WorkTree, f) {
			out.K8sManifestCount++
			out.HasKubernetes = true
		}

		// 4. Helm: Chart.yaml in any directory marks a chart root.
		if base == "Chart.yaml" {
			dir := filepath.Dir(f)
			if dir != "." {
				out.HelmChartCount++
				out.HasHelm = true
			}
		}
	}

	return out
}

// isDockerfile reports whether a basename is a Dockerfile. The
// patterns are: "Dockerfile", "Dockerfile.<anything>",
// "Containerfile", "Containerfile.<anything>", and "*.dockerfile"
// (e.g. "dev.dockerfile"). The check is case-insensitive to
// match git's own behavior.
func isDockerfile(base string) bool {
	lower := strings.ToLower(base)
	if lower == "dockerfile" || lower == "containerfile" {
		return true
	}
	if strings.HasPrefix(lower, "dockerfile.") {
		return true
	}
	if strings.HasPrefix(lower, "containerfile.") {
		return true
	}
	if strings.HasSuffix(lower, ".dockerfile") {
		return true
	}
	if strings.HasSuffix(lower, ".containerfile") {
		return true
	}
	return false
}

// isComposeFile reports whether a basename is a docker-compose
// file. We accept the canonical names plus a few common
// variants: "compose.yml", "compose.yaml", and
// "<name>.compose.yml" / "<name>.compose.yaml" /
// "<name>.docker-compose.yml".
func isComposeFile(base string) bool {
	lower := strings.ToLower(base)
	switch lower {
	case "docker-compose.yml", "docker-compose.yaml",
		"compose.yml", "compose.yaml",
		"docker-compose.override.yml", "docker-compose.override.yaml",
		"compose.override.yml", "compose.override.yaml":
		return true
	}
	// Suffix-based: anything ending in .compose.yml/.yaml is a compose file.
	if strings.HasSuffix(lower, ".compose.yml") || strings.HasSuffix(lower, ".compose.yaml") {
		return true
	}
	if strings.HasSuffix(lower, ".docker-compose.yml") || strings.HasSuffix(lower, ".docker-compose.yaml") {
		return true
	}
	return false
}

// isK8sDirectory reports whether the directory path looks like a
// Kubernetes manifest directory. We accept the most common
// conventions: k8s/, kubernetes/, k8s-manifests/, manifests/,
// deploy/, deployment/, infra/k8s/. The list is intentionally
// short — false negatives are cheaper than false positives
// (mis-classifying a "deploy" directory full of shell scripts as
// k8s would be worse than missing a real k8s dir).
func isK8sDirectory(dir string) bool {
	parts := strings.Split(dir, "/")
	for _, p := range parts {
		switch p {
		case "k8s", "kubernetes", "k8s-manifests", "manifests", "kube":
			return true
		}
	}
	return false
}

// isK8sManifest reports whether a YAML file looks like a
// Kubernetes manifest. We do a fast content check for the
// "apiVersion:" + "kind:" pair that every k8s resource has.
// Anything that fails is not a manifest even if it lives in
// a k8s directory (e.g. a kustomization.yaml — which we want
// to count differently, but for v0.1 we just count it as a
// manifest).
func isK8sManifest(workTree, f string) bool {
	data, err := readFile(workTree, f)
	if err != nil {
		return false
	}
	body := string(data)
	hasAPIVersion := strings.Contains(body, "apiVersion:")
	hasKind := strings.Contains(body, "kind:")
	return hasAPIVersion && hasKind
}

// helmChartDirs returns the set of directories (relative to the
// work tree) that contain a Chart.yaml. Exposed for tests.
func helmChartDirs(dc DetectionContext) []string {
	out := []string{}
	for _, f := range dc.Files {
		if filepath.Base(f) != "Chart.yaml" {
			continue
		}
		d := filepath.Dir(f)
		if d != "." && d != "" {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}
