package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/mynameiswhm/granola2markdown/internal/watchman"
)

type fakeWatchmanManager struct {
	installResult   watchman.InstallResult
	uninstallResult watchman.UninstallResult
	installErr      error
	uninstallErr    error
}

func (f fakeWatchmanManager) Install(outputDir string, cachePath string) (watchman.InstallResult, error) {
	if f.installResult.TriggerName == "" {
		f.installResult.TriggerName = "granola2markdown-test"
	}
	return f.installResult, f.installErr
}

func (f fakeWatchmanManager) Uninstall(outputDir string, cachePath string) (watchman.UninstallResult, error) {
	if f.uninstallResult.TriggerName == "" {
		f.uninstallResult.TriggerName = "granola2markdown-test"
	}
	return f.uninstallResult, f.uninstallErr
}

func TestRunWithHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithManager([]string{"--help"}, fakeWatchmanManager{}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for --help, got %d", exitCode)
	}
}

func TestRunRequiresOutputDir(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithManager([]string{}, fakeWatchmanManager{}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 when missing output dir, got %d", exitCode)
	}
}

func TestTopLevelHelpMentionsWatchmanCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithManager([]string{"--help"}, fakeWatchmanManager{}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0 for --help, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "watchman <install|uninstall>") {
		t.Fatalf("expected help to mention watchman subcommands, got:\n%s", stderr.String())
	}
}

func TestWatchmanInstallRequiresOutputDir(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithManager([]string{"watchman", "install"}, fakeWatchmanManager{}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 when watchman install has no output dir, got %d", exitCode)
	}
}

func TestWatchmanUninstallRequiresOutputDir(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithManager([]string{"watchman", "uninstall"}, fakeWatchmanManager{}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2 when watchman uninstall has no output dir, got %d", exitCode)
	}
}

func TestWatchmanMissingDependencyGivesActionableMessage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	manager := fakeWatchmanManager{
		installErr: &watchman.DependencyError{Binary: "watchman", Err: exec.ErrNotFound},
	}
	exitCode := runWithManager([]string{"watchman", "install", "./notes"}, manager, &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1 for missing watchman, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "brew install watchman") {
		t.Fatalf("expected actionable install guidance, got:\n%s", stderr.String())
	}
}
