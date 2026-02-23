package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func DefaultCachePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Granola", "cache-v3.json"), nil
}

func ResolveCachePath(override string) (string, error) {
	if strings.TrimSpace(override) == "" {
		return DefaultCachePath()
	}
	return override, nil
}
