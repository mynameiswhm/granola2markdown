package export

import (
	"os"
	"path/filepath"
	"strings"
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
		Strategy: "cache",
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
	if metadata[FieldStrategy] != "cache" {
		t.Fatalf("unexpected strategy: %q", metadata[FieldStrategy])
	}
	if metadata[FieldContentSource] != "doc" {
		t.Fatalf("unexpected content source: %q", metadata[FieldContentSource])
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

func TestScanExistingIndexesDocumentEvenWithoutUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown-date-note.md")
	err := WriteMarkdown(path, map[string]string{
		FieldDocumentID: "doc-1",
		FieldUpdatedAt:  "",
		FieldDate:       "[[unknown-date]]",
	}, "body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	index, err := ScanExisting(dir)
	if err != nil {
		t.Fatalf("ScanExisting failed: %v", err)
	}
	record, ok := index.ByDocument["doc-1"]
	if !ok {
		t.Fatalf("expected doc-1 to be indexed by document")
	}
	if record.Path != path {
		t.Fatalf("unexpected path: %q", record.Path)
	}
	if len(index.ByExact) != 0 {
		t.Fatalf("expected no exact index entries, got %d", len(index.ByExact))
	}
}

func TestExportRenamesUnknownDateFileWhenCreatedAtBecomesKnown(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "unknown-date-old-meeting.md")
	err := WriteMarkdown(oldPath, map[string]string{
		FieldDocumentID: "doc-1",
		FieldUpdatedAt:  "",
		FieldDate:       "[[unknown-date]]",
	}, "old body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-02-12T09:00:12.254Z", "2026-02-12T10:17:24.176Z", "Old Meeting")
	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", counts)
	}

	newPath := filepath.Join(dir, "2026-02-12-old-meeting.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected renamed file at %s: %v", newPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be removed, stat err=%v", err)
	}
}

func TestExportRenamesUntitledFileWhenHeadingBecomesKnown(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "2026-04-27-untitled.md")
	err := WriteMarkdown(oldPath, map[string]string{
		FieldDocumentID:    "doc-1",
		FieldUpdatedAt:     "2026-04-27T10:00:00.000Z",
		FieldDate:          "[[2026-04-27]]",
		FieldContentSource: "doc",
		FieldStrategy:      "cache",
	}, "old body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-04-27T09:00:00.000Z", "2026-04-27T11:00:00.000Z", "Final Title")
	candidate.Strategy = "cache"

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", counts)
	}

	newPath := filepath.Join(dir, "2026-04-27-final-title.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected renamed file at %s: %v", newPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be removed, stat err=%v", err)
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldUpdatedAt] != "2026-04-27T11:00:00.000Z" {
		t.Fatalf("unexpected updated_at: %q", metadata[FieldUpdatedAt])
	}
	if metadata[FieldStrategy] != "cache" {
		t.Fatalf("expected updated strategy, got %q", metadata[FieldStrategy])
	}
}

func TestExportRenamesUntitledFileWithCollisionSuffix(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "2026-04-27-untitled.md")
	err := WriteMarkdown(oldPath, map[string]string{
		FieldDocumentID:    "doc-1",
		FieldUpdatedAt:     "2026-04-27T10:00:00.000Z",
		FieldDate:          "[[2026-04-27]]",
		FieldContentSource: "doc",
	}, "old body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	collisionPath := filepath.Join(dir, "2026-04-27-final-title.md")
	err = WriteMarkdown(collisionPath, map[string]string{
		FieldDocumentID: "doc-2",
		FieldUpdatedAt:  "2026-04-27T09:00:00.000Z",
		FieldDate:       "[[2026-04-27]]",
	}, "other body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-04-27T09:00:00.000Z", "2026-04-27T11:00:00.000Z", "Final Title")

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", counts)
	}

	renamedPath := filepath.Join(dir, "2026-04-27-final-title-2.md")
	if _, err := os.Stat(renamedPath); err != nil {
		t.Fatalf("expected collision-safe renamed file at %s: %v", renamedPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be removed, stat err=%v", err)
	}
}

