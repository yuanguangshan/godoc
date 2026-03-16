package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

/*
====================================================
 Configuration & Types
====================================================
*/

const versionStr = "v3.0.0"

// Config 集中管理所有运行时配置
type Config struct {
	RootDir           string
	OutputFile        string
	IncludeExts       []string
	IncludeMatches    []string
	ExcludeExts       []string
	ExcludeMatches    []string
	ExtraIgnores      []string // --ignore 额外忽略模式
	GitignorePatterns []string // 从 .gitignore 加载的模式
	NoDefaultIgnore   bool     // --no-default-ignore
	NoGitignore       bool     // --no-gitignore
	MaxFileSize       int64
	NoSubdirs         bool
	DryRun            bool
	Verbose           bool
	Version           bool
}

// FileMetadata 仅存储元数据；LineCount 在写入阶段填充，避免二次读取文件
type FileMetadata struct {
	RelPath   string
	FullPath  string
	Size      int64
	LineCount int
}

// Stats 统计信息
type Stats struct {
	PotentialMatches   int
	ExplicitlyExcluded int
	FileCount          int
	TotalSize          int64
	TotalLines         int
	Skipped            int
}

var defaultIgnorePatterns = []string{
	".git", ".idea", ".vscode",
	"node_modules", "vendor", "dist", "build", "target", "bin",
	"__pycache__", ".DS_Store",
	"package-lock.json", "yarn.lock", "go.sum",
}

var languageMap = map[string]string{
	".go":         "go",
	".js":         "javascript",
	".ts":         "typescript",
	".tsx":        "typescript",
	".jsx":        "javascript",
	".py":         "python",
	".java":       "java",
	".c":          "c",
	".cpp":        "cpp",
	".cc":         "cpp",
	".cxx":        "cpp",
	".h":          "c",
	".hpp":        "cpp",
	".rs":         "rust",
	".rb":         "ruby",
	".php":        "php",
	".cs":         "csharp",
	".swift":      "swift",
	".kt":         "kotlin",
	".scala":      "scala",
	".r":          "r",
	".sql":        "sql",
	".sh":         "bash",
	".bash":       "bash",
	".zsh":        "bash",
	".fish":       "fish",
	".ps1":        "powershell",
	".md":         "markdown",
	".html":       "html",
	".htm":        "html",
	".css":        "css",
	".scss":       "scss",
	".sass":       "sass",
	".less":       "less",
	".xml":        "xml",
	".json":       "json",
	".yaml":       "yaml",
	".yml":        "yaml",
	".toml":       "toml",
	".ini":        "ini",
	".conf":       "conf",
	".txt":        "text",
	".vue":        "vue",
	".svelte":     "svelte",
	".dart":       "dart",
	".lua":        "lua",
	".pl":         "perl",
	".pm":         "perl",
	".ex":         "elixir",
	".exs":        "elixir",
	".erl":        "erlang",
	".hs":         "haskell",
	".ml":         "ocaml",
	".clj":        "clojure",
	".proto":      "protobuf",
	".graphql":    "graphql",
	".gql":        "graphql",
	".dockerfile": "dockerfile",
	".tf":         "hcl",
	".hcl":        "hcl",
}

/*
====================================================
 Main Entry
====================================================
*/

func main() {
	cfg := parseFlags()

	// 加载 .gitignore 模式
	if !cfg.NoGitignore {
		cfg.GitignorePatterns = loadGitignorePatterns(cfg.RootDir)
	}

	printStartupInfo(cfg)

	// Phase 1: 扫描文件结构（不读取文件内容，不计算行数）
	fmt.Println("⏳ 正在扫描文件结构...")
	files, stats, err := scanDirectory(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 扫描失败: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("⚠️  没有找到符合条件的文件")
		printSummary(stats, cfg.OutputFile)
		return
	}

	// Dry Run 模式：仅预览
	if cfg.DryRun {
		printDryRun(files, stats)
		return
	}

	// Phase 2: 单次读取 + 临时文件 → 生成最终 Markdown
	fmt.Printf("💾 正在写入文档 [文件数: %d]...\n", len(files))
	if err := writeMarkdown(cfg, files, &stats); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 写入失败: %v\n", err)
		os.Exit(1)
	}

	printSummary(stats, cfg.OutputFile)
}

