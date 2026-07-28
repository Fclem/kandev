//go:build linux

package worktree

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestEnsureOwnedDirectoryLinkAcceptsSameFileTargetAlias(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(alias, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mount(source, alias, "", syscall.MS_BIND, ""); err != nil {
		t.Skipf("bind mounts unavailable: %v", err)
	}
	t.Cleanup(func() {
		if err := syscall.Unmount(alias, 0); err != nil {
			t.Errorf("unmount alias: %v", err)
		}
	})

	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	if _, err := CreateOwnedDirectoryLink(root, "source", source); err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}

	if _, _, err := EnsureOwnedDirectoryLink(root, "source", alias); err != nil {
		t.Fatalf("EnsureOwnedDirectoryLink rejected the same directory through an alias: %v", err)
	}
}
