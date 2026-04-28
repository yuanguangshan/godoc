package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/pflag"
)

// ============================================================
//  Configuration & Data Types
// ============================================================

var versionStr = "dev" // overridden via -ldflags "-X main.versionStr=vX.Y.Z"

type Config struct {
	RootDir           string
	OutputFile        string
	IncludeExts       []string
	IncludeMatches    []string
	ExcludeExts       []string
	ExcludeMatches    []string
	MaxFileSize       int64
	NoSubdirs         bool
	Verbose           bool
	Version           bool
	ShowStats         bool
	DryRun            bool
	NoDefaultIgnore   bool
	NoGitignore       bool
	AdditionalIgnores []string
	Copy              bool
	Push              bool
	PushURL           string
	AuthKey           string
}

type FileMetadata struct {
	RelPath   string
	FullPath  string
	Size      int64
	LineCount int
}

type Stats struct {
	PotentialMatches   int
	ExplicitlyExcluded int
	FileCount          int
	TotalSize          int64
	TotalLines         int
	TotalTokens        int
	Skipped            int
	DirCount           int
	
	DirMap             map[string]*DirStats
	ExtMap             map[string]*ExtStats
}

type SkippedFile struct {
	RelPath string
	Reason  string
}

type DirStats struct {
	Path       string
	FileCount  int
	TotalSize  int64
	TotalLines int
}

type ExtStats struct {
	Ext        string
	FileCount  int
	TotalSize  int64
	TotalLines int
}

// ============================================================
//  Ignore Rules & Language Map
// ============================================================

var ignoreDirs = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, ".svn": true, ".hg": true,
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, "bin": true, "out": true, "release": true, "debug": true,
	"__pycache__": true, ".pytest_cache": true, ".tox": true,
	".env": true, ".venv": true, "venv": true, "env": true,
	"Pods": true, "Carthage": true, "CocoaPods": true,
	"obj": true, "ipch": true, "Debug": true, "Release": true,
	"x64": true, "x86": true, "arm64": true,
	".gradle": true, ".sonar": true, ".scannerwork": true,
	"logs": true, "tmp": true, "temp": true, "cache": true,
	".history": true, ".nyc_output": true, ".coverage": true,
}

var ignoreFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "go.sum": true,
	"composer.lock": true, "Gemfile.lock": true,
	"tags": true, "TAGS": true, ".DS_Store": true,
	"coverage.xml": true, "thumbs.db": true,
}

var knownTextFiles = map[string]bool{
	"Makefile": true, "Dockerfile": true, "Rakefile": true, "Gemfile": true,
	"CMakeLists.txt": true, "Vagrantfile": true, "Jenkinsfile": true,
	"README": true, "LICENSE": true, "CHANGELOG": true, "CONTRIBUTING": true,
}

var languageMap = map[string]string{
	".go": "go", ".js": "javascript", ".ts": "typescript", ".py": "python",
	".c": "c", ".cpp": "cpp", ".h": "cpp", ".hpp": "cpp", ".cc": "cpp",
	".java": "java", ".rb": "ruby", ".php": "php", ".rs": "rust",
	".swift": "swift", ".kt": "kotlin", ".m": "objectivec", ".mm": "objectivec",
	".sh": "bash", ".zsh": "bash", ".bash": "bash", ".fish": "fish",
	".yml": "yaml", ".yaml": "yaml", ".json": "json", ".xml": "xml",
	".html": "html", ".css": "css", ".scss": "scss", ".sass": "sass", ".less": "less",
	".md": "markdown", ".sql": "sql", ".graphql": "graphql", ".proto": "protobuf",
	".dockerfile": "dockerfile", ".makefile": "makefile", ".cmake": "cmake",
	".vue": "vue", ".svelte": "svelte", ".dart": "dart", ".lua": "lua",
	".pl": "perl", ".ex": "elixir", ".erl": "erlang", ".hs": "haskell",
	".ml": "ocaml", ".clj": "clojure", ".tf": "hcl",
}

// ============================================================
//  Main Entry
// ============================================================