/*
====================================================
 Flag Parsing
====================================================
*/

func parseFlags() Config {
	var cfg Config
	var include, match, exclude, excludeMatch, ignoreStr string
	var maxKB int64

	flag.StringVar(&cfg.RootDir, "dir", ".", "Root directory to scan")
	flag.StringVar(&cfg.OutputFile, "o", "", "Output markdown file")
	flag.StringVar(&include, "i", "", "Include extensions (e.g. go,js)")
	flag.StringVar(&match, "m", "", "Include path keywords (e.g. _test.go)")
	flag.StringVar(&exclude, "x", "", "Exclude extensions (e.g. exe,o)")
	flag.StringVar(&excludeMatch, "xm", "", "Exclude path keywords (e.g. vendor/)")
	flag.StringVar(&ignoreStr, "ignore", "", "Extra ignore patterns (comma-separated)")
	flag.BoolVar(&cfg.NoDefaultIgnore, "no-default-ignore", false, "Disable default ignore patterns")
	flag.BoolVar(&cfg.NoGitignore, "no-gitignore", false, "Do not load .gitignore patterns")
	flag.Int64Var(&maxKB, "max-size", 500, "Max file size in KB")
	flag.BoolVar(&cfg.NoSubdirs, "no-subdirs", false, "Do not scan subdirectories")
	flag.BoolVar(&cfg.NoSubdirs, "ns", false, "Alias for --no-subdirs")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Preview matched files without writing")
	flag.BoolVar(&cfg.Verbose, "v", false, "Verbose output")
	flag.BoolVar(&cfg.Version, "version", false, "Show version")

	flag.Parse()

	if cfg.Version {
		fmt.Printf("gen-docs %s\n", versionStr)
		os.Exit(0)
	}

	// 支持位置参数
	if args := flag.Args(); len(args) > 0 {
		cfg.RootDir = args[0]
	}

	// 自动生成输出文件名
	if cfg.OutputFile == "" {
		cfg.OutputFile = generateOutputName(cfg.RootDir)
	}

	cfg.IncludeExts = normalizeExts(include)
	cfg.IncludeMatches = splitAndTrim(match)
	cfg.ExcludeExts = normalizeExts(exclude)
	cfg.ExcludeMatches = splitAndTrim(excludeMatch)
	cfg.ExtraIgnores = splitAndTrim(ignoreStr)
	cfg.MaxFileSize = maxKB * 1024

	return cfg
}

func generateOutputName(rootDir string) string {
	baseName := "project"
	cleanRoot := filepath.Clean(rootDir)

	if cleanRoot == "." || cleanRoot == string(filepath.Separator) {
		if abs, err := filepath.Abs(cleanRoot); err == nil {
			baseName = filepath.Base(abs)
		}
	} else {
		baseName = cleanRoot
		baseName = strings.ReplaceAll(baseName, string(filepath.Separator), "_")
		baseName = strings.ReplaceAll(baseName, ".", "_")
		for strings.Contains(baseName, "__") {
			baseName = strings.ReplaceAll(baseName, "__", "_")
		}
		baseName = strings.Trim(baseName, "_")
	}

	date := time.Now().Format("20060102")
	return fmt.Sprintf("%s-%s-docs.md", baseName, date)
}

/*
====================================================
 .gitignore Integration
====================================================
*/

