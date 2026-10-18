//go:build !windows

package controller

import (
	"bytes"
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/agents"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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
	// A zombie counts as dead: an init-less container may never reap the orphan.
	deadline := time.Now().Add(3 * time.Second)
	for descendantAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if descendantAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("descendant %d survived cancellation", pid)
	}
}

func descendantAlive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return true
	}
	if end := bytes.LastIndexByte(raw, ')'); end >= 0 && end+2 < len(raw) {
		return raw[end+2] != 'Z'
	}
	return true
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