func main() {
	config := parseFlags()

	if config.Version {
		fmt.Printf("sourcepack %s\n", versionStr)
		return
	}

	if config.Verbose {
		printConfigSummary(config)
	}

	if !config.DryRun {
		fmt.Println("▶ Sourcepack Started")
	} else {
		fmt.Println("▶ Sourcepack Dry-Run Mode")
	}

	startTime := time.Now()

	// 1. Scan and filter
	files, stats, skipped := scanDirectory(config)

	if config.Verbose && len(skipped) > 0 {
		fmt.Printf("\n⏭  Skipped %d files:\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  - %-40s [%s]\n", s.RelPath, s.Reason)
		}
	}

	// 2. Output to Terminal
	if config.DryRun {
		printDryRun(files, stats, skipped)
	}

	if config.ShowStats {
		printStatsTerminal(files, stats)
		if config.Push {
			statsContent := generateStatsContent(config, files, stats)
			if config.PushURL == "" {
				fmt.Println("❌ SOURCEPACK_PUSH_URL env is required for push")
				os.Exit(1)
			}
			if err := pushToRemote(statsContent, config.PushURL, config.AuthKey); err != nil {
				fmt.Printf("❌ Failed to push: %v\n", err)
				os.Exit(1)
			}
			duration := time.Since(startTime)
			fmt.Printf("\n✨ Done! Pushed stats to %s (%d chars) in %v\n", config.PushURL, len(statsContent), duration)
		}
		return
	}

	if config.DryRun {
		printStatsTerminal(files, stats)
		return
	}

	// 3. Generate content
	content, err := generateContent(config, files, stats)
	if err != nil {
		fmt.Printf("❌ Error generating output: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(startTime)

	if config.Copy {
		if err := copyToClipboard(content); err != nil {
			fmt.Printf("❌ Failed to copy: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✨ Done! Copied to clipboard (%d chars) in %v\n", len(content), duration)
	} else if config.Push {
		if config.PushURL == "" {
			fmt.Println("❌ SOURCEPACK_PUSH_URL env is required for push")
			os.Exit(1)
		}
		if err := pushToRemote(content, config.PushURL, config.AuthKey); err != nil {
			fmt.Printf("❌ Failed to push: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✨ Done! Pushed to %s (%d chars) in %v\n", config.PushURL, len(content), duration)
	} else {
		if err := os.WriteFile(config.OutputFile, []byte(content), 0644); err != nil {
			fmt.Printf("❌ Error writing file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✨ Done! Generated %s in %v\n", config.OutputFile, duration)
	}
}

func parseFlags() Config {
	var c Config

	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		pflag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nCombinations:")
		fmt.Fprintln(os.Stderr, "  -s -p     Show stats and push stats-only markdown to remote")
		fmt.Fprintln(os.Stderr, "  -c -p     Not supported (-c takes priority)")
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  SOURCEPACK_PUSH_URL    Remote URL for push (e.g. https://host/submit)\n")
		fmt.Fprintf(os.Stderr, "  SOURCEPACK_AUTH_KEY    X-Auth-Key for push authentication\n")
	}

	pflag.StringVarP(&c.RootDir, "dir", "d", ".", "Root directory to scan")
	pflag.StringVarP(&c.OutputFile, "out", "o", "project_snapshot.md", "Output markdown file")
	
	var incExts, incMatches, excExts, excMatches, addIgnores string
	pflag.StringVarP(&incExts, "include", "i", "", "Include extensions (comma separated)")
	pflag.StringVarP(&incMatches, "match", "m", "", "Include path keywords (comma separated)")
	pflag.StringVarP(&excExts, "exclude", "x", "", "Exclude extensions (comma separated)")
	pflag.StringVarP(&excMatches, "exclude-match", "X", "", "Exclude path keywords (comma separated)")
	pflag.StringVarP(&addIgnores, "ignore", "", "", "Additional ignore patterns (comma separated)")

	pflag.Int64Var(&c.MaxFileSize, "max-size", 500, "Max file size in KB")
	pflag.BoolVarP(&c.NoSubdirs, "no-subdirs", "n", false, "Do not scan subdirectories")
	pflag.BoolVarP(&c.Verbose, "verbose", "v", false, "Verbose output")
	pflag.BoolVar(&c.Version, "version", false, "Show version")
	pflag.BoolVarP(&c.ShowStats, "stats", "s", false, "Show detailed multi-dimensional statistics")
	pflag.BoolVar(&c.DryRun, "dry-run", false, "Dry run mode (no file write)")
	pflag.BoolVar(&c.NoDefaultIgnore, "no-default-ignore", false, "Disable default ignore rules")
	pflag.BoolVar(&c.NoGitignore, "no-gitignore", false, "Do not load .gitignore")
	pflag.BoolVarP(&c.Copy, "copy", "c", false, "Copy output to clipboard instead of file")
	pflag.BoolVarP(&c.Push, "push", "p", false, "Push output to remote (requires SOURCEPACK_PUSH_URL env)")
	pflag.StringVar(&c.AuthKey, "auth-key", "", "X-Auth-Key for push auth (or env SOURCEPACK_AUTH_KEY)")

	pflag.Parse()

	if c.AuthKey == "" {
		c.AuthKey = os.Getenv("SOURCEPACK_AUTH_KEY")
	}
	if c.PushURL == "" {
		c.PushURL = os.Getenv("SOURCEPACK_PUSH_URL")
	}

	if incExts != "" { c.IncludeExts = cleanList(incExts) }
	if incMatches != "" { c.IncludeMatches = cleanList(incMatches) }
	if excExts != "" { c.ExcludeExts = cleanList(excExts) }
	if excMatches != "" { c.ExcludeMatches = cleanList(excMatches) }
	if addIgnores != "" { c.AdditionalIgnores = cleanList(addIgnores) }

	c.MaxFileSize *= 1024
	absRoot, _ := filepath.Abs(c.RootDir)
	c.RootDir = absRoot

	return c
}

func cleanList(s string) []string {
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" { continue }
		if !strings.HasPrefix(trimmed, ".") && !strings.ContainsAny(trimmed, "/\\") {
			res = append(res, "."+trimmed)
		} else {
			res = append(res, trimmed)
		}
	}
	return res
}

// ============================================================
//  Scanning Logic
// ============================================================

func scanDirectory(config Config) ([]FileMetadata, Stats, []SkippedFile) {
	var files []FileMetadata
	stats := Stats{
		DirMap: make(map[string]*DirStats),
		ExtMap: make(map[string]*ExtStats),
	}
	var skipped []SkippedFile

	var ignorePatterns []string
	var gitCount, gdCount int
	if !config.NoGitignore {
		gitPatterns := loadGitignore(config.RootDir)
		gitCount = len(gitPatterns)
		ignorePatterns = append(ignorePatterns, gitPatterns...)
	}
	gdPatterns := loadGdignore(config.RootDir)
	gdCount = len(gdPatterns)
	ignorePatterns = append(ignorePatterns, gdPatterns...)

	if config.Verbose {
		if gitCount > 0 { fmt.Printf("  Loaded .gitignore (%d patterns)\n", gitCount) }
		if gdCount > 0 { fmt.Printf("  Loaded .gdignore (%d patterns)\n", gdCount) }
	}

	filepath.WalkDir(config.RootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil { return nil }
		relPath, _ := filepath.Rel(config.RootDir, path)
		if relPath == "." { return nil }

		if d.IsDir() {
			if shouldIgnoreDir(relPath, config, ignorePatterns) { return filepath.SkipDir }
			if config.NoSubdirs && strings.Contains(relPath, string(filepath.Separator)) { return filepath.SkipDir }
			stats.DirCount++
			return nil
		}

		stats.PotentialMatches++
		if shouldIgnoreFile(relPath, config, ignorePatterns) {
			stats.ExplicitlyExcluded++
			return nil
		}

		// Skip output file
		outRel := config.OutputFile
		if filepath.IsAbs(outRel) {
			outRel, _ = filepath.Rel(config.RootDir, outRel)
		}
		if relPath == outRel {
			return nil
		}

		info, _ := d.Info()
		if info.Size() > config.MaxFileSize {
			skipped = append(skipped, SkippedFile{relPath, "Size limit"})
			stats.Skipped++
			return nil
		}

		if !isKnownTextFile(relPath) && isBinaryFile(path) {
			skipped = append(skipped, SkippedFile{relPath, "Binary file"})
			stats.Skipped++
			return nil
		}

		lineCount := countLines(path)
		tokens := estimateTokens(path)
		fMeta := FileMetadata{RelPath: relPath, FullPath: path, Size: info.Size(), LineCount: lineCount}
		files = append(files, fMeta)

		if config.Verbose {
			fmt.Printf("  + %-40s %d lines\n", relPath, lineCount)
		}

		// Accumulate Stats
		stats.FileCount++
		stats.TotalSize += fMeta.Size
		stats.TotalLines += fMeta.LineCount
		stats.TotalTokens += tokens

		dir := filepath.Dir(relPath)
		if _, ok := stats.DirMap[dir]; !ok { stats.DirMap[dir] = &DirStats{Path: dir} }
		stats.DirMap[dir].FileCount++
		stats.DirMap[dir].TotalSize += fMeta.Size
		stats.DirMap[dir].TotalLines += fMeta.LineCount

		ext := strings.ToLower(filepath.Ext(relPath))
		if ext == "" { ext = "[no ext]" }
		if _, ok := stats.ExtMap[ext]; !ok { stats.ExtMap[ext] = &ExtStats{Ext: ext} }
		stats.ExtMap[ext].FileCount++
		stats.ExtMap[ext].TotalSize += fMeta.Size
		stats.ExtMap[ext].TotalLines += fMeta.LineCount

		return nil
	})

	return files, stats, skipped
}

func shouldIgnoreDir(relPath string, config Config, gitPatterns []string) bool {
	name := filepath.Base(relPath)
	if !config.NoDefaultIgnore && ignoreDirs[name] { return true }
	for _, p := range config.AdditionalIgnores { if matchPattern(relPath, p) { return true } }
	for _, p := range gitPatterns { if matchPattern(relPath, p) { return true } }
	return false
}

func shouldIgnoreFile(relPath string, config Config, gitPatterns []string) bool {
	name := filepath.Base(relPath)
	ext := filepath.Ext(relPath)
	if !config.NoDefaultIgnore && ignoreFiles[name] { return true }
	if len(config.IncludeExts) > 0 {
		match := false
		for _, e := range config.IncludeExts { if strings.EqualFold(ext, e) { match = true; break } }
		if !match { return true }
	}
	if len(config.IncludeMatches) > 0 {
		match := false
		for _, m := range config.IncludeMatches { if strings.Contains(relPath, m) { match = true; break } }
		if !match { return true }
	}
	for _, e := range config.ExcludeExts { if strings.EqualFold(ext, e) { return true } }
	for _, m := range config.ExcludeMatches { if strings.Contains(relPath, m) { return true } }
	for _, p := range gitPatterns { if matchPattern(relPath, p) { return true } }
	return false
}

func loadGitignore(root string) []string {
	return loadIgnoreFile(filepath.Join(root, ".gitignore"))
}

func loadGdignore(root string) []string {
	return loadIgnoreFile(filepath.Join(root, ".gdignore"))
}

func loadIgnoreFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil { return nil }
	var res []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		l := strings.TrimSpace(scanner.Text())
		if l != "" && !strings.HasPrefix(l, "#") { res = append(res, l) }
	}
	return res
}

