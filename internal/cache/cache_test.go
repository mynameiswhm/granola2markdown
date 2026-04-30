package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestLoadCacheStateAndCandidates(t *testing.T) {
	state, err := LoadCacheState(findFixtureCachePath(t))
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}

	if _, ok := state["documents"]; !ok {
		t.Fatalf("state should include documents")
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected at least 1 candidate from fixture, got %d", len(candidates))
	}
}

func TestInvalidSerializedCacheRaises(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v3.json")
	payload := map[string]any{"cache": "{invalid"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(cachePath, encoded, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_, err = LoadCacheState(cachePath)
	if err == nil {
		t.Fatalf("expected error for invalid serialized cache")
	}
	var loadErr *LoadError
	if !asLoadError(err, &loadErr) {
		t.Fatalf("expected LoadError, got %T", err)
	}
}

func TestDirectObjectPayloadAccepted(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v3.json")
	payload := map[string]any{
		"state": map[string]any{
			"documents":      map[string]any{},
			"documentPanels": map[string]any{},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(cachePath, encoded, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	state, err := LoadCacheState(cachePath)
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}
	if _, ok := state["documents"]; !ok {
		t.Fatalf("state should include documents")
	}
}

func TestCacheObjectPayloadAcceptedAcrossVersionedCacheFiles(t *testing.T) {
	for _, fileName := range []string{"cache-v3.json", "cache-v4.json", "cache-v6.json", "cache-v7.json"} {
		t.Run(fileName, func(t *testing.T) {
			dir := t.TempDir()
			cachePath := filepath.Join(dir, fileName)
			payload := map[string]any{
				"cache": map[string]any{
					"state": map[string]any{
						"documents":      map[string]any{},
						"documentPanels": map[string]any{},
					},
				},
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if err := os.WriteFile(cachePath, encoded, 0o644); err != nil {
				t.Fatalf("write failed: %v", err)
			}

			state, err := LoadCacheState(cachePath)
			if err != nil {
				t.Fatalf("LoadCacheState failed: %v", err)
			}
			if _, ok := state["documents"]; !ok {
				t.Fatalf("state should include documents")
			}
			if _, ok := state["documentPanels"]; !ok {
				t.Fatalf("state should include documentPanels")
			}
		})
	}
}

func TestInvalidSerializedCacheAcrossVersionedCacheFilesRaises(t *testing.T) {
	for _, fileName := range []string{"cache-v3.json", "cache-v4.json", "cache-v6.json", "cache-v7.json"} {
		t.Run(fileName, func(t *testing.T) {
			dir := t.TempDir()
			cachePath := filepath.Join(dir, fileName)
			payload := map[string]any{"cache": "{invalid"}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if err := os.WriteFile(cachePath, encoded, 0o644); err != nil {
				t.Fatalf("write failed: %v", err)
			}

			_, err = LoadCacheState(cachePath)
			if err == nil {
				t.Fatalf("expected error for invalid serialized cache")
			}
			var loadErr *LoadError
			if !asLoadError(err, &loadErr) {
				t.Fatalf("expected LoadError, got %T", err)
			}
		})
	}
}

func TestV4CacheObjectNormalizationAndPanelSelection(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v4.json")
	payload := map[string]any{
		"cache": map[string]any{
			"state": map[string]any{
				"documents": map[string]any{
					"doc-1": map[string]any{
						"id":         "doc-1",
						"created_at": "2026-02-13T14:20:00.000Z",
						"deleted_at": nil,
					},
					"doc-2": map[string]any{
						"id":         "doc-2",
						"created_at": "2026-02-13T15:20:00.000Z",
						"deleted_at": "2026-02-13T15:30:00.000Z",
					},
				},
				"documentPanels": map[string]any{
					"doc-1": map[string]any{
						"panel-raw": map[string]any{
							"id":                 "panel-raw",
							"document_id":        "doc-1",
							"title":              "Raw",
							"created_at":         "2026-02-13T14:20:00.000Z",
							"content_updated_at": "2026-02-13T14:20:00.000Z",
						},
						"panel-summary": map[string]any{
							"id":                 "panel-summary",
							"document_id":        "doc-1",
							"title":              "Summary",
							"template_slug":      "meeting-summary-consolidated",
							"created_at":         "2026-02-13T14:21:00.000Z",
							"content_updated_at": "2026-02-13T14:21:00.000Z",
						},
					},
					"doc-2": map[string]any{
						"panel-deleted-doc": map[string]any{
							"id":                 "panel-deleted-doc",
							"document_id":        "doc-2",
							"template_slug":      "meeting-summary-consolidated",
							"created_at":         "2026-02-13T15:20:00.000Z",
							"content_updated_at": "2026-02-13T15:20:00.000Z",
						},
					},
				},
			},
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(cachePath, encoded, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	state, err := LoadCacheState(cachePath)
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}
	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Document.ID != "doc-1" {
		t.Fatalf("expected candidate document doc-1, got %s", candidates[0].Document.ID)
	}
	if candidates[0].Panel.ID != "panel-summary" {
		t.Fatalf("expected summary panel selection, got %s", candidates[0].Panel.ID)
	}
}

func TestBuildCandidatesFallsBackToDocumentContentWhenPanelsMissing(t *testing.T) {
	state := map[string]any{
		"documents": map[string]any{
			"doc-1": map[string]any{
				"id":         "doc-1",
				"title":      "Sync notes",
				"created_at": "2026-02-27T10:00:00.000Z",
				"updated_at": "2026-02-27T10:05:00.000Z",
				"notes": map[string]any{
					"type": "doc",
					"content": []any{
						map[string]any{
							"type": "heading",
							"attrs": map[string]any{
								"level": 3,
							},
							"content": []any{
								map[string]any{
									"type": "text",
									"text": "Weekly sync",
								},
							},
						},
					},
				},
			},
		},
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].Document.ID != "doc-1" {
		t.Fatalf("expected candidate for doc-1, got %s", candidates[0].Document.ID)
	}
	if candidates[0].Panel.Content == nil {
		t.Fatalf("expected synthesized panel content from document notes")
	}
}

func TestBuildCandidatesUsesActiveEditorMarkdownForMatchingMeeting(t *testing.T) {
	state := map[string]any{
		"documents": map[string]any{
			"doc-1": map[string]any{
				"id":         "doc-1",
				"title":      "Weekly sync",
				"created_at": "2026-04-27T10:00:00.000Z",
				"updated_at": "2026-04-27T10:05:00.000Z",
			},
		},
		"transcripts": map[string]any{
			"doc-1": []any{
				map[string]any{"text": "transcript fallback"},
			},
		},
		"multiChatState": map[string]any{
			"chatContext": map[string]any{
				"meetingId":            "doc-1",
				"activeEditorMarkdown": "### Rich summary\n\n- action item",
			},
		},
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if got := candidates[0].Panel.Markdown; got != "### Rich summary\n\n- action item" {
		t.Fatalf("unexpected panel markdown: %q", got)
	}
	if got := candidates[0].Panel.GeneratedLines[0]["text"]; got != "transcript fallback" {
		t.Fatalf("expected transcript fallback to remain available, got %v", got)
	}
}

func TestBuildCandidatesSkipsPlaceholderNotesWithoutTextFallback(t *testing.T) {
	state := map[string]any{
		"documents": map[string]any{
			"doc-1": map[string]any{
				"id":         "doc-1",
				"title":      "Fallback title",
				"created_at": "2026-02-27T10:00:00.000Z",
				"updated_at": "2026-02-27T10:05:00.000Z",
				"notes": map[string]any{
					"type": "doc",
					"content": []any{
						map[string]any{
							"type":  "paragraph",
							"attrs": map[string]any{"id": "placeholder"},
						},
					},
				},
			},
		},
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates for placeholder-only notes, got %d", len(candidates))
	}
}

func TestBuildCandidatesFallsBackToTranscriptText(t *testing.T) {
	state := map[string]any{
		"documents": map[string]any{
			"doc-1": map[string]any{
				"id":         "doc-1",
				"title":      "Transcript meeting",
				"created_at": "2026-02-27T10:00:00.000Z",
				"updated_at": "2026-02-27T10:05:00.000Z",
				"notes": map[string]any{
					"type": "doc",
					"content": []any{
						map[string]any{
							"type":  "paragraph",
							"attrs": map[string]any{"id": "placeholder"},
						},
					},
				},
			},
		},
		"transcripts": map[string]any{
			"doc-1": []any{
				map[string]any{"text": "first line"},
				map[string]any{"text": "second line"},
			},
		},
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if len(candidates[0].Panel.GeneratedLines) < 2 {
		t.Fatalf("expected transcript-derived generated lines, got %#v", candidates[0].Panel.GeneratedLines)
	}
	if candidates[0].Panel.GeneratedLines[0]["text"] != "first line" {
		t.Fatalf("unexpected transcript line: %#v", candidates[0].Panel.GeneratedLines[0])
	}
}

func TestFallbackKeysAreAccepted(t *testing.T) {
	state := map[string]any{
		"Documents": map[string]any{
			"doc-1": map[string]any{
				"created_at": "2026-02-13T14:20:00.000Z",
			},
		},
		"document_panels": map[string]any{
			"doc-1": map[string]any{
				"panel-1": map[string]any{
					"id":                 "panel-1",
					"document_id":        "doc-1",
					"template_slug":      "meeting-summary-consolidated",
					"content_updated_at": "2026-02-13T14:20:00.000Z",
					"created_at":         "2026-02-13T14:20:00.000Z",
					"content": map[string]any{
						"type": "doc",
						"content": []any{
							map[string]any{
								"type":    "heading",
								"attrs":   map[string]any{"level": 3},
								"content": []any{map[string]any{"type": "text", "text": "Hello"}},
							},
						},
					},
				},
			},
		},
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func findFixtureCachePath(t *testing.T) string {
	t.Helper()
	root := findRepoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "cache-v*.json"))
	if err != nil {
		t.Fatalf("cache fixture glob failed: %v", err)
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(root, "cache-v*-pretty*.json"))
		if err != nil {
			t.Fatalf("cache fixture glob failed: %v", err)
		}
	}
	if len(matches) == 0 {
		t.Fatalf("could not locate a cache fixture under %s", root)
	}
	sort.Strings(matches)
	return matches[0]
}

func asLoadError(err error, target **LoadError) bool {
	if err == nil {
		return false
	}
	loadErr, ok := err.(*LoadError)
	if ok {
		*target = loadErr
		return true
	}
	unwrapped := unwrap(err)
	if unwrapped == nil {
		return false
	}
	return asLoadError(unwrapped, target)
}

func unwrap(err error) error {
	type wrapper interface {
		Unwrap() error
	}
	if w, ok := err.(wrapper); ok {
		return w.Unwrap()
	}
	return nil
}
