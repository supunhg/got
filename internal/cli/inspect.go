package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/analyzer"
)

// inspectOptions holds the resolved flag values for `got inspect`.
// Kept as a struct so tests can construct one directly without
// going through cobra.
type inspectOptions struct {
	asJSON bool
	// asYAML reserved for a future release; the v0.1 surface
	// is human-readable and --json.
	noHeader bool
}

// inspectReport is the JSON shape emitted by `got inspect --json`.
// It bundles the model with a small CLI-specific block (the
// version of got that produced the report, the timestamp the
// model was generated at, etc.) so downstream tools can read
// either or both in one shot.
type inspectReport struct {
	GotVersion  string                    `json:"gotVersion"`
	GeneratedAt string                    `json:"generatedAt"`
	Model       *analyzer.RepositoryModel `json:"model"`
}

// newInspectCmd builds `got inspect [--json]`. The default
// output is a human-readable, multi-section rendering; --json
// emits the full RepositoryModel as indented JSON.
func newInspectCmd(d Deps) *cobra.Command {
	opts := &inspectOptions{}
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Show repository intelligence (languages, frameworks, stats)",
		Long: `Show repository intelligence: detected languages, frameworks,
package managers, CI/CD systems, containerization, monorepo
structure, repository type, and basic statistics (commit count,
branch count, contributor count, file count, line count).

The output is read-only and offline-only: no network calls, no
mutations. Detection walks the work tree and inspects manifest
files; statistics come from the git history.

Use --json to get the full RepositoryModel as machine-readable
JSON. The model is also suitable for piping to ` + "`jq`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInspect(cmd, d, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.asJSON, "json", false, "emit the full RepositoryModel as JSON")
	cmd.Flags().BoolVar(&opts.noHeader, "no-header", false, "omit the human-readable top header (useful for scripts)")
	return cmd
}

// runInspect is the shared implementation behind the cobra
// handler and the test suite. It is split out so tests can
// drive it without going through the command tree.
func runInspect(cmd *cobra.Command, d Deps, opts *inspectOptions) error {
	logger := loggerFor(d)
	logger.Info("inspect starting", "json", opts.asJSON, "noHeader", opts.noHeader)
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := d.AdapterFor(workTree)
	az := analyzer.New(workTree, a)
	model, err := az.Analyze(cmd.Context())
	if err != nil {
		logger.Warn("inspect failed", "err", err.Error())
		return err
	}
	logger.Info(
		"inspect finished",
		"languages", len(model.Languages),
		"frameworks", len(model.Frameworks),
		"commits", model.Stats.CommitCount,
		"branches", model.Stats.BranchCount,
	)
	out := cmdWriter(cmd, d)
	if opts.asJSON {
		return writeInspectJSON(out, d, model)
	}
	return writeInspectHuman(out, d, workTree, model, opts.noHeader)
}

