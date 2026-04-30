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

	extracted := ExtractContent(panel, "Doc title")
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

	extracted := ExtractContent(panel, "Doc title")
	if extracted.Source != "generated_lines" {
		t.Fatalf("expected generated_lines source, got %s", extracted.Source)
	}
	if extracted.Markdown != "line 1\nline 2" {
		t.Fatalf("unexpected generated markdown: %q", extracted.Markdown)
	}
	if extracted.FirstHeading != "Doc title" {
		t.Fatalf("expected fallback heading from document title, got %q", extracted.FirstHeading)
	}
}

func TestExtractContentUsesMarkdownBeforeOtherFallbacks(t *testing.T) {
	panel := model.PanelData{
		Markdown:        "### Rich summary\n\n- action item",
		OriginalContent: "<p>html</p>",
		GeneratedLines:  []map[string]any{{"text": "generated"}},
	}

	extracted := ExtractContent(panel, "Doc title")
	if extracted.Source != "markdown" {
		t.Fatalf("expected markdown source, got %s", extracted.Source)
	}
	if extracted.Markdown != "### Rich summary\n\n- action item" {
		t.Fatalf("unexpected markdown content: %q", extracted.Markdown)
	}
	if extracted.FirstHeading != "Doc title" {
		t.Fatalf("expected document title to drive naming, got %q", extracted.FirstHeading)
	}
}
