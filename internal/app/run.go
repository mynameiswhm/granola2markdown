package app

import (
	"fmt"

	"github.com/mynameiswhm/granola2markdown/internal/cache"
	"github.com/mynameiswhm/granola2markdown/internal/export"
	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func RunExport(outputDir string, cachePath string, verbose bool) (model.ExportCounts, error) {
	candidates, source, err := buildCandidates(cachePath)
	if err != nil {
		return model.ExportCounts{}, err
	}

	if verbose {
		fmt.Printf("source: %s\n", source)
	}

	return export.ExportCandidates(candidates, outputDir, verbose)
}

func buildCandidates(cachePath string) ([]model.NoteCandidate, string, error) {
	state, err := cache.LoadCacheState(cachePath)
	if err != nil {
		return nil, "", err
	}

	candidates, err := cache.BuildCandidates(state)
	if err != nil {
		return nil, "", err
	}

	return candidates, "cache", nil
}