func matchPattern(path, pattern string) bool {
	// Pattern without path separator: match against basename only
	if !strings.ContainsAny(pattern, "/\\") {
		m, _ := filepath.Match(pattern, filepath.Base(path))
		return m
	}
	// Pattern with path separator: match against full relative path
	m, _ := filepath.Match(pattern, path)
	return m
}

func isBinaryFile(path string) bool {
	f, _ := os.Open(path)
	defer f.Close()
	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	if n == 0 { return false }
	if !utf8.Valid(buf[:n]) {
		for _, b := range buf[:n] { if b == 0 { return true } }
	}
	return false
}

func isKnownTextFile(relPath string) bool {
	name := filepath.Base(relPath)
	if knownTextFiles[name] { return true }
	ext := strings.ToLower(filepath.Ext(relPath))
	_, ok := languageMap[ext]
	return ok
}

func countLines(path string) int {
	f, _ := os.Open(path)
	defer f.Close()
	s := bufio.NewScanner(f)
	n := 0
	for s.Scan() { n++ }
	return n
}

// estimateTokens provides a rough token count estimate for LLM context sizing.
// Uses the heuristic of ~1 token per 4 characters (approximates cl100k_base).
func estimateTokens(path string) int {
	data, err := os.ReadFile(path)
	if err != nil { return 0 }
	return len(data) / 4
}

