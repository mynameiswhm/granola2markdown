package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCachePath(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("cannot resolve user config dir in test environment: %v", err)
	}

	got, err := DefaultCachePath()
	if err != nil {
		t.Fatalf("DefaultCachePath() returned error: %v", err)
	}

	want := filepath.Join(configDir, "Granola", "cache-v3.json")
	if got != want {
		t.Fatalf("path mismatch: got %s want %s", got, want)
	}
}

func TestResolveCachePathOverride(t *testing.T) {
	override := "~/custom/cache-v3.json"
	resolved, err := ResolveCachePath(override)
	if err != nil {
		t.Fatalf("ResolveCachePath() returned error for override: %v", err)
	}
	if resolved != override {
		t.Fatalf("expected override path to be used as-is, got %s", resolved)
	}
}
