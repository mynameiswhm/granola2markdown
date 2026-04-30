package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/mynameiswhm/granola2markdown/internal/export"
)

func TestFixtureExportThenIdempotentRerun(t *testing.T) {
	cachePath := findFixtureCachePath(t)
	outputDir := t.TempDir()

	first, err := RunExport(outputDir, cachePath, false)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if first.Exported <= 0 {
		t.Fatalf("expected first run to export notes, got %+v", first)
	}
	if first.Errors != 0 {
		t.Fatalf("expected no errors on first run, got %+v", first)
	}

	second, err := RunExport(outputDir, cachePath, false)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if second.Exported != 0 || second.Updated != 0 || second.Skipped <= 0 {
		t.Fatalf("unexpected second run counts: %+v", second)
	}

	files, err := filepath.Glob(filepath.Join(outputDir, "*.md"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected markdown files to be exported")
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := export.ParseFrontMatter(string(data))
	if metadata[export.FieldDocumentID] == "" {
		t.Fatalf("expected granola document id metadata")
	}
	if metadata[export.FieldUpdatedAt] == "" {
		t.Fatalf("expected granola updated at metadata")
	}
	if metadata[export.FieldDate] == "" {
		t.Fatalf("expected date metadata")
	}
	if _, ok := metadata["granola_panel_id"]; ok {
		t.Fatalf("legacy panel id should not be present")
	}
	if _, ok := metadata["granola_export_source"]; ok {
		t.Fatalf("legacy export source should not be present")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func findFixtureCachePath(t *testing.T) string {
	t.Helper()
	root := findRepoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "cache-v*.json"))
	if err != nil {
		t.Fatalf("cache fixture glob failed: %v", err)
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(root, "cache-v*-pretty*.json"))
		if err != nil {
			t.Fatalf("cache fixture glob failed: %v", err)
		}
	}
	if len(matches) == 0 {
		t.Fatalf("could not locate a cache fixture under %s", root)
	}
	sort.Strings(matches)
	return matches[0]
}