// ============================================================
//  Statistics & Output
// ============================================================

func printStatsTerminal(files []FileMetadata, stats Stats) {
	fmt.Printf("\n📊 Project Statistics Summary\n")
	fmt.Printf("  %-20s %d\n", "Files Processed:", stats.FileCount)
	fmt.Printf("  %-20s %d\n", "Total Lines:", stats.TotalLines)
	fmt.Printf("  %-20s %.2f KB\n", "Total Size:", float64(stats.TotalSize)/1024)
	fmt.Printf("  %-20s %d\n", "Directories:", stats.DirCount)
	fmt.Printf("  %-20s ~%d tokens\n", "Est. Tokens:", stats.TotalTokens)

	// 1. Top Files by Line Count
	sort.Slice(files, func(i, j int) bool { return files[i].LineCount > files[j].LineCount })
	fmt.Printf("\n%-45s %12s %12s\n", "🔝 Top Files (by Lines):", "Lines", "Size")
	fmt.Printf("%-45s %12s %12s\n", "---------------------------------------------", "-----", "----")
	for i := 0; i < len(files) && i < 5; i++ {
		fmt.Printf("%-45s %12d %11.2f KB\n", files[i].RelPath, files[i].LineCount, float64(files[i].Size)/1024)
	}

	// 2. Directory Dimension
	dirs := make([]*DirStats, 0, len(stats.DirMap))
	for _, d := range stats.DirMap { dirs = append(dirs, d) }
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].TotalLines > dirs[j].TotalLines })
	fmt.Printf("\n%-30s %8s %12s %10s %12s %10s\n", "📁 Folder Dimension:", "Files", "Lines", "Lines %", "Size", "Size %")
	fmt.Printf("%-30s %8s %12s %10s %12s %10s\n", "------------------------------", "-------", "-----------", "-------", "-----------", "-------")
	for i := 0; i < len(dirs) && i < 10; i++ {
		linePct := 0.0
		if stats.TotalLines > 0 { linePct = float64(dirs[i].TotalLines) / float64(stats.TotalLines) * 100 }
		sizePct := 0.0
		if stats.TotalSize > 0 { sizePct = float64(dirs[i].TotalSize) / float64(stats.TotalSize) * 100 }
		fmt.Printf("%-30s %8d %12d %9.1f%% %11.2f KB %9.1f%%\n", 
			dirs[i].Path, dirs[i].FileCount, dirs[i].TotalLines, linePct, float64(dirs[i].TotalSize)/1024, sizePct)
	}

	// 3. Language Dimension
	exts := make([]*ExtStats, 0, len(stats.ExtMap))
	for _, e := range stats.ExtMap { exts = append(exts, e) }
	sort.Slice(exts, func(i, j int) bool { return exts[i].TotalLines > exts[j].TotalLines })
	fmt.Printf("\n%-15s %8s %12s %10s %12s %10s\n", "📝 Language:", "Files", "Lines", "Lines %", "Size", "Size %")
	fmt.Printf("%-15s %8s %12s %10s %12s %10s\n", "---------------", "-------", "-----------", "-------", "-----------", "-------")
	for _, e := range exts {
		linePct := 0.0
		if stats.TotalLines > 0 { linePct = float64(e.TotalLines) / float64(stats.TotalLines) * 100 }
		sizePct := 0.0
		if stats.TotalSize > 0 { sizePct = float64(e.TotalSize) / float64(stats.TotalSize) * 100 }
		fmt.Printf("%-15s %8d %12d %9.1f%% %11.2f KB %9.1f%%\n", 
			e.Ext, e.FileCount, e.TotalLines, linePct, float64(e.TotalSize)/1024, sizePct)
	}
}

