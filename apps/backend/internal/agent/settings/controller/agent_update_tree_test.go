//go:build !windows

package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestRunDirectCommandTreeCancelKillsGroupAndReleasesPipes(t *testing.T) {
	updater := &hostRuntimeUpdater{executor: execDirectCommandExecutor{}}
	tree, ok := any(updater).(interface {
		RunUpdateTree(context.Context, agents.Command, func(string)) error
	})
	if !ok {
		t.Fatal("host automatic updater cannot bound descendant cancellation")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "escaped")
	pidFile := filepath.Join(dir, "descendant.pid")
	script := filepath.Join(dir, "updater")
	// Readiness is emitted from inside the TERM-ignoring descendant, after its trap is installed.
	body := "#!/bin/sh\nsh -c \"trap '' TERM; echo \\$\\$ > '" + pidFile + "'; echo ready; exec sleep 20\" &\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- tree.RunUpdateTree(ctx, agents.NewCommand(script), func(chunk string) {
			if strings.Contains(chunk, "ready") {
				select {
				case ready <- struct{}{}:
				default:
				}
			}
		})
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("updater never ready")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled updater succeeded")
		}
	case <-time.After(12 * time.Second):
		t.Fatal("descendant kept updater pipes open")
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("descendant pid unavailable: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	// The group SIGKILL must have removed the TERM-ignoring descendant, not just closed the pipes.
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("descendant %d survived cancellation", pid)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant wrote marker after cancellation: %v", err)
	}
}

func TestRunDirectCommandTreeCancelTreatsWaitDelayAfterZeroExitAsSuccess(t *testing.T) {
	updater := &hostRuntimeUpdater{executor: execDirectCommandExecutor{}}
	tree, ok := any(updater).(interface {
		RunUpdateTree(context.Context, agents.Command, func(string)) error
	})
	if !ok {
		t.Fatal("host updater cannot validate a completed update with inherited pipes")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "updater")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 7 &\necho complete\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := tree.RunUpdateTree(context.Background(), agents.NewCommand(script), func(string) {}); err != nil {
		t.Fatalf("zero-exit updater rejected instead of proceeding to probe: %v", err)
	}
}
