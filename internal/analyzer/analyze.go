package analyzer

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/got-sh/got/internal/config"
)

// Analyze runs the full detection pipeline and returns the
// resulting model. The work tree must exist and be readable; an
// error is returned when the work tree path does not exist or is
// not a directory.
//
// Analyze is safe to call from multiple goroutines against the same
// Analyzer (the file walker and the git adapter are both stateless
// per-call), but the returned RepositoryModel is the analyzer's
// and should be copied by the caller if it will be mutated.
//
// The function never panics; any non-fatal error during file
// walking is collected in the returned model's errors log
// (currently not exposed in the JSON model — they're surfaced via
// slog in the CLI). A fatal error is reserved for "work tree does
// not exist" or similar unrecoverable conditions.
func (a *Analyzer) Analyze(ctx context.Context) (RepositoryModel, error) {
	if a == nil {
		return RepositoryModel{}, fmt.Errorf("analyzer: nil receiver")
	}
	if a.workTree == "" {
		return RepositoryModel{}, fmt.Errorf("analyzer: empty work tree path")
	}

	model := RepositoryModel{
		Path:          a.workTree,
		DetectedAt:    time.Now().UTC(),
		DefaultBranch: "main",
	}

	// 1. Walk the work tree once. Every subsequent detector reads
	//    from this list so we never descend twice.
	walk, err := walkFiles(a.workTree, WalkOptions{})
	if err != nil {
		return model, fmt.Errorf("analyzer: walk %s: %w", a.workTree, err)
	}

	// 2. Repository identity: read got.yml when present.
	projectCfg, cfgErr := readProjectConfig(a.workTree)
	if cfgErr == nil {
		if projectCfg.Project.Name != "" {
			model.Name = projectCfg.Project.Name
		}
		if projectCfg.Project.DefaultBranch != "" {
			model.DefaultBranch = projectCfg.Project.DefaultBranch
		}
	}
	if model.Name == "" {
		model.Name = filepath.Base(a.workTree)
	}

	dc := DetectionContext{
		WorkTree:    a.workTree,
		Files:       walk.files,
		SkippedDirs: walk.skippedDirs,
		Errors:      walk.errors,
	}

	// 3. Built-in detectors. These are the core engine; the order
	//    below matches the order fields appear on the model.
	model.Languages = detectLanguages(dc)
	model.Frameworks = detectFrameworks(dc)
	model.PackageManagers = detectPackageManagers(dc)
	model.CICDSystems = detectCICD(dc)
	model.Containerization = detectContainerization(dc)
	model.Monorepo = detectMonorepo(dc)
	model.Type, model.TypeReason = classifyRepository(dc, model)

	// 4. Statistics. This is the only step that talks to the git
	//    adapter. A nil adapter (rare, mostly in tests) yields
	//    a model with zero-valued statistics and no error.
	stats, statErr := computeStats(ctx, a.adapter, dc)
	if statErr != nil {
		// Statistics are best-effort: a corrupted index or
		// empty repo shouldn't fail the whole inspection.
		// Surface the error in TypeReason for visibility.
		if model.TypeReason != "" {
			model.TypeReason = model.TypeReason + "; "
		}
		model.TypeReason = model.TypeReason + "stats: " + statErr.Error()
	}
	model.Stats = stats

	// 5. Custom detectors. Each may add or refine items. We
	//    rebuild the affected slices from scratch; the core
	//    detectors always run first so the user detectors can
	//    deduplicate against them by name.
	for _, d := range a.detectors {
		if d == nil {
			continue
		}
		items, err := d.Detect(ctx, dc)
		if err != nil {
			if model.TypeReason != "" {
				model.TypeReason = model.TypeReason + "; "
			}
			model.TypeReason = model.TypeReason + "detector " + d.Name() + ": " + err.Error()
			continue
		}
		applyDetectedItems(&model, items)
	}

	return model, nil
}

