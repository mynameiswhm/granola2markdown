package export

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func makeCandidate(documentID string, panelID string, createdAt string, contentUpdatedAt string, heading string) model.NoteCandidate {
	return model.NoteCandidate{
		Document: model.DocumentData{ID: documentID, CreatedAt: createdAt},
		Panel: model.PanelData{
			ID:               panelID,
			DocumentID:       documentID,
			Title:            "Summary",
			TemplateSlug:     "meeting-summary-consolidated",
			CreatedAt:        createdAt,
			ContentUpdatedAt: contentUpdatedAt,
			Content: map[string]any{
				"type": "doc",
				"content": []any{
					map[string]any{
						"type":    "heading",
						"attrs":   map[string]any{"level": 3},
						"content": []any{map[string]any{"type": "text", "text": heading}},
					},
					map[string]any{
						"type":    "paragraph",
						"content": []any{map[string]any{"type": "text", "text": "Body text"}},
					},
				},
			},
		},
	}
}

func TestGenerateFilename(t *testing.T) {
	name := GenerateFilename("2026-02-13T14:20:00.000Z", "Plans and Next Steps")
	if name != "2026-02-13-plans-and-next-steps.md" {
		t.Fatalf("unexpected filename: %s", name)
	}
}

func TestDedupSkipAndUpdate(t *testing.T) {
	dir := t.TempDir()
	candidate := makeCandidate("doc-1", "panel-1", "2026-02-13T14:20:00.000Z", "2026-02-13T14:20:00.000Z", "Plans and Next Steps")

	first, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("first export failed: %v", err)
	}
	if first.Exported != 1 || first.Skipped != 0 {
		t.Fatalf("unexpected first counts: %+v", first)
	}

	second, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("second export failed: %v", err)
	}
	if second.Skipped != 1 || second.Exported != 0 {
		t.Fatalf("unexpected second counts: %+v", second)
	}

	updatedCandidate := makeCandidate("doc-1", "panel-1", "2026-02-13T14:20:00.000Z", "2026-02-14T10:00:00.000Z", "Plans and Next Steps")
	third, err := ExportCandidates([]model.NoteCandidate{updatedCandidate}, dir, false)
	if err != nil {
		t.Fatalf("third export failed: %v", err)
	}
	if third.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", third)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 markdown file, got %d", len(files))
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldUpdatedAt] != "2026-02-14T10:00:00.000Z" {
		t.Fatalf("unexpected updated_at: %q", metadata[FieldUpdatedAt])
	}
	if metadata[FieldDate] != "[[2026-02-13]]" {
		t.Fatalf("unexpected date: %q", metadata[FieldDate])
	}
	if _, ok := metadata["granola_panel_id"]; ok {
		t.Fatalf("legacy panel id should not be present")
	}
	if _, ok := metadata["granola_export_source"]; ok {
		t.Fatalf("legacy export source should not be present")
	}
}

func TestDateUsesDocumentCreatedAtOnReimport(t *testing.T) {
	dir := t.TempDir()
	candidate := makeCandidate(
		"doc-old",
		"panel-old",
		"2021-04-09T09:00:00.000Z",
		"2026-02-20T10:00:00.000Z",
		"Old Meeting",
	)

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if counts.Exported != 1 {
		t.Fatalf("expected one export, got %+v", counts)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 markdown file, got %d", len(files))
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldUpdatedAt] != "2026-02-20T10:00:00.000Z" {
		t.Fatalf("unexpected updated_at: %q", metadata[FieldUpdatedAt])
	}
	if metadata[FieldDate] != "[[2021-04-09]]" {
		t.Fatalf("unexpected date: %q", metadata[FieldDate])
	}
}

func TestCollisionSuffixes(t *testing.T) {
	dir := t.TempDir()
	one := makeCandidate("doc-1", "panel-1", "2026-02-13T14:20:00.000Z", "2026-02-13T14:20:00.000Z", "Same Heading")
	two := makeCandidate("doc-2", "panel-2", "2026-02-13T15:20:00.000Z", "2026-02-13T15:20:00.000Z", "Same Heading")

	counts, err := ExportCandidates([]model.NoteCandidate{one, two}, dir, false)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if counts.Exported != 2 {
		t.Fatalf("expected two exports, got %+v", counts)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	names := map[string]struct{}{}
	for _, path := range files {
		names[filepath.Base(path)] = struct{}{}
	}
	if _, ok := names["2026-02-13-same-heading.md"]; !ok {
		t.Fatalf("missing base filename")
	}
	if _, ok := names["2026-02-13-same-heading-2.md"]; !ok {
		t.Fatalf("missing collision suffix filename")
	}
}

func TestComputeGranolaUpdatedAtUsesLatestTimestamp(t *testing.T) {
	updatedAt := ComputeGranolaUpdatedAt(
		"2026-02-13T14:20:00.000Z",
		"2026-02-15T09:00:00.000Z",
		"2026-02-14T12:00:00.000Z",
	)
	if updatedAt != "2026-02-15T09:00:00.000Z" {
		t.Fatalf("unexpected max timestamp: %s", updatedAt)
	}
}

func TestComputeGranolaUpdatedAtFallbacksToAvailableValues(t *testing.T) {
	updatedAt := ComputeGranolaUpdatedAt("not-a-timestamp", "", "")
	if updatedAt != "not-a-timestamp" {
		t.Fatalf("unexpected fallback: %s", updatedAt)
	}
}

func TestMarkdownDatePropertyUsesWikiLinkFormat(t *testing.T) {
	date := MarkdownDateProperty("2026-02-15T09:00:00.000Z")
	if date != "[[2026-02-15]]" {
		t.Fatalf("unexpected date property: %s", date)
	}
}

func TestMarkdownDatePropertyInvalidTimestampUsesUnknownDate(t *testing.T) {
	date := MarkdownDateProperty("not-a-timestamp")
	if date != "[[unknown-date]]" {
		t.Fatalf("unexpected date property: %s", date)
	}
}
