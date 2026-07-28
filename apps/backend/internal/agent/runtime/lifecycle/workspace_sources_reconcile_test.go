package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReconcileWorkspaceSources_RejectsMissingFolderTarget(t *testing.T) {
	err := reconcileWorkspaceSources(context.Background(), t.TempDir(), []WorkspaceFolderSpec{{Name: "missing", LocalPath: "/definitely/not/a/kandev-folder"}})
	if err == nil {
		t.Fatal("missing durable folder target was accepted")
	}
}

func TestReconcileWorkspaceRepositories_RecreatesMissingOwnedLink(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	linkInfo, err := os.Stat(filepath.Join(root, "api"))
	if err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(linkInfo, sourceInfo) {
		t.Fatal("repository link does not resolve to its source")
	}
	if err := os.Remove(filepath.Join(root, "api")); err != nil {
		t.Fatal(err)
	}
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcile after reset: %v", err)
	}
}

func TestReconcileWorkspaceRepositories_SkipsRepositoryUsedAsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, filepath.Base(root))

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: filepath.Base(root), RepositoryPath: root}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("self-referential repository link exists after reconcile: %v", err)
	}
}

func TestReconcileWorkspaceRepositories_RemovesExistingSelfReferentialLink(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, filepath.Base(root))
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	before, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: filepath.Base(root), RepositoryPath: root}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("self-referential repository link exists after reconcile: %v", err)
	}
	after, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("workspace root changed while removing its self-link")
	}
}
