package cache

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
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

func TestLoadCacheStateUsesEncryptedSiblingWhenPresent(t *testing.T) {
	restore := stubDEKLoader(t, func(string) ([]byte, error) {
		return bytes.Repeat([]byte{0x42}, granolaCacheEncryptionKeySize), nil
	})
	defer restore()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v6.json")
	encPath := cachePath + ".enc"

	plainPayload := mustMarshalJSON(t, map[string]any{
		"state": map[string]any{
			"documents": map[string]any{},
		},
	})
	if err := os.WriteFile(cachePath, plainPayload, 0o644); err != nil {
		t.Fatalf("write plaintext cache failed: %v", err)
	}

	encPayload := mustMarshalJSON(t, map[string]any{
		"state": map[string]any{
			"documents": map[string]any{
				"doc-1": map[string]any{
					"id": "doc-1",
				},
			},
		},
	})
	if err := os.WriteFile(encPath, encryptGranolaPayloadForTest(t, encPayload, bytes.Repeat([]byte{0x42}, granolaCacheEncryptionKeySize)), 0o644); err != nil {
		t.Fatalf("write encrypted cache failed: %v", err)
	}

	state, err := LoadCacheState(cachePath)
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}

	documents, ok := lookupMap(state, "documents")
	if !ok {
		t.Fatalf("decrypted state should include documents")
	}
	if _, ok := documents["doc-1"]; !ok {
		t.Fatalf("expected decrypted encrypted cache payload to win")
	}
}

func TestLoadCacheStateErrorsWhenEncryptedSiblingCannotBeDecrypted(t *testing.T) {
	restore := stubDEKLoader(t, func(string) ([]byte, error) {
		return nil, errors.New("keychain unavailable")
	})
	defer restore()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v6.json")
	encPath := cachePath + ".enc"

	plainPayload := mustMarshalJSON(t, map[string]any{
		"state": map[string]any{
			"documents": map[string]any{
				"doc-plain": map[string]any{
					"id": "doc-plain",
				},
			},
		},
	})
	if err := os.WriteFile(cachePath, plainPayload, 0o644); err != nil {
		t.Fatalf("write plaintext cache failed: %v", err)
	}
	if err := os.WriteFile(encPath, []byte("not-a-real-encrypted-payload"), 0o644); err != nil {
		t.Fatalf("write encrypted cache failed: %v", err)
	}

	_, err := LoadCacheState(cachePath)
	if err == nil {
		t.Fatalf("expected decryption error when encrypted sibling is present")
	}
}

func TestLoadCacheStateErrorsWhenEncryptedStateLacksExportableNoteState(t *testing.T) {
	restore := stubDEKLoader(t, func(string) ([]byte, error) {
		return bytes.Repeat([]byte{0x31}, granolaCacheEncryptionKeySize), nil
	})
	defer restore()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v6.json")
	encPath := cachePath + ".enc"

	plainPayload := mustMarshalJSON(t, map[string]any{
		"state": map[string]any{
			"documents": map[string]any{
				"doc-plain": map[string]any{"id": "doc-plain"},
			},
			"documentPanels": map[string]any{},
		},
	})
	if err := os.WriteFile(cachePath, plainPayload, 0o644); err != nil {
		t.Fatalf("write plaintext cache failed: %v", err)
	}

	encPayload := mustMarshalJSON(t, map[string]any{
		"cache": map[string]any{
			"version": 8,
			"state": map[string]any{
				"transcripts":    map[string]any{},
				"multiChatState": map[string]any{},
			},
		},
	})
	if err := os.WriteFile(encPath, encryptGranolaPayloadForTest(t, encPayload, bytes.Repeat([]byte{0x31}, granolaCacheEncryptionKeySize)), 0o644); err != nil {
		t.Fatalf("write encrypted cache failed: %v", err)
	}

	_, err := LoadCacheState(cachePath)
	if err == nil {
		t.Fatalf("expected error when encrypted payload has no exportable note state")
	}
}

func TestLoadCacheStateDecryptsExplicitEncryptedPath(t *testing.T) {
	restore := stubDEKLoader(t, func(string) ([]byte, error) {
		return bytes.Repeat([]byte{0x24}, granolaCacheEncryptionKeySize), nil
	})
	defer restore()

	dir := t.TempDir()
	encPath := filepath.Join(dir, "cache-v6.json.enc")
	payload := mustMarshalJSON(t, map[string]any{
		"cache": map[string]any{
			"version": 8,
			"state": map[string]any{
				"transcripts": map[string]any{
					"doc-explicit": []any{
						map[string]any{
							"text":            "hello world",
							"start_timestamp": "2026-06-02T17:00:50.965Z",
							"end_timestamp":   "2026-06-02T17:00:55.935Z",
						},
					},
				},
			},
		},
	})
	if err := os.WriteFile(encPath, encryptGranolaPayloadForTest(t, payload, bytes.Repeat([]byte{0x24}, granolaCacheEncryptionKeySize)), 0o644); err != nil {
		t.Fatalf("write encrypted cache failed: %v", err)
	}

	state, err := LoadCacheState(encPath)
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}

	transcripts, ok := lookupMap(state, "transcripts")
	if !ok {
		t.Fatalf("decrypted state should include transcripts")
	}
	if _, ok := transcripts["doc-explicit"]; !ok {
		t.Fatalf("expected explicit encrypted cache path to be decrypted")
	}
}

