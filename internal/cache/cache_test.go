package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCacheStateAndCandidates(t *testing.T) {
	root := findRepoRoot(t)
	state, err := LoadCacheState(filepath.Join(root, "cache-v3.json"))
	if err != nil {
		t.Fatalf("LoadCacheState failed: %v", err)
	}

	if _, ok := state["documents"]; !ok {
		t.Fatalf("state should include documents")
	}
	if _, ok := state["documentPanels"]; !ok {
		t.Fatalf("state should include documentPanels")
	}

	candidates, err := BuildCandidates(state)
	if err != nil {
		t.Fatalf("BuildCandidates failed: %v", err)
	}
	if len(candidates) != 11 {
		t.Fatalf("expected 11 candidates from fixture, got %d", len(candidates))
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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}
	for {
		candidate := filepath.Join(cwd, "cache-v3.json")
		if _, err := os.Stat(candidate); err == nil {
			return cwd
		}
		next := filepath.Dir(cwd)
		if next == cwd {
			t.Fatalf("could not locate repository root from %s", cwd)
		}
		cwd = next
	}
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