func TestExportUpgradesGeneratedLinesNoteToMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-04-27-upgrade-me.md")
	err := WriteMarkdown(path, map[string]string{
		FieldDocumentID:    "doc-1",
		FieldUpdatedAt:     "2026-04-27T10:00:00.000Z",
		FieldDate:          "[[2026-04-27]]",
		FieldContentSource: "generated_lines",
		FieldStrategy:      "cache",
	}, "old generated body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-04-27T09:00:00.000Z", "2026-04-27T10:00:00.000Z", "Doc Title")
	candidate.Panel.Markdown = "### Better markdown\n\nBody"
	candidate.Panel.Content = nil
	candidate.Panel.OriginalContent = ""
	candidate.Panel.GeneratedLines = nil
	candidate.Strategy = "cache"

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", counts)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldContentSource] != "markdown" {
		t.Fatalf("expected upgraded content source markdown, got %q", metadata[FieldContentSource])
	}
	if metadata[FieldStrategy] != "cache" {
		t.Fatalf("expected cache strategy, got %q", metadata[FieldStrategy])
	}
	if !strings.Contains(string(data), "### Better markdown") {
		t.Fatalf("expected markdown body to be replaced")
	}
}

func TestExportNeverTouchesExistingMarkdownNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-04-27-locked.md")
	err := WriteMarkdown(path, map[string]string{
		FieldDocumentID:    "doc-1",
		FieldUpdatedAt:     "2026-04-27T10:00:00.000Z",
		FieldDate:          "[[2026-04-27]]",
		FieldContentSource: "markdown",
		FieldStrategy:      "cache",
	}, "locked markdown body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-04-27T09:00:00.000Z", "2026-04-27T11:00:00.000Z", "Doc Title")
	candidate.Strategy = "cache"

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Skipped != 1 {
		t.Fatalf("expected one skipped note, got %+v", counts)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldContentSource] != "markdown" {
		t.Fatalf("expected markdown content source to remain locked, got %q", metadata[FieldContentSource])
	}
	if metadata[FieldStrategy] != "cache" {
		t.Fatalf("expected original strategy to remain unchanged, got %q", metadata[FieldStrategy])
	}
	if !strings.Contains(string(data), "locked markdown body") {
		t.Fatalf("expected locked markdown body to remain unchanged")
	}
}

func TestExportRenamesExistingMarkdownUntitledNoteWhenTitleBecomesKnown(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "2026-04-27-untitled.md")
	err := WriteMarkdown(oldPath, map[string]string{
		FieldDocumentID:    "doc-1",
		FieldUpdatedAt:     "2026-04-27T10:00:00.000Z",
		FieldDate:          "[[2026-04-27]]",
		FieldContentSource: "markdown",
		FieldStrategy:      "cache",
	}, "locked markdown body")
	if err != nil {
		t.Fatalf("WriteMarkdown failed: %v", err)
	}

	candidate := makeCandidate("doc-1", "panel-1", "2026-04-27T09:00:00.000Z", "2026-04-27T11:00:00.000Z", "Doc Title")
	candidate.Panel.Markdown = "### Doc Title\n\nBetter markdown"
	candidate.Panel.Content = nil
	candidate.Panel.OriginalContent = ""
	candidate.Panel.GeneratedLines = nil
	candidate.Strategy = "cache"

	counts, err := ExportCandidates([]model.NoteCandidate{candidate}, dir, false)
	if err != nil {
		t.Fatalf("ExportCandidates failed: %v", err)
	}
	if counts.Updated != 1 {
		t.Fatalf("expected one updated note, got %+v", counts)
	}

	newPath := filepath.Join(dir, "2026-04-27-doc-title.md")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected renamed file at %s: %v", newPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old path to be removed, stat err=%v", err)
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read markdown failed: %v", err)
	}
	metadata := ParseFrontMatter(string(data))
	if metadata[FieldContentSource] != "markdown" {
		t.Fatalf("expected markdown content source to remain markdown, got %q", metadata[FieldContentSource])
	}
	if metadata[FieldStrategy] != "cache" {
		t.Fatalf("expected strategy to update during rename, got %q", metadata[FieldStrategy])
	}
	if !strings.Contains(string(data), "### Doc Title") {
		t.Fatalf("expected markdown body to be refreshed during rename")
	}
}
