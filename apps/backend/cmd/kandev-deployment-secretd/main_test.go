package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/launcher/deploymentsecretd"
)

func TestParseDaemonArgsDefaultsToTheReleaseContract(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KANDEV_HOME_DIR", home)
	config, err := parseDaemonArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.socketPath != deploymentsecretd.SocketPath {
		t.Fatalf("socket = %q, want %q", config.socketPath, deploymentsecretd.SocketPath)
	}
	if want := filepath.Join(home, "data"); config.dataDir != want {
		t.Fatalf("data dir = %q, want %q", config.dataDir, want)
	}
}

func TestParseDaemonArgsAcceptsBothFlagForms(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	config, err := parseDaemonArgs([]string{"--socket", "/run/kandev/custom.sock", "--data-dir=/srv/kandev/data"})
	if err != nil {
		t.Fatal(err)
	}
	if config.socketPath != "/run/kandev/custom.sock" || config.dataDir != "/srv/kandev/data" {
		t.Fatalf("config = %+v", config)
	}
	config, err = parseDaemonArgs([]string{"--socket=/run/kandev/other.sock", "--data-dir", "/tmp/data"})
	if err != nil {
		t.Fatal(err)
	}
	if config.socketPath != "/run/kandev/other.sock" || config.dataDir != "/tmp/data" {
		t.Fatalf("config = %+v", config)
	}
}

func TestRunReportsUsageErrors(t *testing.T) {
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	for _, testCase := range []struct {
		name    string
		args    []string
		message string
	}{
		{name: "unknown flag", args: []string{"--socket-prefix", "/run"}, message: "unknown flag"},
		{name: "missing value", args: []string{"--socket"}, message: "requires a value"},
		{name: "empty value", args: []string{"--data-dir="}, message: "non-empty value"},
		{name: "flag as value", args: []string{"--socket", "--data-dir"}, message: "requires a value"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(testCase.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit = %d, want 2 (stderr %q)", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), testCase.message) {
				t.Fatalf("stderr = %q, want it to mention %q", stderr.String(), testCase.message)
			}
			if !strings.HasPrefix(stderr.String(), deploymentsecretd.BinaryName+": ") {
				t.Fatalf("stderr = %q, want the daemon prefix", stderr.String())
			}
		})
	}
}

func TestRunPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{
		deploymentsecretd.BinaryName,
		"--socket",
		"--data-dir",
		deploymentsecretd.SocketPath,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("usage does not describe %q:\n%s", want, stdout.String())
		}
	}
}