// applyDetectedItems merges items from a user-supplied detector
// into the core model. Items are deduplicated by (Kind, Name) —
// the first detection (core or user) wins. A KindRepositoryType
// item overrides the inferred type when present.
func applyDetectedItems(m *RepositoryModel, items []DetectedItem) {
	for _, it := range items {
		switch it.Kind {
		case KindLanguage:
			m.Languages = upsertLanguage(m.Languages, it)
		case KindFramework:
			m.Frameworks = upsertFramework(m.Frameworks, it)
		case KindPackageManager:
			m.PackageManagers = upsertPackageManager(m.PackageManagers, it)
		case KindCICD:
			m.CICDSystems = upsertCICD(m.CICDSystems, it)
		case KindMonorepo:
			if !m.Monorepo.IsMonorepo {
				m.Monorepo.IsMonorepo = true
				if it.Name != "" {
					m.Monorepo.Tool = it.Name
				}
			}
		case KindRepositoryType:
			m.Type = RepositoryType(it.Name)
			if it.Category != "" {
				m.TypeReason = it.Category
			}
		case KindCustom, "":
			// Ignored at the model level; downstream tools
			// that consume the JSON can see these if we
			// ever plumb them through.
		}
	}
}

// upsertLanguage inserts it into the language list if a language
// with the same name is not already present. Otherwise it is a
// no-op (the core detection wins).
func upsertLanguage(in []LanguageStat, it DetectedItem) []LanguageStat {
	for _, l := range in {
		if l.Name == it.Name {
			return in
		}
	}
	return append(in, LanguageStat{
		Name:      it.Name,
		FileCount: 0,
	})
}

// upsertFramework is the framework equivalent of upsertLanguage.
// Evidence becomes the ConfigFiles list; Confidence is preserved.
func upsertFramework(in []Framework, it DetectedItem) []Framework {
	for _, f := range in {
		if f.Name == it.Name {
			return in
		}
	}
	return append(in, Framework{
		Name:        it.Name,
		Category:    it.Category,
		Language:    it.Language,
		Version:     it.Version,
		Confidence:  defaultConfidence(it.Confidence),
		ConfigFiles: append([]string(nil), it.Evidence...),
	})
}

// upsertPackageManager is the package-manager equivalent.
func upsertPackageManager(in []PackageManager, it DetectedItem) []PackageManager {
	for _, p := range in {
		if p.Name == it.Name {
			return in
		}
	}
	manifest := ""
	if len(it.Evidence) > 0 {
		manifest = it.Evidence[0]
	}
	return append(in, PackageManager{
		Name:      it.Name,
		Ecosystem: it.Language,
		Manifest:  manifest,
	})
}

// upsertCICD is the CI/CD equivalent. The provider is taken from
// the Category field (e.g. "github") and the config files come from
// Evidence.
func upsertCICD(in []CICDSystem, it DetectedItem) []CICDSystem {
	for _, c := range in {
		if c.Name == it.Name {
			return in
		}
	}
	return append(in, CICDSystem{
		Name:        it.Name,
		Provider:    it.Category,
		ConfigFiles: append([]string(nil), it.Evidence...),
	})
}

// defaultConfidence normalizes the confidence string. Detectors
// may pass empty, "high", "medium", or "low"; we accept those and
// default to "medium" for anything else.
func defaultConfidence(c string) string {
	switch strings.ToLower(strings.TrimSpace(c)) {
	case "high", "medium", "low":
		return strings.ToLower(c)
	}
	return "medium"
}

// readProjectConfig reads got.yml from the work tree. A missing
// file is not an error; in that case (cfg, nil) is returned and
// the caller uses defaults.
func readProjectConfig(workTree string) (config.ProjectConfig, error) {
	ymlPath := filepath.Join(workTree, "got.yml")
	if !fileExists(workTree, "got.yml") {
		return config.ProjectConfig{}, nil
	}
	return config.ReadProjectConfig(ymlPath)
}

// sortLanguages sorts the language list by line count descending.
// Languages with zero lines sort to the bottom but are still kept
// (they are useful as a "this language is present" signal).
func sortLanguages(s []LanguageStat) []LanguageStat {
	sort.SliceStable(s, func(i, j int) bool {
		if s[i].LineCount != s[j].LineCount {
			return s[i].LineCount > s[j].LineCount
		}
		if s[i].Bytes != s[j].Bytes {
			return s[i].Bytes > s[j].Bytes
		}
		return s[i].Name < s[j].Name
	})
	return s
}
