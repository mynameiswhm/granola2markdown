package markdown

import (
	"strings"
	"testing"
)

func TestRenderStructuredDocExtractsHeading(t *testing.T) {
	content := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":    "heading",
				"attrs":   map[string]any{"level": 3},
				"content": []any{map[string]any{"type": "text", "text": "Plans and Next Steps"}},
			},
			map[string]any{
				"type":    "paragraph",
				"content": []any{map[string]any{"type": "text", "text": "Body"}},
			},
		},
	}

	markdown, heading := RenderStructuredDoc(content)
	if heading != "Plans and Next Steps" {
		t.Fatalf("expected heading, got %q", heading)
	}
	if !strings.Contains(markdown, "### Plans and Next Steps") {
		t.Fatalf("expected markdown heading, got: %s", markdown)
	}
}

func TestRenderStructuredDocNestedListsAndLinks(t *testing.T) {
	content := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "bulletList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type":    "paragraph",
								"content": []any{map[string]any{"type": "text", "text": "Top"}},
							},
							map[string]any{
								"type": "orderedList",
								"content": []any{
									map[string]any{
										"type": "listItem",
										"content": []any{
											map[string]any{
												"type": "paragraph",
												"content": []any{
													map[string]any{
														"type":  "text",
														"text":  "Visit",
														"marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "https://example.com"}}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	markdown, _ := RenderStructuredDoc(content)
	if !strings.Contains(markdown, "- Top") {
		t.Fatalf("expected top bullet item, got: %s", markdown)
	}
	if !strings.Contains(markdown, "1. [Visit](https://example.com)") {
		t.Fatalf("expected nested ordered list with link, got: %s", markdown)
	}
}

func TestHTMLToMarkdownFallback(t *testing.T) {
	result := HTMLToMarkdown("<h2>Title</h2><p>Hello <a href=\"https://example.com\">world</a></p>")
	if !strings.Contains(result, "## Title") {
		t.Fatalf("expected heading, got: %s", result)
	}
	if !strings.Contains(result, "[world](https://example.com)") {
		t.Fatalf("expected markdown link, got: %s", result)
	}
}