func generateStatsContent(config Config, files []FileMetadata, stats Stats) string {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	fmt.Fprintf(w, "# Project Statistics: %s\n\n", filepath.Base(config.RootDir))
	fmt.Fprintf(w, "> Generated by [Sourcepack](https://github.com/yuanguangshan/sourcepack) on %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Overview
	fmt.Fprintln(w, "## Overview")
	fmt.Fprintf(w, "- Files: %d\n- Lines: %d\n- Size: %.2f KB\n- Directories: %d\n- Est. Tokens: ~%d\n\n", stats.FileCount, stats.TotalLines, float64(stats.TotalSize)/1024, stats.DirCount, stats.TotalTokens)

	// Top Files
	sort.Slice(files, func(i, j int) bool { return files[i].LineCount > files[j].LineCount })
	fmt.Fprintln(w, "## Top Files (by Lines)")
	fmt.Fprintln(w, "| File | Lines | Size |")
	fmt.Fprintln(w, "| :--- | ---: | ---: |")
	for i := 0; i < len(files) && i < 5; i++ {
		fmt.Fprintf(w, "| %s | %d | %.2f KB |\n", files[i].RelPath, files[i].LineCount, float64(files[i].Size)/1024)
	}
	fmt.Fprintln(w)

	// Directory Dimension
	dirs := make([]*DirStats, 0, len(stats.DirMap))
	for _, d := range stats.DirMap { dirs = append(dirs, d) }
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].TotalLines > dirs[j].TotalLines })
	fmt.Fprintln(w, "## Directory Distribution")
	fmt.Fprintln(w, "| Directory | Files | Lines | Lines % | Size | Size % |")
	fmt.Fprintln(w, "| :--- | ---: | ---: | ---: | ---: | ---: |")
	for i := 0; i < len(dirs) && i < 10; i++ {
		linePct := 0.0
		if stats.TotalLines > 0 { linePct = float64(dirs[i].TotalLines) / float64(stats.TotalLines) * 100 }
		sizePct := 0.0
		if stats.TotalSize > 0 { sizePct = float64(dirs[i].TotalSize) / float64(stats.TotalSize) * 100 }
		fmt.Fprintf(w, "| %s | %d | %d | %.1f%% | %.2f KB | %.1f%% |\n", dirs[i].Path, dirs[i].FileCount, dirs[i].TotalLines, linePct, float64(dirs[i].TotalSize)/1024, sizePct)
	}
	fmt.Fprintln(w)

	// Language Dimension
	exts := make([]*ExtStats, 0, len(stats.ExtMap))
	for _, e := range stats.ExtMap { exts = append(exts, e) }
	sort.Slice(exts, func(i, j int) bool { return exts[i].TotalLines > exts[j].TotalLines })
	fmt.Fprintln(w, "## Language Breakdown")
	fmt.Fprintln(w, "| Extension | Files | Lines | Lines % | Size | Size % |")
	fmt.Fprintln(w, "| :--- | ---: | ---: | ---: | ---: | ---: |")
	for _, e := range exts {
		linePct := 0.0
		if stats.TotalLines > 0 { linePct = float64(e.TotalLines) / float64(stats.TotalLines) * 100 }
		sizePct := 0.0
		if stats.TotalSize > 0 { sizePct = float64(e.TotalSize) / float64(stats.TotalSize) * 100 }
		fmt.Fprintf(w, "| %s | %d | %d | %.1f%% | %.2f KB | %.1f%% |\n", e.Ext, e.FileCount, e.TotalLines, linePct, float64(e.TotalSize)/1024, sizePct)
	}

	w.Flush()
	return buf.String()
}

