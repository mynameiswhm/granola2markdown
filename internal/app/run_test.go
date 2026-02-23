package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mynameiswhm/granola2markdown/internal/export"
)

func TestFixtureExportThenIdempotentRerun(t *testing.T) {
	root := findRepoRoot(t)
	cachePath := filepath.Join(root, "cache-v3.json")
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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "cache-v3.json")); err == nil {
			return cwd
		}
		next := filepath.Dir(cwd)
		if next == cwd {
			t.Fatalf("could not locate repository root from %s", cwd)
		}
		cwd = next
	}
}