// loadGitignorePatterns 从项目根目录的 .gitignore 中加载忽略模式。
// 支持基础模式：纯文件名、目录名/、通配符 *、根路径前缀 /。
// 不支持：**（globstar）、!（取反）。
func loadGitignorePatterns(rootDir string) []string {
	gitignorePath := filepath.Join(rootDir, ".gitignore")
	f, err := os.Open(gitignorePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

func matchesGitignorePattern(pattern, relPath, baseName string, isDir bool) bool {
	dirOnly := strings.HasSuffix(pattern, "/")
	if dirOnly && !isDir {
		return false
	}

	p := strings.TrimSuffix(pattern, "/")
	rootOnly := strings.HasPrefix(p, "/")
	p = strings.TrimPrefix(p, "/")

	if strings.Contains(p, "/") || rootOnly {
		// 路径模式：匹配相对路径
		matched, _ := filepath.Match(p, relPath)
		return matched
	}

	// 名称模式：匹配 basename
	matched, _ := filepath.Match(p, baseName)
	return matched
}

func matchesAnyGitignorePattern(patterns []string, relPath, baseName string, isDir bool) bool {
	for _, p := range patterns {
		if matchesGitignorePattern(p, relPath, baseName, isDir) {
			return true
		}
	}
	return false
}

/*
====================================================
 Startup, Summary & Dry Run
====================================================
*/

func printStartupInfo(cfg Config) {
	fmt.Println("▶ Gen-Docs Started")
	fmt.Printf("  Root: %s\n", cfg.RootDir)
	if cfg.DryRun {
		fmt.Println("  Mode: Dry Run (preview only)")
	} else {
		fmt.Printf("  Out : %s\n", cfg.OutputFile)
	}
	fmt.Printf("  Max : %d KB\n", cfg.MaxFileSize/1024)
	if len(cfg.IncludeExts) > 0 {
		fmt.Printf("  Only Ext: %v\n", cfg.IncludeExts)
	}
	if len(cfg.IncludeMatches) > 0 {
		fmt.Printf("  Match   : %v\n", cfg.IncludeMatches)
	}
	if len(cfg.ExcludeExts) > 0 {
		fmt.Printf("  Skip Ext: %v\n", cfg.ExcludeExts)
	}
	if len(cfg.ExcludeMatches) > 0 {
		fmt.Printf("  Skip Key: %v\n", cfg.ExcludeMatches)
	}
	if cfg.NoDefaultIgnore {
		fmt.Println("  ⚠ Default ignore patterns DISABLED")
	}
	if len(cfg.ExtraIgnores) > 0 {
		fmt.Printf("  Extra Ignore: %v\n", cfg.ExtraIgnores)
	}
	if len(cfg.GitignorePatterns) > 0 {
		fmt.Printf("  .gitignore  : %d patterns loaded\n", len(cfg.GitignorePatterns))
	}
	if cfg.NoGitignore {
		fmt.Println("  .gitignore  : DISABLED")
	}
	fmt.Println()
}

func printSummary(stats Stats, output string) {
	fmt.Println("\n✔ 完成!")
	fmt.Printf("  符合包含规则 (Potential) : %d\n", stats.PotentialMatches)
	fmt.Printf("  由排除规则移除 (Excluded): %d\n", stats.ExplicitlyExcluded)
	fmt.Printf("  最终写入文件数 (Final)   : %d\n", stats.FileCount)
	fmt.Printf("  总行数 (Total Lines)     : %d\n", stats.TotalLines)
	fmt.Printf("  总物理大小 (Total Size)  : %.2f KB\n", float64(stats.TotalSize)/1024)
	fmt.Printf("  无需处理的无关文件        : %d\n", stats.Skipped)
	fmt.Printf("  输出路径                 : %s\n", output)
}

func printDryRun(files []FileMetadata, stats Stats) {
	fmt.Println("\n📋 预览模式 (Dry Run) — 以下文件将被包含：")
	fmt.Println()

	var totalSize int64
	for i, f := range files {
		fmt.Printf("  %3d. %-60s %8.2f KB\n", i+1, f.RelPath, float64(f.Size)/1024)
		totalSize += f.Size
	}

	fmt.Println()
	fmt.Printf("  符合包含规则 (Potential) : %d\n", stats.PotentialMatches)
	fmt.Printf("  由排除规则移除 (Excluded): %d\n", stats.ExplicitlyExcluded)
	fmt.Printf("  最终文件数 (Final)       : %d\n", stats.FileCount)
	fmt.Printf("  预估总大小               : %.2f KB\n", float64(totalSize)/1024)
	fmt.Printf("  无关文件                 : %d\n", stats.Skipped)
	fmt.Println()
	fmt.Println("💡 去掉 --dry-run 参数即可生成文档")
}

/*
====================================================
 Directory Scanning (Phase 1: Metadata Only)
====================================================
*/

func scanDirectory(cfg Config) ([]FileMetadata, Stats, error) {
	var files []FileMetadata
	var stats Stats

	absOutput, err := filepath.Abs(cfg.OutputFile)
	if err != nil {
		logf(cfg.Verbose, "⚠ 无法解析输出文件绝对路径: %v", err)
	}

	ignorePatterns := buildIgnorePatterns(cfg)
	skipDotDirs := !cfg.NoDefaultIgnore

	err = filepath.WalkDir(cfg.RootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logf(cfg.Verbose, "⚠ 无法访问: %s (%v)", path, err)
			stats.Skipped++
			return nil
		}

		relPath, relErr := filepath.Rel(cfg.RootDir, path)
		if relErr != nil {
			logf(cfg.Verbose, "⚠ 无法计算相对路径: %s (%v)", path, relErr)
			stats.Skipped++
			return nil
		}
		if relPath == "." {
			return nil
		}

		// ---- 目录处理 ----
		if d.IsDir() {
			if cfg.NoSubdirs {
				return filepath.SkipDir
			}
			if shouldIgnoreDir(d.Name(), relPath, ignorePatterns, cfg.GitignorePatterns, skipDotDirs) {
				logf(cfg.Verbose, "⊘ 跳过目录: %s", relPath)
				return filepath.SkipDir
			}
			return nil
		}

		// ---- 排除输出文件自身 ----
		if absPath, err := filepath.Abs(path); err == nil && absPath == absOutput {
			return nil
		}

		// ---- 获取文件信息 ----
		info, err := d.Info()
		if err != nil {
			logf(cfg.Verbose, "⚠ 无法获取文件信息: %s (%v)", relPath, err)
			stats.Skipped++
			return nil
		}

		// ---- 基础过滤：大小 ----
		if info.Size() > cfg.MaxFileSize {
			logf(cfg.Verbose, "⊘ 文件过大: %s (%d KB)", relPath, info.Size()/1024)
			stats.Skipped++
			return nil
		}

		// ---- 基础过滤：二进制 ----
		if isBinaryFile(path) {
			logf(cfg.Verbose, "⊘ 二进制文件: %s", relPath)
			stats.Skipped++
			return nil
		}

		// ---- .gitignore 文件级过滤 ----
		if matchesAnyGitignorePattern(cfg.GitignorePatterns, relPath, d.Name(), false) {
			logf(cfg.Verbose, "⊘ .gitignore 排除: %s", relPath)
			stats.Skipped++
			return nil
		}

		// ---- 默认忽略模式（文件名级别，如 .DS_Store, package-lock.json）----
		if !cfg.NoDefaultIgnore {
			baseName := d.Name()
			ignored := false
			for _, p := range ignorePatterns {
				if baseName == p {
					ignored = true
					break
				}
			}
			if ignored {
				logf(cfg.Verbose, "⊘ 默认忽略: %s", relPath)
				stats.Skipped++
				return nil
			}
		}

		// ---- 包含规则（白名单）：-i 和 -m 取 AND ----
		if len(cfg.IncludeExts) > 0 || len(cfg.IncludeMatches) > 0 {
			extMatched := len(cfg.IncludeExts) == 0
			if !extMatched {
				ext := strings.ToLower(filepath.Ext(relPath))
				for _, e := range cfg.IncludeExts {
					if ext == e {
						extMatched = true
						break
					}
				}
			}

			pathMatched := len(cfg.IncludeMatches) == 0
			if !pathMatched {
				for _, m := range cfg.IncludeMatches {
					if strings.Contains(relPath, m) {
						pathMatched = true
						break
					}
				}
			}

			if !extMatched || !pathMatched {
				stats.Skipped++
				return nil
			}
		}

		// ---- 符合包含意图 (Potential Match) ----
		stats.PotentialMatches++

		// ---- 排除规则（黑名单）：-x 和 -xm ----
		ext := strings.ToLower(filepath.Ext(relPath))
		for _, e := range cfg.ExcludeExts {
			if ext == e {
				stats.ExplicitlyExcluded++
				logf(cfg.Verbose, "⊘ 排除后缀: %s", relPath)
				return nil
			}
		}
		for _, m := range cfg.ExcludeMatches {
			if strings.Contains(relPath, m) {
				stats.ExplicitlyExcluded++
				logf(cfg.Verbose, "⊘ 排除关键字 [%s]: %s", m, relPath)
				return nil
			}
		}

		// ---- 最终通过（行数在写入阶段统计）----
		files = append(files, FileMetadata{
			RelPath:  relPath,
			FullPath: path,
			Size:     info.Size(),
		})
		stats.FileCount++
		stats.TotalSize += info.Size()

		logf(cfg.Verbose, "✓ 添加: %s (%.2f KB)", relPath, float64(info.Size())/1024)
		return nil
	})

	// 排序保证输出一致性
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	return files, stats, err
}

