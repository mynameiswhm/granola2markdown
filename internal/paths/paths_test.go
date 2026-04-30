package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCachePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("cannot resolve user config dir in test environment: %v", err)
	}

	granolaDir := filepath.Join(configDir, "Granola")
	if err := os.MkdirAll(granolaDir, 0o755); err != nil {
		t.Fatalf("failed to create Granola config dir: %v", err)
	}

	v6Path := filepath.Join(granolaDir, "cache-v6.json")
	if err := os.WriteFile(v6Path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write cache-v6 fixture: %v", err)
	}
	v4Path := filepath.Join(granolaDir, "cache-v4.json")
	if err := os.WriteFile(v4Path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write cache-v4 fixture: %v", err)
	}
	got, err := DefaultCachePath()
	if err != nil {
		t.Fatalf("DefaultCachePath() returned error: %v", err)
	}

	want := filepath.Join(configDir, "Granola", "cache-v6.json")
	if got != want {
		t.Fatalf("path mismatch: got %s want %s", got, want)
	}
}

func TestSelectDefaultCachePathFallsBackToOlderVersion(t *testing.T) {
	granolaDir := t.TempDir()
	v3Path := filepath.Join(granolaDir, "cache-v3.json")
	if err := os.WriteFile(v3Path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write cache-v3 fixture: %v", err)
	}

	got := selectDefaultCachePath(granolaDir)
	if got != v3Path {
		t.Fatalf("path mismatch: got %s want %s", got, v3Path)
	}
}

func TestSelectDefaultCachePathPrefersFutureVersionWhenPresent(t *testing.T) {
	granolaDir := t.TempDir()
	v6Path := filepath.Join(granolaDir, "cache-v6.json")
	if err := os.WriteFile(v6Path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write cache-v6 fixture: %v", err)
	}
	v7Path := filepath.Join(granolaDir, "cache-v7.json")
	if err := os.WriteFile(v7Path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write cache-v7 fixture: %v", err)
	}

	got := selectDefaultCachePath(granolaDir)
	if got != v7Path {
		t.Fatalf("path mismatch: got %s want %s", got, v7Path)
	}
}

func TestSelectDefaultCachePathDefaultsToLatestKnownWhenNoFileExists(t *testing.T) {
	granolaDir := t.TempDir()
	got := selectDefaultCachePath(granolaDir)
	want := filepath.Join(granolaDir, fallbackCacheFileName())
	if got != want {
		t.Fatalf("path mismatch: got %s want %s", got, want)
	}
}

func TestResolveCachePathOverride(t *testing.T) {
	override := "~/custom/cache-v6.json"
	resolved, err := ResolveCachePath(override)
	if err != nil {
		t.Fatalf("ResolveCachePath() returned error for override: %v", err)
	}
	if resolved != override {
		t.Fatalf("expected override path to be used as-is, got %s", resolved)
	}
}
