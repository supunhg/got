package analyzer

import (
	"path/filepath"
	"sort"
	"strings"
)

// languageByExt maps a lower-cased file extension (with leading
// dot) to a language name. The set is intentionally conservative:
// ambiguous extensions like .h (C, C++, Objective-C) are mapped
// to the most common language; the per-file language picker (in
// detectLanguages) does the final classification.
var languageByExt = map[string]string{
	".go":         "Go",
	".py":         "Python",
	".pyi":        "Python",
	".js":         "JavaScript",
	".mjs":        "JavaScript",
	".cjs":        "JavaScript",
	".jsx":        "JavaScript",
	".ts":         "TypeScript",
	".tsx":        "TypeScript",
	".d.ts":       "TypeScript",
	".java":       "Java",
	".kt":         "Kotlin",
	".kts":        "Kotlin",
	".swift":      "Swift",
	".m":          "Objective-C",
	".mm":         "Objective-C++",
	".c":          "C",
	".h":          "C",
	".cc":         "C++",
	".cpp":        "C++",
	".cxx":        "C++",
	".c++":        "C++",
	".hpp":        "C++",
	".hh":         "C++",
	".hxx":        "C++",
	".cs":         "C#",
	".fs":         "F#",
	".fsx":        "F#",
	".vb":         "Visual Basic",
	".rb":         "Ruby",
	".erb":        "Ruby",
	".php":        "PHP",
	".phtml":      "PHP",
	".rs":         "Rust",
	".scala":      "Scala",
	".sc":         "Scala",
	".sh":         "Shell",
	".bash":       "Shell",
	".zsh":        "Shell",
	".ksh":        "Shell",
	".fish":       "Shell",
	".ps1":        "PowerShell",
	".psm1":       "PowerShell",
	".pl":         "Perl",
	".pm":         "Perl",
	".lua":        "Lua",
	".r":          "R",
	".R":          "R",
	".dart":       "Dart",
	".ex":         "Elixir",
	".exs":        "Elixir",
	".erl":        "Erlang",
	".hrl":        "Erlang",
	".hs":         "Haskell",
	".lhs":        "Haskell",
	".ml":         "OCaml",
	".mli":        "OCaml",
	".clj":        "Clojure",
	".cljs":       "ClojureScript",
	".cljc":       "Clojure",
	".edn":        "Clojure",
	".lisp":       "Lisp",
	".lsp":        "Lisp",
	".scm":        "Scheme",
	".ss":         "Scheme",
	".groovy":     "Groovy",
	".gradle":     "Groovy",
	".tf":         "Terraform",
	".tfvars":     "Terraform",
	".hcl":        "HCL",
	".nomad":      "HCL",
	".vue":        "Vue",
	".svelte":     "Svelte",
	".astro":      "Astro",
	".html":       "HTML",
	".htm":        "HTML",
	".xhtml":      "HTML",
	".xml":        "XML",
	".xsd":        "XML",
	".xsl":        "XML",
	".svg":        "SVG",
	".css":        "CSS",
	".scss":       "SCSS",
	".sass":       "Sass",
	".less":       "Less",
	".styl":       "Stylus",
	".pcss":       "PostCSS",
	".md":         "Markdown",
	".mdx":        "Markdown",
	".markdown":   "Markdown",
	".rst":        "reStructuredText",
	".adoc":       "AsciiDoc",
	".asciidoc":   "AsciiDoc",
	".tex":        "TeX",
	".json":       "JSON",
	".json5":      "JSON",
	".jsonc":      "JSON",
	".yaml":       "YAML",
	".yml":        "YAML",
	".toml":       "TOML",
	".ini":        "INI",
	".cfg":        "INI",
	".conf":       "Config",
	".properties": "Properties",
	".sql":        "SQL",
	".pls":        "SQL",
	".plsql":      "SQL",
	".psql":       "SQL",
	".proto":      "Protocol Buffers",
	".graphql":    "GraphQL",
	".gql":        "GraphQL",
	".dockerfile": "Dockerfile",
	".asm":        "Assembly",
	".s":          "Assembly",
	".S":          "Assembly",
	".vim":        "Vim Script",
	".el":         "Emacs Lisp",
	".cl":         "Common Lisp",
	".zig":        "Zig",
	".nim":        "Nim",
	".nims":       "Nim",
	".cr":         "Crystal",
	".d":          "D",
	".di":         "D",
	".v":          "V",
	".sv":         "SystemVerilog",
	".vhd":        "VHDL",
	".vhdl":       "VHDL",
	".f":          "Fortran",
	".f90":        "Fortran",
	".f95":        "Fortran",
	".for":        "Fortran",
	".cob":        "COBOL",
	".pas":        "Pascal",
	".pp":         "Pascal",
	".ada":        "Ada",
	".adb":        "Ada",
	".ads":        "Ada",
	".jl":         "Julia",
	".tcl":        "Tcl",
	".rkt":        "Racket",
	".hx":         "Haxe",
	".hxml":       "Haxe",
	".pde":        "Processing",
	".tsv":        "TSV",
	".csv":        "CSV",
	".srt":        "SubRip",
	".vtt":        "WebVTT",
	".bzl":        "Starlark",
	".bzlmod":     "Starlark",
	".star":       "Starlark",
	".bazel":      "Starlark",
	".lock":       "Lockfile",
	".sum":        "Checksums",
	".mod":        "Go Modules",
	".worktrees":  "Git",
	".gitignore":  "Git",
}