func buildIgnorePatterns(cfg Config) []string {
	var patterns []string
	if !cfg.NoDefaultIgnore {
		patterns = append(patterns, defaultIgnorePatterns...)
	}
	patterns = append(patterns, cfg.ExtraIgnores...)
	return patterns
}

func shouldIgnoreDir(name, relPath string, ignorePatterns, gitignorePatterns []string, skipDotDirs bool) bool {
	// 跳过隐藏目录
	if skipDotDirs && strings.HasPrefix(name, ".") && name != "." {
		return true
	}

	// 检查合并后的忽略模式
	for _, p := range ignorePatterns {
		if name == p {
			return true
		}
	}

	// 检查 .gitignore 模式
	return matchesAnyGitignorePattern(gitignorePatterns, relPath, name, true)
}

/*
====================================================
 File Utilities
====================================================
*/

func normalizeExts(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var exts []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		if !strings.HasPrefix(p, ".") {
			p = "." + p
		}
		exts = append(exts, p)
	}
	return exts
}

func splitAndTrim(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// isBinaryFile 通过 NULL 字节检测和 UTF-8 有效性判断文件是否为二进制。
// 不再将 .min.* 文件硬编码为二进制——minified 文件是合法文本文件。
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return true
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	buf = buf[:n]

	// NULL 字节检测
	for _, b := range buf {
		if b == 0 {
			return true
		}
	}

	// UTF-8 有效性检测
	return !utf8.Valid(buf)
}