func TestDecryptSafeStorageBlobWithPassword(t *testing.T) {
	password := "test-safe-storage-password"
	dek := bytes.Repeat([]byte{0x7a}, granolaCacheEncryptionKeySize)
	base64DEK := []byte(base64.StdEncoding.EncodeToString(dek))

	blob := encryptSafeStorageBlobForTest(t, base64DEK, password)
	got, err := decryptSafeStorageBlobWithPassword(blob, password)
	if err != nil {
		t.Fatalf("decryptSafeStorageBlobWithPassword failed: %v", err)
	}
	if !bytes.Equal(got, base64DEK) {
		t.Fatalf("safeStorage plaintext mismatch: got %q want %q", got, base64DEK)
	}
}

func TestDumpDecryptedCacheWritesRawJSON(t *testing.T) {
	restoreLoader := stubDEKLoader(t, func(string) ([]byte, error) {
		return bytes.Repeat([]byte{0x51}, granolaCacheEncryptionKeySize), nil
	})
	defer restoreLoader()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "cache-v6.json")
	encPath := cachePath + ".enc"
	outputPath := filepath.Join(dir, "dump", "cache.json")

	payload := mustMarshalJSON(t, map[string]any{
		"cache": map[string]any{
			"version": 8,
			"state": map[string]any{
				"transcripts": map[string]any{
					"doc-1": []any{
						map[string]any{"text": "hello"},
					},
				},
			},
		},
	})
	if err := os.WriteFile(encPath, encryptGranolaPayloadForTest(t, payload, bytes.Repeat([]byte{0x51}, granolaCacheEncryptionKeySize)), 0o644); err != nil {
		t.Fatalf("write encrypted cache failed: %v", err)
	}

	if err := DumpDecryptedCache(cachePath, outputPath); err != nil {
		t.Fatalf("DumpDecryptedCache failed: %v", err)
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("dumped payload mismatch: got %q want %q", got, payload)
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

func TestBuildCandidatesUsesActiveEditorMarkdownWithoutDocuments(t *testing.T) {
	state := map[string]any{
		"transcripts": map[string]any{
			"doc-1": []any{
				map[string]any{
					"text":            "transcript fallback",
					"start_timestamp": "2026-06-02T17:00:50.965Z",
					"end_timestamp":   "2026-06-02T17:00:55.935Z",
				},
			},
		},
		"multiChatState": map[string]any{
			"documentIds": []any{"doc-1"},
			"chatContext": map[string]any{
				"meetingId":            "doc-1",
				"activeEditorMarkdown": "### Rich summary\n\n- action item",
				"summaryPanelId":       "panel-1",
				"summaryPanelSlug":     "meeting-summary-consolidated",
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
		t.Fatalf("unexpected document id: %q", candidates[0].Document.ID)
	}
	if candidates[0].Document.Title != "Rich summary" {
		t.Fatalf("unexpected document title: %q", candidates[0].Document.Title)
	}
	if candidates[0].Document.CreatedAt != "2026-06-02T17:00:50.965Z" {
		t.Fatalf("unexpected created_at: %q", candidates[0].Document.CreatedAt)
	}
	if candidates[0].Panel.ID != "panel-1" {
		t.Fatalf("unexpected panel id: %q", candidates[0].Panel.ID)
	}
	if candidates[0].Panel.ContentUpdatedAt != "2026-06-02T17:00:55.935Z" {
		t.Fatalf("unexpected content_updated_at: %q", candidates[0].Panel.ContentUpdatedAt)
	}
	if got := candidates[0].Panel.Markdown; got != "### Rich summary\n\n- action item" {
		t.Fatalf("unexpected panel markdown: %q", got)
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

func mustMarshalJSON(t *testing.T, payload any) []byte {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func stubDEKLoader(t *testing.T, fn func(string) ([]byte, error)) func() {
	t.Helper()
	previous := loadDEKForCachePathFunc
	loadDEKForCachePathFunc = fn
	return func() {
		loadDEKForCachePathFunc = previous
	}
}

func encryptGranolaPayloadForTest(t *testing.T, plaintext []byte, dek []byte) []byte {
	t.Helper()

	block, err := aes.NewCipher(dek)
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	gcm, err := cipher.NewGCMWithTagSize(block, granolaCacheGCMTagSize)
	if err != nil {
		t.Fatalf("cipher.NewGCMWithTagSize failed: %v", err)
	}

	nonce := bytes.Repeat([]byte{0x09}, granolaCacheGCMNonceSize)
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	ciphertext := sealed[:len(sealed)-granolaCacheGCMTagSize]
	tag := sealed[len(sealed)-granolaCacheGCMTagSize:]

	payload := make([]byte, 0, len(nonce)+len(ciphertext)+len(tag))
	payload = append(payload, nonce...)
	payload = append(payload, ciphertext...)
	payload = append(payload, tag...)
	return payload
}

func encryptSafeStorageBlobForTest(t *testing.T, plaintext []byte, password string) []byte {
	t.Helper()

	key := pbkdf2SHA1([]byte(password), []byte("saltysalt"), safeStoragePBKDF2Iterations, safeStorageDerivedKeyLength)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}

	padLen := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	padded := append([]byte(nil), plaintext...)
	padded = append(padded, bytes.Repeat([]byte{byte(padLen)}, padLen)...)

	iv := bytes.Repeat([]byte(" "), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	payload := append([]byte(safeStoragePrefix), ciphertext...)
	return payload
}