// binaryExts is the set of extensions that are always treated as
// binary. Files with these extensions contribute to a language's
// byte count but not its line count.
var binaryExts = map[string]bool{
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".bmp":   true,
	".ico":   true,
	".webp":  true,
	".tiff":  true,
	".tif":   true,
	".svg":   true, // XML, but classified as binary for line-count purposes
	".pdf":   true,
	".zip":   true,
	".tar":   true,
	".gz":    true,
	".bz2":   true,
	".xz":    true,
	".7z":    true,
	".rar":   true,
	".jar":   true,
	".war":   true,
	".ear":   true,
	".aar":   true,
	".class": true,
	".pyc":   true,
	".pyo":   true,
	".pyd":   true,
	".so":    true,
	".dylib": true,
	".dll":   true,
	".exe":   true,
	".bin":   true,
	".o":     true,
	".a":     true,
	".lib":   true,
	".obj":   true,
	".deb":   true,
	".rpm":   true,
	".dmg":   true,
	".iso":   true,
	".img":   true,
	".apk":   true,
	".ipa":   true,
	".app":   true,
	".mp3":   true,
	".mp4":   true,
	".m4a":   true,
	".m4v":   true,
	".wav":   true,
	".ogg":   true,
	".flac":  true,
	".webm":  true,
	".mov":   true,
	".avi":   true,
	".mkv":   true,
	".wmv":   true,
	".flv":   true,
	".woff":  true,
	".woff2": true,
	".ttf":   true,
	".otf":   true,
	".eot":   true,
}