func detectLanguage(path string) string {
	// 优先检查特殊文件名（无扩展名的常见文件）
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "dockerfile":
		return "dockerfile"
	case "makefile", "gnumakefile":
		return "makefile"
	case ".gitignore", ".dockerignore", ".editorconfig":
		return "text"
	}

	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := languageMap[ext]; ok {
		return lang
	}
	return "text"
}

/*
====================================================
 Markdown Output (Single-Read, Temp-File Approach)
====================================================

 策略：
   1. 将每个源文件读取一次，写入临时文件；同时统计行数并生成动态 fence。
   2. 组装最终输出：Header → TOC（此时行数已知）→ 内容（从临时文件复制）→ Footer。
 好处：每个源文件只读取一次，消除了旧版本扫描+写入两次读取的 I/O 浪费。
*/

func writeMarkdown(cfg Config, files []FileMetadata, stats *Stats) error {
	// ---- Phase 1: 写入格式化内容到临时文件，同时收集行数 ----
	tmpFile, err := os.CreateTemp("", "gen-docs-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tw := bufio.NewWriterSize(tmpFile, 64*1024)
	total := len(files)
	failedIndices := make(map[int]bool)

	for i := range files {
		if !cfg.Verbose && (i%10 == 0 || i == total-1) {
			fmt.Printf("\r🚀 处理进度: %d/%d (%.1f%%)", i+1, total, float64(i+1)/float64(total)*100)
		}

		content, err := os.ReadFile(files[i].FullPath)
		if err != nil {
			logf(true, "\n⚠ 读取失败 %s: %v", files[i].RelPath, err)
			failedIndices[i] = true
			stats.FileCount--
			stats.TotalSize -= files[i].Size
			continue
		}

		lineCount := countLinesInContent(content)
		files[i].LineCount = lineCount
		stats.TotalLines += lineCount

		fence := determineFence(content)
		lang := detectLanguage(files[i].RelPath)

		fmt.Fprintln(tw)
		fmt.Fprintf(tw, "## 📄 %s\n\n", files[i].RelPath)
		fmt.Fprintf(tw, "%s%s\n", fence, lang)
		tw.Write(content)
		if len(content) > 0 && content[len(content)-1] != '\n' {
			fmt.Fprintln(tw)
		}
		fmt.Fprintln(tw, fence)
	}
	fmt.Println()

	if err := tw.Flush(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}
	tmpFile.Close()

	// ---- Phase 2: 组装最终输出 ----
	outFile, err := os.Create(cfg.OutputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer outFile.Close()

	w := bufio.NewWriterSize(outFile, 64*1024)

	// Header
	fmt.Fprintln(w, "# Project Documentation")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- **Generated at:** %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "- **Root Dir:** `%s`\n", cfg.RootDir)
	fmt.Fprintf(w, "- **File Count:** %d\n", stats.FileCount)
	fmt.Fprintf(w, "- **Total Lines:** %d\n", stats.TotalLines)
	fmt.Fprintf(w, "- **Total Size:** %.2f KB\n", float64(stats.TotalSize)/1024)
	fmt.Fprintln(w)

	// Table of Contents（行数此时已全部统计完毕）
	fmt.Fprintln(w, "## 📂 扫描目录")
	for i, file := range files {
		if failedIndices[i] {
			continue
		}
		anchor := generateAnchor(file.RelPath)
		fmt.Fprintf(w, "- [%s](#%s) (%d lines, %.2f KB)\n",
			file.RelPath, anchor, file.LineCount, float64(file.Size)/1024)
	}
	fmt.Fprintln(w, "\n---")

	// 从临时文件复制内容
	tmpRead, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("读取临时文件失败: %w", err)
	}
	defer tmpRead.Close()

	if _, err := io.Copy(w, tmpRead); err != nil {
		return fmt.Errorf("复制内容失败: %w", err)
	}

	// Footer
	fmt.Fprintln(w, "\n---")
	fmt.Fprintln(w, "### 📊 最终统计汇总")
	fmt.Fprintf(w, "- **文件总数:** %d\n", stats.FileCount)
	fmt.Fprintf(w, "- **代码总行数:** %d\n", stats.TotalLines)
	fmt.Fprintf(w, "- **物理总大小:** %.2f KB\n", float64(stats.TotalSize)/1024)

	return w.Flush()
}

