package paths

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const latestKnownCacheVersion = 6

var cacheFileNamePattern = regexp.MustCompile(`^cache-v([0-9]+)\.json$`)

func DefaultCachePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	granolaConfigDir := filepath.Join(configDir, "Granola")
	return selectDefaultCachePath(granolaConfigDir), nil
}

func ResolveCachePath(override string) (string, error) {
	if strings.TrimSpace(override) == "" {
		return DefaultCachePath()
	}
	return override, nil
}

func selectDefaultCachePath(granolaConfigDir string) string {
	bestPath, ok := newestVersionedCachePath(granolaConfigDir)
	if ok {
		return bestPath
	}

	return filepath.Join(granolaConfigDir, fallbackCacheFileName())
}

func newestVersionedCachePath(granolaConfigDir string) (string, bool) {
	entries, err := os.ReadDir(granolaConfigDir)
	if err != nil {
		return "", false
	}

	bestVersion := -1
	bestName := ""
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := cacheFileNamePattern.FindStringSubmatch(entry.Name())
		if len(matches) != 2 {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		if version > bestVersion {
			bestVersion = version
			bestName = entry.Name()
		}
	}

	if bestVersion < 0 {
		return "", false
	}

	return filepath.Join(granolaConfigDir, bestName), true
}

func fallbackCacheFileName() string {
	return "cache-v" + strconv.Itoa(latestKnownCacheVersion) + ".json"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