func generateContent(config Config, files []FileMetadata, stats Stats) (string, error) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	fmt.Fprintf(w, "# Project Snapshot: %s\n\n", filepath.Base(config.RootDir))
	fmt.Fprintf(w, "> Generated by [Sourcepack](https://github.com/yuanguangshan/sourcepack) on %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Project Structure Tree
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	tree := buildTreeString(files, filepath.Base(config.RootDir))
	fmt.Fprintln(w, "## Project Structure")
	fmt.Fprintln(w, "```")
	fmt.Fprint(w, tree)
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Table of Contents")
	for _, f := range files {
		fmt.Fprintf(w, "- [%s](#%s)\n", f.RelPath, generateAnchor(f.RelPath))
	}
	fmt.Fprintln(w, "\n---")

	for _, f := range files {
		fmt.Fprintf(w, "\n## %s\n\n", f.RelPath)
		c, _ := os.ReadFile(f.FullPath)
		lang := detectLanguage(f.RelPath)
		fence := determineFence(string(c))
		fmt.Fprintf(w, "%s%s\n%s%s\n%s\n", fence, lang, string(c), func()string{if len(c)>0 && c[len(c)-1]!='\n'{return "\n"} ; return ""}(), fence)
	}

	if config.ShowStats {
		fmt.Fprintln(w, "\n---")
		fmt.Fprintln(w, "## Detailed Project Audit")
		
		fmt.Fprintln(w, "### 📈 Overview")
		fmt.Fprintf(w, "- Files: %d\n- Lines: %d\n- Size: %.2f KB\n- Est. Tokens: ~%d\n\n", stats.FileCount, stats.TotalLines, float64(stats.TotalSize)/1024, stats.TotalTokens)

		fmt.Fprintln(w, "### 📁 Directory Distribution")
		fmt.Fprintln(w, "| Directory | Files | Lines | Lines % | Size | Size % |")
		fmt.Fprintln(w, "| :--- | :---: | :---: | :---: | :---: | :---: |")
		dirs := make([]*DirStats, 0, len(stats.DirMap))
		for _, d := range stats.DirMap { dirs = append(dirs, d) }
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
		for _, d := range dirs {
			linePct := 0.0
			if stats.TotalLines > 0 { linePct = float64(d.TotalLines) / float64(stats.TotalLines) * 100 }
			sizePct := 0.0
			if stats.TotalSize > 0 { sizePct = float64(d.TotalSize) / float64(stats.TotalSize) * 100 }
			fmt.Fprintf(w, "| %s | %d | %d | %.1f%% | %.2f KB | %.1f%% |\n", d.Path, d.FileCount, d.TotalLines, linePct, float64(d.TotalSize)/1024, sizePct)
		}

		fmt.Fprintln(w, "\n### 📝 Language Breakdown")
		fmt.Fprintln(w, "| Extension | Files | Lines | Lines % | Size | Size % |")
		fmt.Fprintln(w, "| :--- | :---: | :---: | :---: | :---: | :---: |")
		exts := make([]*ExtStats, 0, len(stats.ExtMap))
		for _, e := range stats.ExtMap { exts = append(exts, e) }
		sort.Slice(exts, func(i, j int) bool { return exts[i].TotalLines > exts[j].TotalLines })
		for _, e := range exts {
			linePct := 0.0
			if stats.TotalLines > 0 { linePct = float64(e.TotalLines) / float64(stats.TotalLines) * 100 }
			sizePct := 0.0
			if stats.TotalSize > 0 { sizePct = float64(e.TotalSize) / float64(stats.TotalSize) * 100 }
			fmt.Fprintf(w, "| %s | %d | %d | %.1f%% | %.2f KB | %.1f%% |\n", e.Ext, e.FileCount, e.TotalLines, linePct, float64(e.TotalSize)/1024, sizePct)
		}
	}

	w.Flush()
	return buf.String(), nil
}

