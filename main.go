package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mynameiswhm/granola2markdown/internal/app"
	"github.com/mynameiswhm/granola2markdown/internal/cache"
	"github.com/mynameiswhm/granola2markdown/internal/paths"
	"github.com/mynameiswhm/granola2markdown/internal/watchman"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	return runWithManager(args, watchman.NewManager(), os.Stdout, os.Stderr)
}

type watchmanManager interface {
	Install(outputDir string, cachePath string) (watchman.InstallResult, error)
	Uninstall(outputDir string, cachePath string) (watchman.UninstallResult, error)
}

func runWithManager(args []string, manager watchmanManager, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "watchman" {
		return runWatchman(args[1:], manager, stdout, stderr)
	}
	return runExport(args, stdout, stderr)
}

func runExport(args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("granola2markdown", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cachePathFlag := fs.String("cache-path", "", "Override path to Granola cache-v3.json")
	verbose := fs.Bool("verbose", false, "Show detailed export decisions")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: %s [--cache-path PATH] [--verbose] <output_dir>\n", fs.Name())
		fmt.Fprintf(fs.Output(), "       %s watchman <install|uninstall> [--cache-path PATH] <output_dir>\n", fs.Name())
		fmt.Fprintln(fs.Output(), "Export Granola meeting notes from cache-v3.json to Markdown files.")
		fmt.Fprintln(fs.Output(), "Manage Watchman triggers for background note export.")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fs.Usage()
		return 2
	}

	outputDir := positional[0]
	cachePath, err := paths.ResolveCachePath(*cachePathFlag)
	if err != nil {
		fmt.Fprintf(stderr, "Fatal: cannot determine default cache path: %v\n", err)
		return 1
	}

	counts, err := app.RunExport(outputDir, cachePath, *verbose)
	if err != nil {
		var loadErr *cache.LoadError
		switch {
		case errors.Is(err, os.ErrNotExist):
			fmt.Fprintf(stderr, "Fatal: cache file not found: %s\n", cachePath)
		case errors.Is(err, os.ErrPermission):
			fmt.Fprintf(stderr, "Fatal: cannot access file system path: %v\n", err)
		case errors.As(err, &loadErr):
			fmt.Fprintf(stderr, "Fatal: %s\n", loadErr.Error())
		default:
			fmt.Fprintf(stderr, "Fatal: file operation failed: %v\n", err)
		}
		return 1
	}

	fmt.Fprintln(stdout, "Export summary")
	fmt.Fprintf(stdout, "  exported: %d\n", counts.Exported)
	fmt.Fprintf(stdout, "  updated:  %d\n", counts.Updated)
	fmt.Fprintf(stdout, "  skipped:  %d\n", counts.Skipped)
	fmt.Fprintf(stdout, "  errors:   %d\n", counts.Errors)
	return 0
}

func runWatchman(args []string, manager watchmanManager, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("granola2markdown watchman", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: granola2markdown watchman <install|uninstall> [--cache-path PATH] <output_dir>")
		fmt.Fprintln(fs.Output(), "Subcommands:")
		fmt.Fprintln(fs.Output(), "  install    Configure Watchman trigger for cache-v3.json updates")
		fmt.Fprintln(fs.Output(), "  uninstall  Remove Watchman trigger for the provided output directory")
	}

	if len(args) == 0 {
		fs.Usage()
		return 2
	}

	switch args[0] {
	case "help", "--help", "-h":
		fs.Usage()
		return 0
	case "install":
		return runWatchmanInstall(args[1:], manager, stdout, stderr)
	case "uninstall":
		return runWatchmanUninstall(args[1:], manager, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Fatal: unknown watchman subcommand: %s\n", args[0])
		fs.Usage()
		return 2
	}
}

func runWatchmanInstall(args []string, manager watchmanManager, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("granola2markdown watchman install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cachePathFlag := fs.String("cache-path", "", "Override path to Granola cache-v3.json")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: granola2markdown watchman install [--cache-path PATH] <output_dir>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fs.Usage()
		return 2
	}

	cachePath, err := paths.ResolveCachePath(*cachePathFlag)
	if err != nil {
		fmt.Fprintf(stderr, "Fatal: cannot determine default cache path: %v\n", err)
		return 1
	}

	result, err := manager.Install(positional[0], cachePath)
	if err != nil {
		if errors.Is(err, watchman.ErrDependencyMissing) {
			fmt.Fprintln(stderr, "Fatal: watchman executable not found on PATH.")
			fmt.Fprintln(stderr, "Install it with: brew install watchman")
			fmt.Fprintln(stderr, "See README.md for manual setup alternatives.")
			return 1
		}
		fmt.Fprintf(stderr, "Fatal: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "Watchman trigger installed.")
	fmt.Fprintf(stdout, "  trigger:   %s\n", result.TriggerName)
	fmt.Fprintf(stdout, "  watch root: %s\n", result.WatchRoot)
	fmt.Fprintf(stdout, "  output dir: %s\n", result.OutputDir)
	return 0
}

func runWatchmanUninstall(args []string, manager watchmanManager, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("granola2markdown watchman uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cachePathFlag := fs.String("cache-path", "", "Override path to Granola cache-v3.json")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: granola2markdown watchman uninstall [--cache-path PATH] <output_dir>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fs.Usage()
		return 2
	}

	cachePath, err := paths.ResolveCachePath(*cachePathFlag)
	if err != nil {
		fmt.Fprintf(stderr, "Fatal: cannot determine default cache path: %v\n", err)
		return 1
	}

	result, err := manager.Uninstall(positional[0], cachePath)
	if err != nil {
		if errors.Is(err, watchman.ErrDependencyMissing) {
			fmt.Fprintln(stderr, "Fatal: watchman executable not found on PATH.")
			fmt.Fprintln(stderr, "Install it with: brew install watchman")
			fmt.Fprintln(stderr, "See README.md for manual setup alternatives.")
			return 1
		}
		fmt.Fprintf(stderr, "Fatal: %v\n", err)
		return 1
	}

	if !result.Removed {
		fmt.Fprintln(stdout, "No matching Watchman trigger found; nothing to uninstall.")
		fmt.Fprintf(stdout, "  trigger:   %s\n", result.TriggerName)
		return 0
	}

	fmt.Fprintln(stdout, "Watchman trigger removed.")
	fmt.Fprintf(stdout, "  trigger:   %s\n", result.TriggerName)
	fmt.Fprintf(stdout, "  watch root: %s\n", result.WatchRoot)
	if strings.TrimSpace(result.OutputDir) != "" {
		fmt.Fprintf(stdout, "  output dir: %s\n", result.OutputDir)
	}
	return 0
}
