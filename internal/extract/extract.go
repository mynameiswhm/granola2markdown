package extract

import (
	"strings"

	"github.com/mynameiswhm/granola2markdown/internal/markdown"
	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func ExtractContent(panel model.PanelData, documentTitle string) model.ExtractedContent {
	if markdownContent := strings.TrimSpace(panel.Markdown); markdownContent != "" {
		heading := firstNonEmpty(documentTitle, firstMarkdownHeading(markdownContent))
		return model.ExtractedContent{Markdown: markdownContent, FirstHeading: heading, Source: "markdown"}
	}

	structuredMarkdown, firstHeading := markdown.RenderStructuredDoc(panel.Content)
	heading := firstNonEmpty(firstHeading, documentTitle)
	if strings.TrimSpace(structuredMarkdown) != "" {
		return model.ExtractedContent{Markdown: structuredMarkdown, FirstHeading: heading, Source: "doc"}
	}

	htmlMarkdown := markdown.HTMLToMarkdown(panel.OriginalContent)
	if strings.TrimSpace(htmlMarkdown) != "" {
		return model.ExtractedContent{Markdown: htmlMarkdown, FirstHeading: heading, Source: "html"}
	}

	generated := GeneratedLinesToMarkdown(panel.GeneratedLines)
	if strings.TrimSpace(generated) != "" {
		return model.ExtractedContent{Markdown: generated, FirstHeading: heading, Source: "generated_lines"}
	}

	return model.ExtractedContent{Markdown: "", FirstHeading: heading, Source: "empty"}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMarkdownHeading(value string) string {
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasPrefix(trimmed, "#") {
			continue
		}
		return strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	}
	return ""
}