func generateAnchor(p string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ".", "-").Replace(strings.ToLower(p))
}

func detectLanguage(p string) string {
	ext := strings.ToLower(filepath.Ext(p))
	if l, ok := languageMap[ext]; ok { return l }
	base := strings.ToLower(filepath.Base(p))
	if base == "dockerfile" || base == "makefile" { return base }
	return ""
}

func determineFence(c string) string {
	max := 0
	cur := 0
	for _, r := range c {
		if r == '`' { cur++; if cur > max { max = cur } } else { cur = 0 }
	}
	if max < 3 { return "```" }
	return strings.Repeat("`", max+1)
}

func printDryRun(files []FileMetadata, stats Stats, skipped []SkippedFile) {
	fmt.Printf("\n🔍 Files to be included (%d):\n", len(files))
	for _, f := range files { fmt.Printf("  - %-40s (%d lines)\n", f.RelPath, f.LineCount) }
	if len(skipped) > 0 {
		fmt.Printf("\n⏭  Skipped:\n")
		for _, s := range skipped { fmt.Printf("  - %-40s [%s]\n", s.RelPath, s.Reason) }
	}
}

func printConfigSummary(c Config) {
	fmt.Println("⚙  Configuration:")
	fmt.Printf("  %-20s %s\n", "Root:", c.RootDir)
	fmt.Printf("  %-20s %s\n", "Output:", c.OutputFile)
	fmt.Printf("  %-20s %d KB\n", "Max file size:", c.MaxFileSize/1024)
	if len(c.IncludeExts) > 0 { fmt.Printf("  %-20s %v\n", "Include exts:", c.IncludeExts) }
	if len(c.ExcludeExts) > 0 { fmt.Printf("  %-20s %v\n", "Exclude exts:", c.ExcludeExts) }
	if len(c.IncludeMatches) > 0 { fmt.Printf("  %-20s %v\n", "Include matches:", c.IncludeMatches) }
	if len(c.ExcludeMatches) > 0 { fmt.Printf("  %-20s %v\n", "Exclude matches:", c.ExcludeMatches) }
	if len(c.AdditionalIgnores) > 0 { fmt.Printf("  %-20s %v\n", "Extra ignores:", c.AdditionalIgnores) }
	fmt.Printf("  %-20s %v\n", "No subdirs:", c.NoSubdirs)
	fmt.Printf("  %-20s %v\n", "No default ignore:", c.NoDefaultIgnore)
	fmt.Printf("  %-20s %v\n", "No .gitignore:", c.NoGitignore)
	fmt.Printf("  %-20s %v\n", "Copy to clipboard:", c.Copy)
	fmt.Printf("  %-20s %v\n", "Push to remote:", c.Push)
	if c.Push {
		fmt.Printf("  %-20s %s\n", "Push URL:", c.PushURL)
	}
	fmt.Println()
}

