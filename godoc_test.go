package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		// Basename matching (no path separator in pattern)
		{"godoc.go", "godoc.go", true},
		{"godoc.go", "godoc", false}, // exact basename match only, no prefix
		{"src/godoc.go", "godoc.go", true},
		{"src/godoc.go", "godoc", false},
		{"foo.go", "*.go", true},
		{"foo.txt", "*.go", false},
		{"README.md", "*.md", true},

		// Wildcard patterns
		{"test.txt", "test.*", true},
		{"test.go", "test.*", true},
		{"main.go", "test.*", false},

		// Path-based patterns (contain /)
		{"src/main.go", "src/*.go", true},
		{"src/main.go", "pkg/*.go", false},
		{"src/sub/main.go", "src/*.go", false}, // filepath.Match does not match nested
	}

	for _, tt := range tests {
		got := matchPattern(tt.path, tt.pattern)
		if got != tt.want {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
		}
	}
}

func TestDetermineFence(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"hello world", "```"},
		{"no backticks here", "```"},
		{"some `inline` code", "```"},
		{"``double``", "```"},
		{"```code block```", "````"},
		{"````four````", "`````"},
		{"``````", "```````"},
	}
	for _, tt := range tests {
		got := determineFence(tt.content)
		if got != tt.want {
			t.Errorf("determineFence(%q) = %q, want %q", tt.content, got, tt.want)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"app.ts", "typescript"},
		{"index.js", "javascript"},
		{"style.css", "css"},
		{"README.md", "markdown"},
		{"config.yaml", "yaml"},
		{"Dockerfile", "dockerfile"},
		{"Makefile", "makefile"},
		{"unknown.xyz", ""},
	}
	for _, tt := range tests {
		got := detectLanguage(tt.path)
		if got != tt.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestIsBinaryFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Text file
	textPath := filepath.Join(tmpDir, "text.txt")
	os.WriteFile(textPath, []byte("hello world\nline 2\n"), 0644)
	if isBinaryFile(textPath) {
		t.Error("text file should not be detected as binary")
	}

	// Binary file: invalid UTF-8 containing NULL byte
	binPath := filepath.Join(tmpDir, "binary.bin")
	os.WriteFile(binPath, []byte("\xff\x00\xfe\x00"), 0644)
	if !isBinaryFile(binPath) {
		t.Error("file with invalid UTF-8 and NULL bytes should be detected as binary")
	}

	// Empty file
	emptyPath := filepath.Join(tmpDir, "empty.txt")
	os.WriteFile(emptyPath, []byte{}, 0644)
	if isBinaryFile(emptyPath) {
		t.Error("empty file should not be detected as binary")
	}
}

func TestEstimateTokens(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	content := "hello world"
	os.WriteFile(path, []byte(content), 0644)

	tokens := estimateTokens(path)
	expected := len(content) / 4 // 11 / 4 = 2
	if tokens != expected {
		t.Errorf("estimateTokens() = %d, want %d", tokens, expected)
	}
}

func TestCountLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "lines.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	lines := countLines(path)
	if lines != 3 {
		t.Errorf("countLines() = %d, want 3", lines)
	}
}

func TestCleanList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"go,ts,py", []string{".go", ".ts", ".py"}},
		{".go, .ts", []string{".go", ".ts"}},
		{"go", []string{".go"}},
		{"src/main", []string{"src/main"}},
	}
	for _, tt := range tests {
		got := cleanList(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("cleanList(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("cleanList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestGenerateAnchor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"src/main.go", "src-main-go"},
		{"README.md", "readme-md"},
		{"pkg\\util.js", "pkg-util-js"},
	}
	for _, tt := range tests {
		got := generateAnchor(tt.input)
		if got != tt.want {
			t.Errorf("generateAnchor(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildTreeString(t *testing.T) {
	files := []FileMetadata{
		{RelPath: "cmd/app/main.go"},
		{RelPath: "cmd/app/util.go"},
		{RelPath: "go.mod"},
		{RelPath: "internal/handler.go"},
		{RelPath: "README.md"},
	}

	got := buildTreeString(files, "myproject")
	want := `myproject/
├── cmd/
│   └── app/
│       ├── main.go  (0 lines)
│       └── util.go  (0 lines)
├── internal/
│   └── handler.go  (0 lines)
├── README.md  (0 lines)
└── go.mod  (0 lines)
`
	if got != want {
		t.Errorf("buildTreeString() =\n%s\nwant:\n%s", got, want)
	}
}