// detectLanguages classifies every file in dc.Files by extension
// and aggregates per-language statistics. Files with no
// recognized extension are bucketed under "Other".
//
// Line counting is best-effort: binary files contribute bytes but
// not lines. SVG is treated as binary for line-count purposes
// (it's XML, but the lines aren't "source"). JSON/YAML/TOML are
// treated as text and counted line-by-line.
//
// The result is sorted by line count descending; languages with
// zero lines sink to the bottom.
func detectLanguages(dc DetectionContext) []LanguageStat {
	if len(dc.Files) == 0 {
		return nil
	}

	// accumulators
	type acc struct {
		count int
		lines int
		bytes int64
		exts  map[string]struct{}
	}
	stats := make(map[string]*acc)
	extensions := make(map[string][]string) // lang -> ext list (sorted)
	other := &acc{exts: make(map[string]struct{})}

	// Special-case files (basenames) that imply a language
	// regardless of extension.
	basenameLang := map[string]string{
		"Dockerfile":          "Dockerfile",
		"Containerfile":       "Dockerfile",
		"Makefile":            "Makefile",
		"GNUmakefile":         "Makefile",
		"Rakefile":            "Ruby",
		"Gemfile":             "Ruby",
		"Vagrantfile":         "Ruby",
		"Brewfile":            "Ruby",
		"Jenkinsfile":         "Groovy",
		"CMakeLists.txt":      "CMake",
		"Buildfile":           "Clojure",
		"build.gradle":        "Groovy",
		"build.gradle.kts":    "Kotlin",
		"settings.gradle":     "Groovy",
		"settings.gradle.kts": "Kotlin",
		"WORKSPACE":           "Starlark",
		"BUILD":               "Starlark",
		"BUILD.bazel":         "Starlark",
	}

	get := func(name string) *acc {
		if a, ok := stats[name]; ok {
			return a
		}
		a := &acc{exts: make(map[string]struct{})}
		stats[name] = a
		return a
	}

	for _, rel := range dc.Files {
		base := filepath.Base(rel)
		ext := strings.ToLower(filepath.Ext(base))

		// Basename override wins over extension. Useful for
		// files like "Makefile" (no extension) and "Dockerfile.dev"
		// (extension that doesn't match its language).
		lang, ok := basenameLang[base]
		if !ok {
			// Some basenames are case-sensitive in the
			// filename but case-insensitive in their
			// language. Fall back to a case-insensitive match.
			upper := strings.ToUpper(base)
			for k, v := range basenameLang {
				if strings.EqualFold(k, base) || strings.ToUpper(k) == upper {
					lang = v
					ok = true
					break
				}
			}
		}
		if !ok {
			lang = languageByExt[ext]
		}
		if lang == "" {
			// Check for compound extensions (.d.ts, .blade.php)
			lang = compoundLanguage(ext, base)
		}
		if lang == "" {
			lang = "Other"
		}
		if lang == "Other" {
			other.count++
			if ext != "" {
				other.exts[ext] = struct{}{}
			}
			continue
		}
		a := get(lang)
		a.count++
		if ext != "" {
			a.exts[ext] = struct{}{}
		}
		extensions[lang] = append(extensions[lang], ext)

		// Add bytes and (for text files) lines.
		full, err := resolveUnderRoot(dc.WorkTree, rel)
		if err != nil {
			continue
		}
		a.bytes += fileSize(full)
		if isTextFile(full) && !binaryExts[ext] {
			data, err := readFile(dc.WorkTree, rel)
			if err == nil {
				a.lines += countLines(data)
			}
		}
	}

	// Emit a sorted list.
	out := make([]LanguageStat, 0, len(stats)+1)
	var totalBytes int64
	for _, a := range stats {
		totalBytes += a.bytes
	}
	if other.count > 0 {
		totalBytes += 0 // we don't double-count "Other" file sizes (no size data)
	}
	for name, a := range stats {
		exts := make([]string, 0, len(a.exts))
		for e := range a.exts {
			if e != "" {
				exts = append(exts, e)
			}
		}
		sort.Strings(exts)
		// de-duplicate extension list
		exts = uniqueStrings(exts)
		out = append(out, LanguageStat{
			Name:       name,
			Extensions: exts,
			FileCount:  a.count,
			LineCount:  a.lines,
			Bytes:      a.bytes,
			Percentage: 0, // filled below
		})
	}
	// Compute percentages after the loop so we know the denominator.
	if totalBytes > 0 {
		for i := range out {
			out[i].Percentage = round1(100.0 * float64(out[i].Bytes) / float64(totalBytes))
		}
	}
	return sortLanguages(out)
}

// compoundLanguage handles multi-part extensions like ".d.ts"
// (TypeScript declaration), ".blade.php" (Laravel Blade), and
// ".inc.php" (PHP include). Returns "" when the extension is not
// a recognized compound.
func compoundLanguage(ext, base string) string {
	// TypeScript declaration files often end in .d.ts; the .Ext
	// call above returns ".ts" and we'd mis-classify. Override.
	if strings.HasSuffix(base, ".d.ts") {
		return "TypeScript"
	}
	if strings.HasSuffix(base, ".blade.php") {
		return "PHP"
	}
	if strings.HasSuffix(base, ".inc.php") {
		return "PHP"
	}
	if strings.HasSuffix(base, ".test.js") || strings.HasSuffix(base, ".spec.js") {
		return "JavaScript"
	}
	if strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".spec.ts") {
		return "TypeScript"
	}
	if strings.HasSuffix(base, ".test.go") {
		return "Go"
	}
	if strings.HasSuffix(base, ".test.py") {
		return "Python"
	}
	switch ext {
	case ".jsx":
		return "JavaScript"
	case ".tsx":
		return "TypeScript"
	}
	return ""
}

// round1 rounds x to one decimal place.
func round1(x float64) float64 {
	return float64(int(x*10+0.5)) / 10
}

// uniqueStrings returns a copy of s with duplicates removed,
// preserving order. Used to clean extension lists that may have
// accumulated duplicates during accumulation.
func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
