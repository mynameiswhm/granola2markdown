package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mynameiswhm/granola2markdown/internal/model"
)

type LoadError struct {
	message string
}

func (e *LoadError) Error() string {
	return e.message
}

func newLoadError(format string, args ...any) error {
	return &LoadError{message: fmt.Sprintf(format, args...)}
}

func LoadCacheState(cachePath string) (map[string]any, error) {
	plainPath, encPath := relatedCachePaths(cachePath)
	if shouldPreferEncryptedCache(cachePath, plainPath, encPath) {
		data, err := readEncryptedCacheData(encPath)
		if err != nil {
			return nil, err
		}
		state, err := decodeStateObject(data, encPath)
		if err != nil {
			return nil, err
		}
		if !hasExportableDocumentState(state) {
			return nil, newLoadError("decoded cache payload does not contain exportable note state")
		}
		return state, nil
	}

	data, err := os.ReadFile(plainPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read cache file: %w", err)
	}
	state, err := decodeStateObject(data, plainPath)
	if err != nil {
		return nil, err
	}
	if !hasExportableDocumentState(state) {
		return nil, newLoadError("decoded cache payload does not contain exportable note state")
	}
	return state, nil
}

func BuildCandidates(state map[string]any) ([]model.NoteCandidate, error) {
	documents, ok := lookupMap(state, "documents", "Documents", "docs", "Docs")
	if ok {
		return buildLegacyCandidates(state, documents)
	}
	return buildModernCandidates(state)
}

func buildLegacyCandidates(state map[string]any, documents map[string]any) ([]model.NoteCandidate, error) {
	documentPanels, hasDocumentPanels := lookupMap(state, "documentPanels", "document_panels", "DocumentPanels", "panels")
	transcripts, _ := lookupMap(state, "transcripts", "Transcripts")
	activeEditor := activeEditorState(state)

	candidates := make([]model.NoteCandidate, 0, len(documents))
	for documentID, rawDoc := range documents {
		doc, ok := toMap(rawDoc)
		if !ok {
			continue
		}
		if firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])) != "" {
			continue
		}

		if hasDocumentPanels {
			docPanelsRaw, ok := documentPanels[documentID]
			if !ok {
				altID := toString(doc["id"])
				if altID != "" {
					docPanelsRaw = documentPanels[altID]
				}
			}

			docPanels, ok := toMap(docPanelsRaw)
			if ok {
				nonDeletedPanels := parseNonDeletedPanels(documentID, docPanels)
				if len(nonDeletedPanels) > 0 {
					selected := SelectPrimaryPanel(nonDeletedPanels)
					if selected != nil {
						candidate := model.NoteCandidate{
							Document: model.DocumentData{
								ID:        documentID,
								CreatedAt: firstNonEmpty(toString(doc["created_at"]), toString(doc["createdAt"])),
								DeletedAt: firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])),
								Title:     firstNonEmpty(toString(doc["title"]), toString(doc["name"])),
							},
							Panel:    *selected,
							Strategy: "cache",
						}
						candidates = append(candidates, candidate)
						continue
					}
				}
			}
		}

		synthesized := synthesizePanelFromDocument(
			documentID,
			doc,
			transcriptLinesForDocument(transcripts, documentID, toString(doc["id"])),
			activeEditor.markdownForDocument(documentID, toString(doc["id"])),
		)
		if synthesized == nil {
			continue
		}

		candidate := model.NoteCandidate{
			Document: model.DocumentData{
				ID:        documentID,
				CreatedAt: firstNonEmpty(toString(doc["created_at"]), toString(doc["createdAt"])),
				DeletedAt: firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])),
				Title:     firstNonEmpty(toString(doc["title"]), toString(doc["name"])),
			},
			Panel:    *synthesized,
			Strategy: "cache",
		}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.Document.CreatedAt == right.Document.CreatedAt {
			return left.Document.ID < right.Document.ID
		}
		return left.Document.CreatedAt < right.Document.CreatedAt
	})

	return candidates, nil
}

