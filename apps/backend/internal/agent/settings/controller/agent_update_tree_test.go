package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestRunDirectCommandTreeCancelKillsGroupAndReleasesPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process groups")
	}
	updater := &hostRuntimeUpdater{executor: execDirectCommandExecutor{}}
	tree, ok := any(updater).(interface {
		RunUpdateTree(context.Context, agents.Command, func(string)) error
	})
	if !ok {
		t.Fatal("host automatic updater cannot bound descendant cancellation")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "escaped")
	script := filepath.Join(dir, "updater")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n(trap '' TERM; sleep 8; echo survived > '"+marker+"') &\necho ready\nwait\n"), 0700); err != nil {
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
	case <-time.After(7 * time.Second):
		t.Fatal("descendant kept updater pipes open")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant survived cancellation: %v", err)
	}
}

func TestRunDirectCommandTreeCancelTreatsWaitDelayAfterZeroExitAsSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
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
