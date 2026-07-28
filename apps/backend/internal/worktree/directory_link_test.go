package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateOwnedDirectoryLinkCreatesLiveLinkInsideOwnedRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	link, err := CreateOwnedDirectoryLink(root, "source", target)
	if err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(link, "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through link = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(target, "live.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(link, "live.txt")); err != nil || string(got) != "two" {
		t.Fatalf("link is not live: %q, %v", got, err)
	}
}

func TestCreateOwnedDirectoryLinkRejectsCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOwnedDirectoryLink(root, "source", target); err == nil {
		t.Fatal("CreateOwnedDirectoryLink succeeded for collision")
	}
}

func TestCreateOwnedDirectoryLinkRejectsSymlinkedControlAncestor(t *testing.T) {
	realBase := t.TempDir()
	linkBase := filepath.Join(t.TempDir(), "tasks")
	if err := os.Symlink(realBase, linkBase); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := CreateOwnedDirectoryLink(filepath.Join(linkBase, "task-1"), "source", t.TempDir()); err == nil {
		t.Fatal("CreateOwnedDirectoryLink accepted symlinked control ancestor")
	}
}

func TestEnsureOwnedDirectoryLinkReturnsExistingMatchingLink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	target := t.TempDir()
	link, err := CreateOwnedDirectoryLink(root, "source", target)
	if err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}

	got, created, err := EnsureOwnedDirectoryLink(root, "source", target)
	if err != nil {
		t.Fatalf("EnsureOwnedDirectoryLink: %v", err)
	}
	if got != link || created {
		t.Fatalf("EnsureOwnedDirectoryLink = %q, %t; want %q, false", got, created, link)
	}
}

func TestEnsureOwnedDirectoryLinkRejectsDifferentTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	originalTarget := t.TempDir()
	if _, err := CreateOwnedDirectoryLink(root, "source", originalTarget); err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}

	if _, _, err := EnsureOwnedDirectoryLink(root, "source", t.TempDir()); err == nil {
		t.Fatal("EnsureOwnedDirectoryLink accepted a different target")
	}
	targetInfo, err := os.Stat(filepath.Join(root, "source"))
	if err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(originalTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(targetInfo, originalInfo) {
		t.Fatal("EnsureOwnedDirectoryLink replaced the existing link")
	}
}

func TestRemoveSelfReferentialDirectoryLinkRemovesOnlyLink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOwnedDirectoryLink(root, "source", root); err != nil {
		t.Fatalf("CreateOwnedDirectoryLink: %v", err)
	}

	removed, err := RemoveSelfReferentialDirectoryLink(root, "source")
	if err != nil {
		t.Fatalf("RemoveSelfReferentialDirectoryLink: %v", err)
	}
	if !removed {
		t.Fatal("RemoveSelfReferentialDirectoryLink did not remove the self-link")
	}
	if _, err := os.Lstat(filepath.Join(root, "source")); !os.IsNotExist(err) {
		t.Fatalf("self-link still exists: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "keep.txt")); err != nil || string(got) != "keep" {
		t.Fatalf("root contents changed: %q, %v", got, err)
	}
}

func TestRemoveSelfReferentialDirectoryLinkLeavesOtherEntries(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, root, entry string)
	}{
		{
			name: "link to another directory",
			setup: func(t *testing.T, root, entry string) {
				t.Helper()
				if _, err := CreateOwnedDirectoryLink(root, entry, t.TempDir()); err != nil {
					t.Fatalf("CreateOwnedDirectoryLink: %v", err)
				}
			},
		},
		{
			name: "real directory",
			setup: func(t *testing.T, root, entry string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(root, entry), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			tt.setup(t, root, "source")

			removed, err := RemoveSelfReferentialDirectoryLink(root, "source")
			if err != nil {
				t.Fatalf("RemoveSelfReferentialDirectoryLink: %v", err)
			}
			if removed {
				t.Fatal("RemoveSelfReferentialDirectoryLink removed a non-self entry")
			}
			if _, err := os.Lstat(filepath.Join(root, "source")); err != nil {
				t.Fatalf("entry was modified: %v", err)
			}
		})
	}
}
