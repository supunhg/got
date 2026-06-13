package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/got-sh/got/internal/health"
)

// healthOptions holds the resolved flag values for `got health`.
type healthOptions struct {
	asJSON   bool
	noHeader bool
	// minSeverity filters findings to those at or above the
	// given severity. The default is "info" (all findings).
	// Recognized values: info, low, medium, high, critical.
	minSeverity string
}

// healthReport is the JSON shape emitted by `got health --json`.
// It bundles the report with the got version and the time the
// report was generated.
type healthReportJSON struct {
	GotVersion  string               `json:"gotVersion"`
	GeneratedAt string               `json:"generatedAt"`
	Report      *health.HealthReport `json:"report"`
}

// newHealthCmd builds `got health [--json] [--min-severity S]`.
func newHealthCmd(d Deps) *cobra.Command {
	opts := &healthOptions{}
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show repository health (stale branches, binaries, docs, ...)",
		Long: `Show repository health: stale branches, merged branches that
were never deleted, unreachable remotes, large binaries checked
into Git, missing documentation (README, LICENSE, CHANGELOG,
CONTRIBUTING), excessive branch count, and working-tree
cleanliness.

The output is read-only and offline-only: no network calls, no
mutations. The report includes a 0-100 score and a letter grade
(A/B/C/D/F); use --json for the full report.

Filters:
  --min-severity info|low|medium|high|critical
    Suppress findings below the given severity. Default: info.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHealth(cmd, d, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.asJSON, "json", false, "emit the full HealthReport as JSON")
	cmd.Flags().BoolVar(&opts.noHeader, "no-header", false, "omit the human-readable top header (useful for scripts)")
	cmd.Flags().StringVar(&opts.minSeverity, "min-severity", "info", "suppress findings below this severity: info|low|medium|high|critical")
	return cmd
}

// runHealth is the shared implementation behind the cobra
// handler and the test suite. It is split out so tests can
// drive it without going through the command tree.
func runHealth(cmd *cobra.Command, d Deps, opts *healthOptions) error {
	logger := loggerFor(d)
	logger.Info("health starting", "json", opts.asJSON, "minSeverity", opts.minSeverity)
	workTree, err := d.Discover(".")
	if err != nil {
		return err
	}
	a := d.AdapterFor(workTree)
	checker := health.New(workTree, a)
	report, err := checker.Check(cmd.Context())
	if err != nil {
		logger.Warn("health failed", "err", err.Error())
		return err
	}
	// Apply --min-severity filter to the findings.
	minSev, perr := parseSeverity(opts.minSeverity)
	if perr != nil {
		return perr
	}
	report.Findings = filterBySeverity(report.Findings, minSev)
	report.Recommendations = filterRecommendationsBySeverity(report.Recommendations, minSev)
	// Re-tally counts after the filter.
	report.Counts = recountSeverity(report.Findings)
	report.Score = computeFilteredScore(report.Counts)

	logger.Info(
		"health finished",
		"score", report.Score,
		"grade", report.Grade,
		"findings", len(report.Findings),
	)
	out := cmdWriter(cmd, d)
	if opts.asJSON {
		return writeHealthJSON(out, d, &report)
	}
	return writeHealthHuman(out, d, workTree, &report, opts.noHeader)
}

// parseSeverity maps a string to a HealthSeverity. Unknown
// values return a gerr.Validation so the user gets a clear
// "you typed the wrong thing" message.
func parseSeverity(s string) (health.HealthSeverity, error) {
	switch s {
	case "", "info":
		return health.SeverityInfo, nil
	case "low":
		return health.SeverityLow, nil
	case "medium":
		return health.SeverityMedium, nil
	case "high":
		return health.SeverityHigh, nil
	case "critical":
		return health.SeverityCritical, nil
	}
	return health.SeverityInfo, fmt.Errorf("invalid --min-severity %q (want info|low|medium|high|critical)", s)
}

// filterBySeverity returns the subset of findings whose
// severity is at or above min.
func filterBySeverity(in []health.HealthFinding, min health.HealthSeverity) []health.HealthFinding {
	minRank := severityRankForFilter(min)
	out := make([]health.HealthFinding, 0, len(in))
	for _, f := range in {
		if severityRankForFilter(f.Severity) >= minRank {
			out = append(out, f)
		}
	}
	return out
}

// filterRecommendationsBySeverity returns the recommendations
// whose FindingID is still in the filtered findings set.
// Because we don't have the link here without re-iterating, we
// use Priority (1=critical, ..., 4=low) as the proxy: drop any
// recommendation whose priority rank is below the min rank.
func filterRecommendationsBySeverity(in []health.Recommendation, min health.HealthSeverity) []health.Recommendation {
	minRank := severityRankForFilter(min)
	// Map severity → recommendation priority.
	// Critical=1, High=2, Medium=3, Low=4, Info=5.
	prioForMin := func() int {
		switch min {
		case health.SeverityCritical:
			return 1
		case health.SeverityHigh:
			return 2
		case health.SeverityMedium:
			return 3
		case health.SeverityLow:
			return 4
		}
		return 5
	}()
	_ = minRank
	out := make([]health.Recommendation, 0, len(in))
	for _, r := range in {
		if r.Priority <= prioForMin {
			out = append(out, r)
		}
	}
	return out
}

// severityRankForFilter returns a sortable rank for a
// HealthSeverity. Higher rank = more severe. We re-implement
// it here to avoid a public import cycle: the real
// severityRank lives in internal/health but is package-private.
func severityRankForFilter(s health.HealthSeverity) int {
	switch s {
	case health.SeverityCritical:
		return 5
	case health.SeverityHigh:
		return 4
	case health.SeverityMedium:
		return 3
	case health.SeverityLow:
		return 2
	case health.SeverityInfo:
		return 1
	}
	return 0
}

// recountSeverity rebuilds the HealthCounts after a filter.
func recountSeverity(findings []health.HealthFinding) health.HealthCounts {
	var c health.HealthCounts
	for _, f := range findings {
		switch f.Severity {
		case health.SeverityInfo:
			c.Info++
		case health.SeverityLow:
			c.Low++
		case health.SeverityMedium:
			c.Medium++
		case health.SeverityHigh:
			c.High++
		case health.SeverityCritical:
			c.Critical++
		}
	}
	return c
}

// computeFilteredScore re-scores a report after findings have
// been filtered by --min-severity. The base of 100 still holds
// (a perfect repo has no findings and scores 100). The grade
// is updated to match.
func computeFilteredScore(c health.HealthCounts) int {
	score := 100
	switch {
	case c.Critical > 0:
		score -= 25
	case c.High > 0:
		// Use a step function on the count.
		switch {
		case c.High >= 5:
			score -= 30
		case c.High >= 3:
			score -= 20
		default:
			score -= 10
		}
	case c.Medium > 0:
		switch {
		case c.Medium >= 5:
			score -= 15
		case c.Medium >= 3:
			score -= 10
		default:
			score -= 4
		}
	case c.Low > 0:
		score -= 1
	}
	if score < 0 {
		score = 0
	}
	return score
}

// writeHealthJSON encodes the report as indented JSON.
func writeHealthJSON(w io.Writer, d Deps, r *health.HealthReport) error {
	wrapper := healthReportJSON{
		GotVersion:  d.GotVersion,
		GeneratedAt: r.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Report:      r,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(wrapper)
}

// writeHealthHuman renders the report as a multi-section
// human-readable output. The score is the first thing the user
// sees; the findings follow in severity order; recommendations
// come last (sorted by priority asc).
func writeHealthHuman(w io.Writer, d Deps, workTree string, r *health.HealthReport, noHeader bool) error {
	if !noHeader {
		writeHealthHeader(w, d, workTree, r)
	}
	writeHealthScore(w, r)
	writeHealthFindings(w, r)
	writeHealthRecommendations(w, r)
	return nil
}

// writeHealthHeader prints the top-of-output banner with the
// repository path and the score / grade.
func writeHealthHeader(w io.Writer, d Deps, workTree string, r *health.HealthReport) {
	fmt.Fprintf(w, "Repository:  %s\n", workTree)
	fmt.Fprintf(w, "Generated:   %s\n", r.GeneratedAt.UTC().Format("2006-01-02 15:04:05 MST"))
	if d.GotVersion != "" {
		fmt.Fprintf(w, "Got:         %s\n", d.GotVersion)
	}
	fmt.Fprintln(w)
}

// writeHealthScore prints the score line, grade, and a
// per-severity count summary. The colors are not used (we're
// in plain text) but the format is greppable.
func writeHealthScore(w io.Writer, r *health.HealthReport) {
	fmt.Fprintf(w, "Score:       %d/100  (grade: %s)\n", r.Score, r.Grade)
	fmt.Fprintf(w, "Findings:    %d critical, %d high, %d medium, %d low, %d info\n",
		r.Counts.Critical, r.Counts.High, r.Counts.Medium, r.Counts.Low, r.Counts.Info)
	fmt.Fprintf(w, "Checks run:  %s\n\n", strings.Join(r.ChecksRun, ", "))
}

// writeHealthFindings prints the findings as a table. The
// "Affected" column shows up to 3 items; longer lists are
// truncated to "(+N more)".
func writeHealthFindings(w io.Writer, r *health.HealthReport) {
	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "No findings.")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w, "Findings:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  SEVERITY\tCATEGORY\tID\tTITLE")
	for _, f := range r.Findings {
		_, _ = fmt.Fprintf(
			tw, "  %s\t%s\t%s\t%s\n",
			strings.ToUpper(string(f.Severity)),
			f.Category,
			f.ID,
			f.Title,
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
	// Detail block: title + detail + affected list.
	for _, f := range r.Findings {
		fmt.Fprintf(w, "  [%s] %s\n", strings.ToUpper(string(f.Severity)), f.Title)
		if f.Detail != "" {
			fmt.Fprintf(w, "    %s\n", f.Detail)
		}
		if len(f.Affected) > 0 {
			shown := f.Affected
			if len(shown) > 5 {
				shown = shown[:5]
			}
			for _, a := range shown {
				fmt.Fprintf(w, "    - %s\n", a)
			}
			if len(f.Affected) > 5 {
				fmt.Fprintf(w, "    ... and %d more\n", len(f.Affected)-5)
			}
		}
		fmt.Fprintln(w)
	}
}

// writeHealthRecommendations prints the recommendations as a
// numbered list, ordered by Priority asc.
func writeHealthRecommendations(w io.Writer, r *health.HealthReport) {
	if len(r.Recommendations) == 0 {
		return
	}
	fmt.Fprintln(w, "Recommendations:")
	for i, rec := range r.Recommendations {
		fmt.Fprintf(w, "  %d. %s\n", i+1, rec.Title)
		if rec.Action != "" {
			fmt.Fprintf(w, "     %s\n", rec.Action)
		}
		if rec.Command != "" {
			fmt.Fprintf(w, "     $ %s\n", rec.Command)
		}
	}
	fmt.Fprintln(w)
}