// writeInspectJSON encodes the model as indented JSON. The
// wrapper includes the got version and the generation time so
// the report is self-describing.
func writeInspectJSON(w io.Writer, d Deps, model analyzer.RepositoryModel) error {
	report := inspectReport{
		GotVersion:  d.GotVersion,
		GeneratedAt: model.DetectedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Model:       &model,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// writeInspectHuman renders the model as a multi-section
// human-readable report. Each section is printed unconditionally
// (the user knows what they asked for); empty sections are
// rendered as "(none)" so the structure is obvious.
func writeInspectHuman(w io.Writer, d Deps, workTree string, m analyzer.RepositoryModel, noHeader bool) error {
	if !noHeader {
		writeInspectHeader(w, d, workTree, m)
	}
	writeIdentitySection(w, m)
	writeLanguagesSection(w, m.Languages)
	writeFrameworksSection(w, m.Frameworks)
	writePackageManagersSection(w, m.PackageManagers)
	writeCICDSection(w, m.CICDSystems)
	writeContainerizationSection(w, m.Containerization)
	writeMonorepoSection(w, m.Monorepo)
	writeTypeSection(w, m)
	writeStatsSection(w, m.Stats)
	return nil
}

// writeInspectHeader prints the top-of-output banner with the
// work tree path and a short summary of the most important
// facts. The header is what users see in CI logs and pipe
// captures — keep it short and skimmable.
func writeInspectHeader(w io.Writer, d Deps, workTree string, m analyzer.RepositoryModel) {
	defaultBranch := m.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	fmt.Fprintf(w, "Repository:  %s\n", m.Name)
	fmt.Fprintf(w, "Path:        %s\n", workTree)
	fmt.Fprintf(w, "Type:        %s\n", m.Type)
	fmt.Fprintf(w, "Default:     %s\n", defaultBranch)
	if m.TypeReason != "" {
		fmt.Fprintf(w, "             %s\n", m.TypeReason)
	}
	fmt.Fprintln(w)
}

// writeIdentitySection prints the Description field (when set).
// The header already covers the other identity fields.
func writeIdentitySection(w io.Writer, m analyzer.RepositoryModel) {
	if m.Description == "" {
		return
	}
	fmt.Fprintf(w, "Description: %s\n\n", m.Description)
}

// writeLanguagesSection prints the detected languages as a
// table. Languages with zero lines (binary-only or non-source
// languages) are included but flagged as "binary" in the
// table. The Percentage column is hidden when the total
// source bytes is zero (an empty repo) to avoid "0.0%" noise.
func writeLanguagesSection(w io.Writer, langs []analyzer.LanguageStat) {
	fmt.Fprintln(w, "Languages:")
	if len(langs) == 0 {
		fmt.Fprintln(w, "  (none detected)")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tFILES\tLINES\tBYTES\t%")
	totalBytes := int64(0)
	for _, l := range langs {
		totalBytes += l.Bytes
	}
	for _, l := range langs {
		pct := ""
		if totalBytes > 0 {
			pct = fmt.Sprintf("%.1f%%", l.Percentage)
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%d\t%d\t%s\t%s\n",
			l.Name, l.FileCount, l.LineCount, formatBytes(l.Bytes), pct)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

// writeFrameworksSection prints the detected frameworks as a
// table. Frameworks are grouped by Category (web, ui, test, ...)
// in the order they appear in the model (which is already
// sorted by confidence desc, then name asc).
func writeFrameworksSection(w io.Writer, fws []analyzer.Framework) {
	if len(fws) == 0 {
		fmt.Fprintln(w, "Frameworks:")
		fmt.Fprintln(w, "  (none detected)")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "Frameworks:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tCATEGORY\tLANGUAGE\tVERSION\tCONFIDENCE")
	for _, f := range fws {
		version := f.Version
		if version == "" {
			version = "-"
		}
		lang := f.Language
		if lang == "" {
			lang = "-"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			f.Name, f.Category, lang, version, f.Confidence)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

// writePackageManagersSection prints package managers as a
// table. Manifest / Lockfile columns are shown; "frozen" is a
// boolean column (yes / no).
func writePackageManagersSection(w io.Writer, pms []analyzer.PackageManager) {
	fmt.Fprintln(w, "Package managers:")
	if len(pms) == 0 {
		fmt.Fprintln(w, "  (none detected)")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tECOSYSTEM\tMANIFEST\tLOCKFILE\tFROZEN")
	for _, p := range pms {
		manifest := p.Manifest
		if manifest == "" {
			manifest = "-"
		}
		lockfile := p.Lockfile
		if lockfile == "" {
			lockfile = "-"
		}
		frozen := "no"
		if p.IsFrozen {
			frozen = "yes"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n",
			p.Name, p.Ecosystem, manifest, lockfile, frozen)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

// writeCICDSection prints CI/CD systems as a table. Workflow
// count is the number of files in the config directory.
func writeCICDSection(w io.Writer, cics []analyzer.CICDSystem) {
	fmt.Fprintln(w, "CI/CD:")
	if len(cics) == 0 {
		fmt.Fprintln(w, "  (none detected)")
		fmt.Fprintln(w)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  NAME\tPROVIDER\tWORKFLOWS")
	for _, c := range cics {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%d\n", c.Name, c.Provider, c.WorkflowCount)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

// writeContainerizationSection prints the containerization
// summary as a list of "yes" / "no" lines. The detail (counts
// of Dockerfiles, compose files, K8s manifests, Helm charts) is
// only printed for the "yes" lines.
func writeContainerizationSection(w io.Writer, c analyzer.Containerization) {
	fmt.Fprintln(w, "Containerization:")
	if c.IsEmpty() {
		fmt.Fprintln(w, "  (no container artifacts detected)")
		fmt.Fprintln(w)
		return
	}
	if c.HasDockerfile {
		fmt.Fprintf(w, "  Dockerfile:        yes (%d file(s))\n", c.DockerfileCount)
	} else {
		fmt.Fprintln(w, "  Dockerfile:        no")
	}
	if c.HasDockerCompose {
		fmt.Fprintf(w, "  Docker Compose:    yes (%d file(s))\n", c.ComposeFileCount)
	} else {
		fmt.Fprintln(w, "  Docker Compose:    no")
	}
	if c.HasKubernetes {
		fmt.Fprintf(w, "  Kubernetes:        yes (%d manifest(s))\n", c.K8sManifestCount)
	} else {
		fmt.Fprintln(w, "  Kubernetes:        no")
	}
	if c.HasHelm {
		fmt.Fprintf(w, "  Helm:              yes (%d chart(s))\n", c.HelmChartCount)
	} else {
		fmt.Fprintln(w, "  Helm:              no")
	}
	fmt.Fprintln(w)
}

// writeMonorepoSection prints the monorepo summary. When the
// repo is not a monorepo we print a single line so the user
// knows the section was checked.
func writeMonorepoSection(w io.Writer, m analyzer.MonorepoInfo) {
	fmt.Fprintln(w, "Monorepo:")
	if !m.IsMonorepo {
		fmt.Fprintln(w, "  (not a monorepo)")
		fmt.Fprintln(w)
		return
	}
	if m.Tool != "" {
		fmt.Fprintf(w, "  Tool:      %s\n", m.Tool)
	}
	fmt.Fprintf(w, "  Packages:  %d\n", m.PackageCount)
	if len(m.Packages) > 0 {
		// Show up to 10 paths, then "(and N more)".
		shown := m.Packages
		if len(shown) > 10 {
			shown = shown[:10]
		}
		for _, p := range shown {
			fmt.Fprintf(w, "    - %s\n", filepath.ToSlash(p))
		}
		if len(m.Packages) > 10 {
			fmt.Fprintf(w, "    ... and %d more\n", len(m.Packages)-10)
		}
	}
	fmt.Fprintln(w)
}

// writeTypeSection prints the inferred repository type and
// the reason string. Empty reason is suppressed.
func writeTypeSection(w io.Writer, m analyzer.RepositoryModel) {
	fmt.Fprintf(w, "Type:        %s\n", m.Type)
	if m.TypeReason != "" {
		fmt.Fprintf(w, "             %s\n", m.TypeReason)
	}
	fmt.Fprintln(w)
}

// writeStatsSection prints the basic statistics block. Dates
// are rendered in ISO-8601 in UTC.
func writeStatsSection(w io.Writer, s analyzer.RepositoryStats) {
	fmt.Fprintln(w, "Statistics:")
	fmt.Fprintf(w, "  Commits:        %d\n", s.CommitCount)
	fmt.Fprintf(w, "  Branches:       %d local, %d remote-tracking\n", s.BranchCount, s.RemoteBranchCount)
	fmt.Fprintf(w, "  Contributors:   %d\n", s.ContributorCount)
	fmt.Fprintf(w, "  Files:          %d\n", s.FileCount)
	fmt.Fprintf(w, "  Lines:          %d\n", s.LineCount)
	fmt.Fprintf(w, "  Size:           %s\n", formatBytes(s.SizeBytes))
	if !s.FirstCommitAt.IsZero() {
		fmt.Fprintf(w, "  First commit:   %s\n", s.FirstCommitAt.UTC().Format("2006-01-02"))
	}
	if !s.LastCommitAt.IsZero() {
		fmt.Fprintf(w, "  Last commit:    %s\n", s.LastCommitAt.UTC().Format("2006-01-02"))
	}
}
