package watchman

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const binaryName = "watchman"

var ErrDependencyMissing = errors.New("required dependency missing")

type DependencyError struct {
	Binary string
	Err    error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("%s executable not found: %v", e.Binary, e.Err)
}

func (e *DependencyError) Unwrap() error {
	return ErrDependencyMissing
}

type CommandError struct {
	Name   string
	Args   []string
	Stdout string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	cmd := strings.Join(append([]string{e.Name}, e.Args...), " ")
	if text := strings.TrimSpace(e.Stderr); text != "" {
		return fmt.Sprintf("%s failed: %s", cmd, text)
	}
	if text := strings.TrimSpace(e.Stdout); text != "" {
		return fmt.Sprintf("%s failed: %s", cmd, text)
	}
	return fmt.Sprintf("%s failed: %v", cmd, e.Err)
}

func (e *CommandError) Unwrap() error {
	return e.Err
}

type LookPathFunc func(file string) (string, error)

type RunFunc func(name string, args []string, stdin []byte) (stdout string, stderr string, err error)

type Manager struct {
	lookPath LookPathFunc
	run      RunFunc
	binary   string
}

type InstallOptions struct {
	OutputDir string
	WatchRoot string
	CachePath string
}

type UninstallOptions struct {
	OutputDir string
	WatchRoot string
}

type InstallResult struct {
	TriggerName string
	WatchRoot   string
	OutputDir   string
}

type UninstallResult struct {
	TriggerName string
	WatchRoot   string
	OutputDir   string
	Removed     bool
}

func NewManager() *Manager {
	return NewManagerWithDeps(exec.LookPath, runCommand)
}

func NewManagerWithDeps(lookPath LookPathFunc, run RunFunc) *Manager {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if run == nil {
		run = runCommand
	}
	return &Manager{
		lookPath: lookPath,
		run:      run,
		binary:   binaryName,
	}
}

func TriggerNameForOutputDir(outputDir string) (string, error) {
	normalized, err := normalizeOutputDir(outputDir)
	if err != nil {
		return "", err
	}
	return deriveTriggerName(normalized), nil
}

func (m *Manager) Install(options InstallOptions) (InstallResult, error) {
	if err := m.ensureDependency(); err != nil {
		return InstallResult{}, err
	}

	normalizedOutput, err := normalizeOutputDir(options.OutputDir)
	if err != nil {
		return InstallResult{}, err
	}
	watchRoot, err := normalizeWatchRoot(options.WatchRoot)
	if err != nil {
		return InstallResult{}, err
	}

	triggerName := deriveTriggerName(normalizedOutput)

	if _, _, err := m.exec(m.binary, []string{"watch-project", watchRoot}, nil); err != nil {
		return InstallResult{}, err
	}

	payload, err := buildInstallPayload(triggerName, watchRoot, normalizedOutput, options.CachePath)
	if err != nil {
		return InstallResult{}, err
	}

	if _, _, err := m.exec(m.binary, []string{"-j"}, payload); err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		TriggerName: triggerName,
		WatchRoot:   watchRoot,
		OutputDir:   normalizedOutput,
	}, nil
}

func (m *Manager) Uninstall(options UninstallOptions) (UninstallResult, error) {
	if err := m.ensureDependency(); err != nil {
		return UninstallResult{}, err
	}

	normalizedOutput, err := normalizeOutputDir(options.OutputDir)
	if err != nil {
		return UninstallResult{}, err
	}
	watchRoot, err := normalizeWatchRoot(options.WatchRoot)
	if err != nil {
		return UninstallResult{}, err
	}

	triggerName := deriveTriggerName(normalizedOutput)

	stdout, stderr, err := m.run(m.binary, []string{"trigger-del", watchRoot, triggerName}, nil)
	if err != nil {
		if missingTriggerError(stdout, stderr, err) {
			return UninstallResult{
				TriggerName: triggerName,
				WatchRoot:   watchRoot,
				OutputDir:   normalizedOutput,
				Removed:     false,
			}, nil
		}
		return UninstallResult{}, &CommandError{
			Name:   m.binary,
			Args:   []string{"trigger-del", watchRoot, triggerName},
			Stdout: stdout,
			Stderr: stderr,
			Err:    err,
		}
	}

	return UninstallResult{
		TriggerName: triggerName,
		WatchRoot:   watchRoot,
		OutputDir:   normalizedOutput,
		Removed:     true,
	}, nil
}

func (m *Manager) ensureDependency() error {
	if _, err := m.lookPath(m.binary); err != nil {
		return &DependencyError{
			Binary: m.binary,
			Err:    err,
		}
	}
	return nil
}

func (m *Manager) exec(name string, args []string, stdin []byte) (string, string, error) {
	stdout, stderr, err := m.run(name, args, stdin)
	if err != nil {
		return stdout, stderr, &CommandError{
			Name:   name,
			Args:   append([]string(nil), args...),
			Stdout: stdout,
			Stderr: stderr,
			Err:    err,
		}
	}
	return stdout, stderr, nil
}

func runCommand(name string, args []string, stdin []byte) (string, string, error) {
	cmd := exec.Command(name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

func normalizeWatchRoot(watchRoot string) (string, error) {
	trimmed := strings.TrimSpace(watchRoot)
	if trimmed == "" {
		return "", errors.New("watch root is required")
	}
	return filepath.Clean(trimmed), nil
}

func normalizeOutputDir(outputDir string) (string, error) {
	trimmed := strings.TrimSpace(outputDir)
	if trimmed == "" {
		return "", errors.New("output directory is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	return filepath.Clean(abs), nil
}

func deriveTriggerName(normalizedOutputDir string) string {
	sum := sha1.Sum([]byte(normalizedOutputDir))
	return fmt.Sprintf("granola2markdown-%x", sum[:6])
}

func buildInstallPayload(triggerName string, watchRoot string, outputDir string, cachePath string) ([]byte, error) {
	command := []string{"granola2markdown"}
	if strings.TrimSpace(cachePath) != "" {
		command = append(command, "--cache-path", cachePath)
	}
	command = append(command, outputDir)

	payload := []any{
		"trigger",
		watchRoot,
		map[string]any{
			"name":         triggerName,
			"expression":   buildCacheMatchExpression(cachePath),
			"command":      command,
			"append_files": false,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal watchman trigger payload: %w", err)
	}
	return data, nil
}

func buildCacheMatchExpression(cachePath string) []any {
	if strings.TrimSpace(cachePath) == "" {
		return []any{"match", "cache-v*.json", "wholename"}
	}
	return []any{"match", filepath.Base(cachePath), "wholename"}
}

func missingTriggerError(stdout string, stderr string, err error) bool {
	text := strings.ToLower(strings.Join([]string{
		stdout,
		stderr,
		err.Error(),
	}, "\n"))
	markers := []string{
		"unknown trigger",
		"no trigger",
		"not found",
		"does not exist",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
