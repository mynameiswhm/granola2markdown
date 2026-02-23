package extract

import (
	"testing"

	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func TestExtractContentUsesStructuredDocFirst(t *testing.T) {
	panel := model.PanelData{
		Content: map[string]any{
			"type": "doc",
			"content": []any{
				map[string]any{
					"type":    "heading",
					"attrs":   map[string]any{"level": 2},
					"content": []any{map[string]any{"type": "text", "text": "Structured"}},
				},
			},
		},
		OriginalContent: "<p>html</p>",
		GeneratedLines:  []map[string]any{{"text": "generated"}},
	}

	extracted := ExtractContent(panel)
	if extracted.Source != "doc" {
		t.Fatalf("expected doc source, got %s", extracted.Source)
	}
	if extracted.FirstHeading != "Structured" {
		t.Fatalf("expected heading Structured, got %q", extracted.FirstHeading)
	}
}

func TestExtractContentFallsBackToGeneratedLines(t *testing.T) {
	panel := model.PanelData{
		GeneratedLines: []map[string]any{{"text": "line 1"}, {"text": "line 2"}},
	}

	extracted := ExtractContent(panel)
	if extracted.Source != "generated_lines" {
		t.Fatalf("expected generated_lines source, got %s", extracted.Source)
	}
	if extracted.Markdown != "line 1\nline 2" {
		t.Fatalf("unexpected generated markdown: %q", extracted.Markdown)
	}
}