// ============================================================
//  Tree Structure Generation
// ============================================================

type treeNode struct {
	children  map[string]*treeNode
	lineCount int // only set for files
}

func buildTreeString(files []FileMetadata, rootName string) string {
	root := &treeNode{children: make(map[string]*treeNode)}
	for _, f := range files {
		parts := strings.Split(f.RelPath, string(filepath.Separator))
		node := root
		for _, part := range parts {
			if _, ok := node.children[part]; !ok {
				node.children[part] = &treeNode{children: make(map[string]*treeNode)}
			}
			node = node.children[part]
		}
		node.lineCount = f.LineCount
	}
	var sb strings.Builder
	sb.WriteString(rootName + "/\n")
	formatTree(&sb, root, "")
	return sb.String()
}

func formatTree(sb *strings.Builder, node *treeNode, prefix string) {
	// Sort: directories first, then files, alphabetically within each group
	names := make([]string, 0, len(node.children))
	for name := range node.children {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		iDir := len(node.children[names[i]].children) > 0
		jDir := len(node.children[names[j]].children) > 0
		if iDir != jDir { return iDir }
		return names[i] < names[j]
	})

	for i, name := range names {
		child := node.children[name]
		isDir := len(child.children) > 0
		isLast := i == len(names)-1
		connector := "├── "
		newPrefix := "│   "
		if isLast {
			connector = "└── "
			newPrefix = "    "
		}
		sb.WriteString(prefix + connector + name)
		if isDir {
			sb.WriteString("/")
		} else {
			sb.WriteString(fmt.Sprintf("  (%d lines)", child.lineCount))
		}
		sb.WriteString("\n")
		formatTree(sb, child, prefix+newPrefix)
	}
}

// ============================================================
//  Clipboard Support
// ============================================================

func copyToClipboard(content string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("unsupported platform for clipboard: %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

// ============================================================
//  Remote Push Support
// ============================================================

func pushToRemote(content, url, authKey string) error {
	body, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authKey != "" {
		req.Header.Set("X-Auth-Key", authKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