func buildModernCandidates(state map[string]any) ([]model.NoteCandidate, error) {
	transcripts, _ := lookupMap(state, "transcripts", "Transcripts")
	activeEditor := activeEditorState(state)
	documentIDs := collectModernDocumentIDs(transcripts, activeEditor)

	candidates := make([]model.NoteCandidate, 0, len(documentIDs))
	for _, documentID := range documentIDs {
		markdown := activeEditor.markdownForDocument(documentID)
		generatedLines := transcriptLinesForDocument(transcripts, documentID)
		if strings.TrimSpace(markdown) == "" && len(generatedLines) == 0 {
			continue
		}

		createdAt, updatedAt := transcriptBoundsForDocument(transcripts, documentID)
		candidate := model.NoteCandidate{
			Document: model.DocumentData{
				ID:        documentID,
				CreatedAt: createdAt,
				Title:     firstMarkdownHeading(markdown),
			},
			Panel: model.PanelData{
				ID:               activeEditor.panelIDForDocument(documentID),
				DocumentID:       documentID,
				Title:            "Summary",
				TemplateSlug:     activeEditor.panelSlugForDocument(documentID),
				Markdown:         markdown,
				GeneratedLines:   generatedLines,
				CreatedAt:        createdAt,
				ContentUpdatedAt: updatedAt,
			},
			Strategy: "cache",
		}
		candidates = append(candidates, candidate)
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.Document.CreatedAt == right.Document.CreatedAt {
			return left.Document.ID < right.Document.ID
		}
		if left.Document.CreatedAt == "" {
			return false
		}
		if right.Document.CreatedAt == "" {
			return true
		}
		return left.Document.CreatedAt < right.Document.CreatedAt
	})

	if len(candidates) == 0 {
		return nil, newLoadError("state does not contain exportable documents, transcripts, or active editor markdown")
	}

	return candidates, nil
}

func collectModernDocumentIDs(transcripts map[string]any, activeEditor activeEditorDocumentState) []string {
	seen := make(map[string]struct{})
	ordered := make([]string, 0, len(transcripts)+len(activeEditor.documentIDs)+1)
	add := func(value string) {
		id := strings.TrimSpace(value)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}

	add(activeEditor.meetingID)
	for _, documentID := range activeEditor.documentIDs {
		add(documentID)
	}
	for documentID := range transcripts {
		add(documentID)
	}

	return ordered
}

func synthesizePanelFromDocument(documentID string, doc map[string]any, transcriptLines []map[string]any, activeMarkdown string) *model.PanelData {
	content := firstNonNilMap(doc["summary"], doc["overview"], doc["notes"], doc["content"])
	if !hasStructuredText(content) {
		content = nil
	}

	notesMarkdown := firstNonEmpty(toString(doc["notes_markdown"]), toString(doc["notesMarkdown"]))
	notesPlain := firstNonEmpty(toString(doc["notes_plain"]), toString(doc["notesPlain"]))
	markdownContent := firstNonEmpty(activeMarkdown, notesMarkdown)

	originalContent := firstNonEmpty(
		toString(doc["summary_html"]),
		toString(doc["overview_html"]),
		toString(doc["notes_html"]),
		toString(doc["original_content"]),
		notesPlain,
	)
	generatedLines := append([]map[string]any{}, toSliceOfMaps(doc["generated_lines"])...)
	generatedLines = append(generatedLines, transcriptLines...)
	if len(generatedLines) == 0 {
		generatedLines = append(generatedLines, textToGeneratedLines(firstNonEmpty(notesPlain, markdownContent))...)
	}

	if content == nil && strings.TrimSpace(markdownContent) == "" && strings.TrimSpace(originalContent) == "" && len(generatedLines) == 0 {
		return nil
	}

	panel := model.PanelData{
		ID:               firstNonEmpty(toString(doc["summary_id"]), toString(doc["id"])+"-document"),
		DocumentID:       firstNonEmpty(toString(doc["id"]), documentID),
		Title:            firstNonEmpty(toString(doc["title"]), "Summary"),
		TemplateSlug:     firstNonEmpty(toString(doc["template_slug"]), "document-summary"),
		Markdown:         markdownContent,
		Content:          content,
		OriginalContent:  originalContent,
		GeneratedLines:   generatedLines,
		CreatedAt:        firstNonEmpty(toString(doc["created_at"]), toString(doc["createdAt"])),
		ContentUpdatedAt: firstNonEmpty(toString(doc["updated_at"]), toString(doc["updatedAt"]), toString(doc["created_at"]), toString(doc["createdAt"])),
		DeletedAt:        firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])),
	}
	return &panel
}

func transcriptLinesForDocument(transcripts map[string]any, candidateIDs ...string) []map[string]any {
	if len(transcripts) == 0 {
		return nil
	}

	for _, candidateID := range candidateIDs {
		id := strings.TrimSpace(candidateID)
		if id == "" {
			continue
		}

		raw := transcripts[id]
		items := toSliceOfMaps(raw)
		if len(items) == 0 {
			continue
		}

		lines := make([]map[string]any, 0, len(items))
		for _, item := range items {
			text := strings.TrimSpace(toString(item["text"]))
			if text == "" {
				continue
			}
			lines = append(lines, map[string]any{"text": text})
		}
		if len(lines) > 0 {
			return lines
		}
	}

	return nil
}

