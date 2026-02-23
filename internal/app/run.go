package app

import (
	"github.com/mynameiswhm/granola2markdown/internal/cache"
	"github.com/mynameiswhm/granola2markdown/internal/export"
	"github.com/mynameiswhm/granola2markdown/internal/model"
)

func RunExport(outputDir string, cachePath string, verbose bool) (model.ExportCounts, error) {
	state, err := cache.LoadCacheState(cachePath)
	if err != nil {
		return model.ExportCounts{}, err
	}

	candidates, err := cache.BuildCandidates(state)
	if err != nil {
		return model.ExportCounts{}, err
	}

	return export.ExportCandidates(candidates, outputDir, verbose)
}
