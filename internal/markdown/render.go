package markdown

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

func RenderStructuredDoc(content map[string]any) (string, string) {
	if content == nil {
		return "", ""
	}
	if toString(content["type"]) != "doc" {
		return "", ""
	}

	blocks, ok := asSlice(content["content"])
	if !ok {
		return "", ""
	}

	renderedBlocks := make([]string, 0, len(blocks))
	for _, node := range blocks {
		block := strings.TrimSpace(renderBlock(node))
		if block != "" {
			renderedBlocks = append(renderedBlocks, strings.TrimRight(block, "\n"))
		}
	}

	markdown := strings.TrimSpace(strings.Join(renderedBlocks, "\n\n"))
	return markdown, extractFirstHeading(blocks)
}

func HTMLToMarkdown(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	s := html.UnescapeString(value)
	s = strings.ReplaceAll(s, "\r\n", "\n")

	anchorRe := regexp.MustCompile(`(?is)<a[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	s = anchorRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := anchorRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return ""
		}
		text := strings.TrimSpace(stripTags(parts[2]))
		if text == "" {
			return ""
		}
		return fmt.Sprintf("[%s](%s)", collapseInlineSpacing(text), parts[1])
	})

	headingRe := regexp.MustCompile(`(?is)<h([1-6])[^>]*>(.*?)</h[1-6]>`)
	s = headingRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := headingRe.FindStringSubmatch(match)
		if len(parts) != 3 {
			return "\n\n"
		}
		level := toInt(parts[1], 1)
		text := strings.TrimSpace(stripTags(parts[2]))
		if text == "" {
			return "\n\n"
		}
		return fmt.Sprintf("\n\n%s %s\n\n", strings.Repeat("#", level), collapseInlineSpacing(text))
	})

	hrRe := regexp.MustCompile(`(?is)<hr[^>]*>`)
	s = hrRe.ReplaceAllString(s, "\n\n---\n\n")

	liRe := regexp.MustCompile(`(?is)<li[^>]*>(.*?)</li>`)
	s = liRe.ReplaceAllStringFunc(s, func(match string) string {
		parts := liRe.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
		}
		text := strings.TrimSpace(stripTags(parts[1]))
		if text == "" {
			return ""
		}
		return "\n- " + collapseInlineSpacing(text)
	})

	brRe := regexp.MustCompile(`(?is)<br\s*/?>`)
	s = brRe.ReplaceAllString(s, "\n")

	closingBlockRe := regexp.MustCompile(`(?is)</(p|div|ul|ol)>`)
	s = closingBlockRe.ReplaceAllString(s, "\n\n")

	openingBlockRe := regexp.MustCompile(`(?is)<(p|div|ul|ol)[^>]*>`)
	s = openingBlockRe.ReplaceAllString(s, "")

	s = stripTags(s)
	return normalizeMarkdownWhitespace(s)
}

func extractFirstHeading(nodes []any) string {
	for _, node := range nodes {
		mapped, ok := asMap(node)
		if !ok {
			continue
		}
		if toString(mapped["type"]) != "heading" {
			continue
		}
		text := inlineText(mapped["content"])
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func renderBlock(node any) string {
	mapped, ok := asMap(node)
	if !ok {
		return ""
	}

	nodeType := toString(mapped["type"])
	switch nodeType {
	case "heading":
		attrs, _ := asMap(mapped["attrs"])
		level := toInt(attrs["level"], 1)
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		text := inlineText(mapped["content"])
		if text == "" {
			return strings.Repeat("#", level)
		}
		return fmt.Sprintf("%s %s", strings.Repeat("#", level), text)
	case "paragraph":
		return inlineText(mapped["content"])
	case "horizontalRule":
		return "---"
	case "bulletList":
		return strings.Join(renderList(mapped, false, 0), "\n")
	case "orderedList":
		return strings.Join(renderList(mapped, true, 0), "\n")
	default:
		children, ok := asSlice(mapped["content"])
		if !ok {
			return ""
		}
		parts := make([]string, 0, len(children))
		for _, child := range children {
			rendered := strings.TrimSpace(renderBlock(child))
			if rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "\n\n")
	}
}

func renderList(node map[string]any, ordered bool, indent int) []string {
	items, ok := asSlice(node["content"])
	if !ok {
		return nil
	}

	var out []string
	for i, item := range items {
		itemMap, ok := asMap(item)
		if !ok || toString(itemMap["type"]) != "listItem" {
			continue
		}
		renderListItem(itemMap, indent, ordered, i+1, &out)
	}
	return out
}

func renderListItem(item map[string]any, indent int, ordered bool, index int, out *[]string) {
	children, ok := asSlice(item["content"])
	if !ok {
		return
	}

	prefix := "- "
	if ordered {
		prefix = fmt.Sprintf("%d. ", index)
	}
	prefixPadding := strings.Repeat(" ", indent+len(prefix))
	indentPrefix := strings.Repeat(" ", indent)
	firstLineWritten := false

	for _, child := range children {
		childMap, ok := asMap(child)
		if !ok {
			continue
		}
		childType := toString(childMap["type"])

		switch childType {
		case "paragraph", "heading":
			text := inlineText(childMap["content"])
			if !firstLineWritten {
				*out = append(*out, strings.TrimRight(indentPrefix+prefix+text, " "))
				firstLineWritten = true
			} else if strings.TrimSpace(text) != "" {
				*out = append(*out, strings.TrimRight(prefixPadding+text, " "))
			}
		case "bulletList", "orderedList":
			if !firstLineWritten {
				*out = append(*out, strings.TrimRight(indentPrefix+prefix, " "))
				firstLineWritten = true
			}
			nested := renderList(childMap, childType == "orderedList", indent+2)
			*out = append(*out, nested...)
		default:
			rendered := strings.TrimSpace(renderBlock(child))
			if rendered == "" {
				continue
			}
			if !firstLineWritten {
				*out = append(*out, strings.TrimRight(indentPrefix+prefix+rendered, " "))
				firstLineWritten = true
			} else {
				for _, line := range strings.Split(rendered, "\n") {
					*out = append(*out, strings.TrimRight(prefixPadding+line, " "))
				}
			}
		}
	}

	if !firstLineWritten {
		*out = append(*out, strings.TrimRight(indentPrefix+prefix, " "))
	}
}

func inlineText(content any) string {
	nodes, ok := asSlice(content)
	if !ok {
		return ""
	}

	var parts []string
	for _, node := range nodes {
		nodeMap, ok := asMap(node)
		if !ok {
			continue
		}
		nodeType := toString(nodeMap["type"])

		switch nodeType {
		case "text":
			text := toString(nodeMap["text"])
			if marks, ok := asSlice(nodeMap["marks"]); ok {
				for _, mark := range marks {
					markMap, ok := asMap(mark)
					if !ok || toString(markMap["type"]) != "link" {
						continue
					}
					attrs, _ := asMap(markMap["attrs"])
					href := toString(attrs["href"])
					if href != "" {
						text = fmt.Sprintf("[%s](%s)", text, href)
					}
				}
			}
			parts = append(parts, text)
		case "hardBreak":
			parts = append(parts, "\n")
		default:
			if childNodes, ok := asSlice(nodeMap["content"]); ok {
				parts = append(parts, inlineText(childNodes))
			}
		}
	}

	return collapseInlineSpacing(strings.Join(parts, ""))
}

func collapseInlineSpacing(value string) string {
	lines := strings.Split(value, "\n")
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = strings.Join(strings.Fields(lines[i]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func normalizeMarkdownWhitespace(value string) string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blankCount++
			if blankCount <= 2 {
				out = append(out, "")
			}
			continue
		}
		blankCount = 0
		out = append(out, strings.Join(strings.Fields(trimmed), " "))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func stripTags(value string) string {
	tagRe := regexp.MustCompile(`(?is)<[^>]+>`)
	return tagRe.ReplaceAllString(value, "")
}

func asMap(value any) (map[string]any, bool) {
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

func asSlice(value any) ([]any, bool) {
	slice, ok := value.([]any)
	if ok {
		return slice, true
	}

	// Allows tests to pass []map[string]any directly.
	if typed, ok := value.([]map[string]any); ok {
		converted := make([]any, 0, len(typed))
		for _, item := range typed {
			converted = append(converted, item)
		}
		return converted, true
	}

	return nil, false
}

func toString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func toInt(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if typed == "" {
			return fallback
		}
		var parsed int
		_, err := fmt.Sscanf(typed, "%d", &parsed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
