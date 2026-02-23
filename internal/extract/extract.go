package extract

import (
	"strings"

	"github.com/mynameiswhm/granola2markdown/internal/markdown"
	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func ExtractContent(panel model.PanelData) model.ExtractedContent {
	structuredMarkdown, firstHeading := markdown.RenderStructuredDoc(panel.Content)
	if strings.TrimSpace(structuredMarkdown) != "" {
		return model.ExtractedContent{Markdown: structuredMarkdown, FirstHeading: firstHeading, Source: "doc"}
	}

	htmlMarkdown := markdown.HTMLToMarkdown(panel.OriginalContent)
	if strings.TrimSpace(htmlMarkdown) != "" {
		return model.ExtractedContent{Markdown: htmlMarkdown, FirstHeading: firstHeading, Source: "html"}
	}

	generated := GeneratedLinesToMarkdown(panel.GeneratedLines)
	if strings.TrimSpace(generated) != "" {
		return model.ExtractedContent{Markdown: generated, FirstHeading: firstHeading, Source: "generated_lines"}
	}

	return model.ExtractedContent{Markdown: "", FirstHeading: firstHeading, Source: "empty"}
}

func GeneratedLinesToMarkdown(lines []map[string]any) string {
	if len(lines) == 0 {
		return ""
	}

	parts := make([]string, 0, len(lines))
	for _, item := range lines {
		text, _ := item["text"].(string)
		if strings.TrimSpace(text) != "" {
			parts = append(parts, strings.TrimSpace(text))
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