func transcriptBoundsForDocument(transcripts map[string]any, candidateIDs ...string) (string, string) {
	if len(transcripts) == 0 {
		return "", ""
	}

	for _, candidateID := range candidateIDs {
		id := strings.TrimSpace(candidateID)
		if id == "" {
			continue
		}

		items := toSliceOfMaps(transcripts[id])
		if len(items) == 0 {
			continue
		}

		startedAt := ""
		endedAt := ""
		for _, item := range items {
			if startedAt == "" {
				startedAt = firstNonEmpty(toString(item["start_timestamp"]), toString(item["startTimestamp"]))
			}
			endedAt = firstNonEmpty(
				toString(item["end_timestamp"]),
				toString(item["endTimestamp"]),
				toString(item["start_timestamp"]),
				toString(item["startTimestamp"]),
				endedAt,
			)
		}
		return startedAt, endedAt
	}

	return "", ""
}

func textToGeneratedLines(text string) []map[string]any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	result := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		result = append(result, map[string]any{"text": value})
	}
	return result
}

func hasStructuredText(content map[string]any) bool {
	if content == nil {
		return false
	}
	return hasStructuredTextAny(content)
}

func hasStructuredTextAny(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if strings.TrimSpace(toString(typed["text"])) != "" {
			return true
		}
		if hasStructuredTextAny(typed["content"]) {
			return true
		}
		if hasStructuredTextAny(typed["children"]) {
			return true
		}
		return false
	case []any:
		for _, item := range typed {
			if hasStructuredTextAny(item) {
				return true
			}
		}
		return false
	case []map[string]any:
		for _, item := range typed {
			if hasStructuredTextAny(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func SelectPrimaryPanel(panels []model.PanelData) *model.PanelData {
	if len(panels) == 0 {
		return nil
	}

	sortedPanels := append([]model.PanelData(nil), panels...)
	sort.Slice(sortedPanels, func(i, j int) bool {
		if sortedPanels[i].CreatedAt == sortedPanels[j].CreatedAt {
			return sortedPanels[i].ID < sortedPanels[j].ID
		}
		return sortedPanels[i].CreatedAt < sortedPanels[j].CreatedAt
	})

	for _, panel := range sortedPanels {
		if panel.TemplateSlug == "meeting-summary-consolidated" || panel.Title == "Summary" {
			selected := panel
			return &selected
		}
	}

	selected := sortedPanels[0]
	return &selected
}

func parseNonDeletedPanels(documentID string, rawPanels map[string]any) []model.PanelData {
	panels := make([]model.PanelData, 0, len(rawPanels))
	for panelID, rawPanel := range rawPanels {
		payload, ok := toMap(rawPanel)
		if !ok {
			continue
		}

		deletedAt := firstNonEmpty(toString(payload["deleted_at"]), toString(payload["deletedAt"]))
		if deletedAt != "" {
			continue
		}

		panel := model.PanelData{
			ID:               firstNonEmpty(toString(payload["id"]), panelID),
			DocumentID:       firstNonEmpty(toString(payload["document_id"]), toString(payload["documentId"]), documentID),
			Title:            firstNonEmpty(toString(payload["title"]), toString(payload["name"])),
			TemplateSlug:     firstNonEmpty(toString(payload["template_slug"]), toString(payload["templateSlug"])),
			Content:          toMapOrNil(payload["content"]),
			OriginalContent:  firstNonEmpty(toString(payload["original_content"]), toString(payload["originalContent"])),
			GeneratedLines:   toSliceOfMaps(payload["generated_lines"]),
			CreatedAt:        firstNonEmpty(toString(payload["created_at"]), toString(payload["createdAt"])),
			ContentUpdatedAt: firstNonEmpty(toString(payload["content_updated_at"]), toString(payload["updated_at"]), toString(payload["updatedAt"]), toString(payload["contentUpdatedAt"])),
			DeletedAt:        deletedAt,
		}
		panels = append(panels, panel)
	}
	return panels
}

func decodePayloadObject(root map[string]any) (map[string]any, error) {
	if serialized, ok := root["cache"].(string); ok {
		var payload any
		if err := json.Unmarshal([]byte(serialized), &payload); err != nil {
			return nil, newLoadError("top-level '.cache' field is not valid JSON")
		}
		payloadMap, ok := toMap(payload)
		if !ok {
			return nil, newLoadError("decoded '.cache' payload is not a JSON object")
		}
		return payloadMap, nil
	}

	if embeddedCache, ok := toMap(root["cache"]); ok {
		return embeddedCache, nil
	}

	return root, nil
}

func decodeStateObject(data []byte, sourcePath string) (map[string]any, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, newLoadError("invalid JSON in cache file: %s", sourcePath)
	}

	root, ok := toMap(raw)
	if !ok {
		return nil, newLoadError("cache payload must be a JSON object")
	}

	payloadObj, err := decodePayloadObject(root)
	if err != nil {
		return nil, err
	}

	state, ok := lookupMap(payloadObj, "state", "State")
	if !ok {
		return nil, newLoadError("decoded cache payload does not contain state object")
	}

	return state, nil
}

func lookupMap(container map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := container[key]; ok {
			if mapped, ok := toMap(value); ok {
				return mapped, true
			}
		}
	}
	return nil, false
}

func toMap(value any) (map[string]any, bool) {
	mapped, ok := value.(map[string]any)
	return mapped, ok
}

func toMapOrNil(value any) map[string]any {
	if mapped, ok := toMap(value); ok {
		return mapped
	}
	return nil
}

func firstNonNilMap(values ...any) map[string]any {
	for _, value := range values {
		if mapped, ok := toMap(value); ok {
			return mapped
		}
	}
	return nil
}

func toSliceOfMaps(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		if maps, ok := value.([]map[string]any); ok {
			return maps
		}
		return nil
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mapped, ok := toMap(item); ok {
			result = append(result, mapped)
		}
	}
	return result
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

func hasExportableDocumentState(state map[string]any) bool {
	if _, ok := lookupMap(state, "documents", "Documents", "docs", "Docs"); ok {
		return true
	}
	if _, ok := lookupMap(state, "documentPanels", "document_panels", "DocumentPanels", "panels"); ok {
		return true
	}
	if transcripts, ok := lookupMap(state, "transcripts", "Transcripts"); ok && len(transcripts) > 0 {
		return true
	}
	active := activeEditorState(state)
	return strings.TrimSpace(active.meetingID) != "" && strings.TrimSpace(active.markdown) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type activeEditorDocumentState struct {
	meetingID   string
	markdown    string
	panelID     string
	panelSlug   string
	documentIDs []string
}

func activeEditorState(state map[string]any) activeEditorDocumentState {
	multiChatState, ok := lookupMap(state, "multiChatState")
	if !ok {
		return activeEditorDocumentState{}
	}

	chatContext, ok := lookupMap(multiChatState, "chatContext")
	if !ok {
		return activeEditorDocumentState{}
	}

	documentIDs := toStringSlice(multiChatState["documentIds"])

	return activeEditorDocumentState{
		meetingID:   strings.TrimSpace(toString(chatContext["meetingId"])),
		markdown:    strings.TrimSpace(toString(chatContext["activeEditorMarkdown"])),
		panelID:     strings.TrimSpace(toString(chatContext["summaryPanelId"])),
		panelSlug:   strings.TrimSpace(toString(chatContext["summaryPanelSlug"])),
		documentIDs: documentIDs,
	}
}

func (s activeEditorDocumentState) markdownForDocument(candidateIDs ...string) string {
	if s.meetingID == "" || s.markdown == "" {
		return ""
	}

	for _, candidateID := range candidateIDs {
		if strings.TrimSpace(candidateID) == s.meetingID {
			return s.markdown
		}
	}
	return ""
}

func (s activeEditorDocumentState) panelIDForDocument(documentID string) string {
	if strings.TrimSpace(documentID) == "" || strings.TrimSpace(documentID) != s.meetingID {
		return documentID + "-summary"
	}
	return firstNonEmpty(s.panelID, documentID+"-summary")
}

func (s activeEditorDocumentState) panelSlugForDocument(documentID string) string {
	if strings.TrimSpace(documentID) == "" || strings.TrimSpace(documentID) != s.meetingID {
		return "meeting-summary-consolidated"
	}
	return firstNonEmpty(s.panelSlug, "meeting-summary-consolidated")
}

func toStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if strings, ok := value.([]string); ok {
			return append([]string(nil), strings...)
		}
		return nil
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(toString(item))
		if text == "" {
			continue
		}
		result = append(result, text)
	}
	return result
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
