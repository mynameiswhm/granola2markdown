package watchman

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin []byte
}

func TestInstallSequenceAndPayload(t *testing.T) {
	var calls []call
	manager := NewManagerWithDeps(
		func(file string) (string, error) {
			return "/usr/local/bin/watchman", nil
		},
		func(name string, args []string, stdin []byte) (string, string, error) {
			calls = append(calls, call{
				name:  name,
				args:  append([]string(nil), args...),
				stdin: append([]byte(nil), stdin...),
			})
			return "", "", nil
		},
	)

	cachePath := filepath.Join(string(filepath.Separator), "tmp", "granola", "cache-v3.json")
	result, err := manager.Install("./notes", cachePath)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 watchman calls, got %d", len(calls))
	}

	if calls[0].name != "watchman" || len(calls[0].args) != 2 || calls[0].args[0] != "watch-project" {
		t.Fatalf("unexpected watch-project call: %#v", calls[0])
	}
	watchRoot := filepath.Dir(cachePath)
	if calls[0].args[1] != watchRoot {
		t.Fatalf("watch-project root mismatch: got %q want %q", calls[0].args[1], watchRoot)
	}

	if calls[1].name != "watchman" || len(calls[1].args) != 1 || calls[1].args[0] != "-j" {
		t.Fatalf("unexpected trigger call: %#v", calls[1])
	}

	var payload []any
	if err := json.Unmarshal(calls[1].stdin, &payload); err != nil {
		t.Fatalf("invalid trigger payload JSON: %v", err)
	}

	if got := payload[0].(string); got != "trigger" {
		t.Fatalf("payload[0] = %q, want trigger", got)
	}
	if got := payload[1].(string); got != watchRoot {
		t.Fatalf("payload[1] = %q, want %q", got, watchRoot)
	}

	config, ok := payload[2].(map[string]any)
	if !ok {
		t.Fatalf("payload[2] is not an object: %#v", payload[2])
	}

	commandRaw, ok := config["command"].([]any)
	if !ok {
		t.Fatalf("command is not an array: %#v", config["command"])
	}
	command := make([]string, 0, len(commandRaw))
	for _, item := range commandRaw {
		command = append(command, item.(string))
	}

	expectedOutput, err := filepath.Abs("./notes")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	expectedOutput = filepath.Clean(expectedOutput)
	expectedCommand := []string{"granola2markdown", "--cache-path", cachePath, expectedOutput}
	if strings.Join(command, "|") != strings.Join(expectedCommand, "|") {
		t.Fatalf("trigger command mismatch: got %v want %v", command, expectedCommand)
	}

	expectedTrigger, err := TriggerNameForOutputDir("./notes")
	if err != nil {
		t.Fatalf("TriggerNameForOutputDir() error = %v", err)
	}
	if result.TriggerName != expectedTrigger {
		t.Fatalf("trigger name mismatch: got %q want %q", result.TriggerName, expectedTrigger)
	}
}

func TestInstallMissingDependency(t *testing.T) {
	manager := NewManagerWithDeps(
		func(file string) (string, error) {
			return "", exec.ErrNotFound
		},
		func(name string, args []string, stdin []byte) (string, string, error) {
			t.Fatal("run function should not be called when dependency is missing")
			return "", "", nil
		},
	)

	_, err := manager.Install("./notes", "/tmp/granola/cache-v3.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrDependencyMissing) {
		t.Fatalf("expected ErrDependencyMissing, got %v", err)
	}
}

func TestInstallAndUninstallUseSameTriggerName(t *testing.T) {
	var calls []call
	manager := NewManagerWithDeps(
		func(file string) (string, error) {
			return "/usr/local/bin/watchman", nil
		},
		func(name string, args []string, stdin []byte) (string, string, error) {
			calls = append(calls, call{
				name:  name,
				args:  append([]string(nil), args...),
				stdin: append([]byte(nil), stdin...),
			})
			return "", "", nil
		},
	)

	cachePath := "/tmp/granola/cache-v3.json"
	installRes, err := manager.Install("./notes", cachePath)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	uninstallRes, err := manager.Uninstall("./notes", cachePath)
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if installRes.TriggerName != uninstallRes.TriggerName {
		t.Fatalf("trigger names differ: install=%q uninstall=%q", installRes.TriggerName, uninstallRes.TriggerName)
	}

	last := calls[len(calls)-1]
	if len(last.args) != 3 || last.args[0] != "trigger-del" {
		t.Fatalf("unexpected uninstall command: %#v", last)
	}
	if last.args[1] != filepath.Dir(cachePath) {
		t.Fatalf("uninstall watch root mismatch: got %q want %q", last.args[1], filepath.Dir(cachePath))
	}
	if last.args[2] != installRes.TriggerName {
		t.Fatalf("uninstall trigger mismatch: got %q want %q", last.args[2], installRes.TriggerName)
	}
}

func TestUninstallMissingTriggerIsIdempotent(t *testing.T) {
	manager := NewManagerWithDeps(
		func(file string) (string, error) {
			return "/usr/local/bin/watchman", nil
		},
		func(name string, args []string, stdin []byte) (string, string, error) {
			return "", "unknown trigger: granola2markdown-123", errors.New("exit status 1")
		},
	)

	result, err := manager.Uninstall("./notes", "/tmp/granola/cache-v3.json")
	if err != nil {
		t.Fatalf("Uninstall() unexpected error: %v", err)
	}
	if result.Removed {
		t.Fatalf("expected Removed=false for missing trigger, got true")
	}
}