// countLinesInContent 从内存中的字节切片计算行数（避免再次打开文件）
func countLinesInContent(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	count := bytes.Count(data, []byte{'\n'})
	// 如果文件不以换行符结尾，最后一行也要计入
	if data[len(data)-1] != '\n' {
		count++
	}
	return count
}

// determineFence 扫描文件内容中反引号的最大连续长度，返回比它多 1 的 fence。
// 这样即使源文件里包含 ``` 或 ````，输出也不会被截断。
func determineFence(content []byte) string {
	maxRun := 0
	currentRun := 0
	for _, b := range content {
		if b == '`' {
			currentRun++
			if currentRun > maxRun {
				maxRun = currentRun
			}
		} else {
			currentRun = 0
		}
	}
	fenceLen := maxRun + 1
	if fenceLen < 3 {
		fenceLen = 3
	}
	return strings.Repeat("`", fenceLen)
}

// generateAnchor 根据 "## 📄 {relPath}" 格式的标题生成 Markdown 兼容锚点。
// 用 '-' 替换 '/'、'.' 等分隔符，避免不同路径产生相同锚点。
func generateAnchor(relPath string) string {
	raw := "📄-" + relPath
	raw = strings.ToLower(raw)

	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ', r == '/', r == '.':
			b.WriteRune('-')
		case r > 127:
			b.WriteRune(r) // 保留 Unicode（emoji 等）
		}
	}

	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

/*
====================================================
 Logging
====================================================
*/

func logf(verbose bool, format string, a ...any) {
	if verbose {
		fmt.Printf(format+"\n", a...)
	}
}