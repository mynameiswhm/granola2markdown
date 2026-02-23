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
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read cache file: %w", err)
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, newLoadError("invalid JSON in cache file: %s", cachePath)
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

func BuildCandidates(state map[string]any) ([]model.NoteCandidate, error) {
	documents, ok := lookupMap(state, "documents", "Documents", "docs", "Docs")
	if !ok {
		return nil, newLoadError("state.documents is missing or invalid")
	}

	documentPanels, ok := lookupMap(state, "documentPanels", "document_panels", "DocumentPanels", "panels")
	if !ok {
		return nil, newLoadError("state.documentPanels is missing or invalid")
	}

	candidates := make([]model.NoteCandidate, 0, len(documents))
	for documentID, rawDoc := range documents {
		doc, ok := toMap(rawDoc)
		if !ok {
			continue
		}
		if firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])) != "" {
			continue
		}

		docPanelsRaw, ok := documentPanels[documentID]
		if !ok {
			altID := toString(doc["id"])
			if altID != "" {
				docPanelsRaw = documentPanels[altID]
			}
		}

		docPanels, ok := toMap(docPanelsRaw)
		if !ok {
			continue
		}

		nonDeletedPanels := parseNonDeletedPanels(documentID, docPanels)
		if len(nonDeletedPanels) == 0 {
			continue
		}

		selected := SelectPrimaryPanel(nonDeletedPanels)
		if selected == nil {
			continue
		}

		candidate := model.NoteCandidate{
			Document: model.DocumentData{
				ID:        documentID,
				CreatedAt: firstNonEmpty(toString(doc["created_at"]), toString(doc["createdAt"])),
				DeletedAt: firstNonEmpty(toString(doc["deleted_at"]), toString(doc["deletedAt"])),
			},
			Panel: *selected,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
