package repoclone

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestGitCmdKeepsCredentialOutOfArgumentsAndEnvironment(t *testing.T) {
	t.Parallel()
	cloneURL := "https://dev.azure.com/acme/p/_git/r"
	cmd := exec.CommandContext(context.Background(), "git", "clone", "--", cloneURL)
	configureTestGitCommand(t, cmd, &cloneAuth{
		origin: "https://dev.azure.com", username: "kandev", password: "secret-pat",
	})
	if strings.Contains(strings.Join(cmd.Args, " "), "secret-pat") {
		t.Fatal("credential leaked into command arguments")
	}
	if joined := strings.Join(cmd.Env, "\n"); strings.Contains(joined, "secret-pat") ||
		!strings.Contains(joined, "GIT_CONFIG_KEY_1=credential.https://dev.azure.com.helper") {
		t.Fatalf("credential environment = %s", joined)
	}
}

func TestEnsureWorkspaceClonedWithBasicAuthKeepsCredentialScopedToGitChild(t *testing.T) {
	tests := []struct {
		name   string
		cancel bool
	}{
		{name: "git failure"},
		{name: "context cancellation", cancel: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			capturePath := filepath.Join(t.TempDir(), "git-env")
			fakeGit := "#!/bin/sh\nprintf '%s\\n%s' \"$GIT_CONFIG_KEY_1\" \"$GIT_CONFIG_VALUE_1\" > \"$CAPTURE_PATH\"\n" +
				"helper=${GIT_CONFIG_VALUE_1#!}\n\"$helper\" get > \"$CAPTURE_PATH.helper\"\n" +
				"if [ \"$BLOCK_GIT\" = 1 ]; then exec sleep 10; fi\nexit 1\n"
			if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("CAPTURE_PATH", capturePath)
			t.Setenv("GIT_CONFIG_VALUE_0", "")
			if tc.cancel {
				t.Setenv("BLOCK_GIT", "1")
			}
			cloner := NewCloner(Config{BasePath: t.TempDir()}, ProtocolHTTPS, "", logger.Default())
			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.cancel {
				ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
				defer cancel()
			}
			targetPath, err := cloner.EnsureWorkspaceClonedWithBasicAuth(
				ctx, "workspace-a", "azure_devops", "", "https://dev.azure.com/acme/p/_git/r",
				"p", "r", "kandev", "secret-pat",
			)
			if err == nil {
				t.Fatal("expected git clone error")
			}
			if tc.cancel && ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("expected cancelled clone context, got %v", ctx.Err())
			}
			wantPath := filepath.Join("workspaces", "workspace-a", "azure_devops", "p", "r")
			if !strings.Contains(targetPath, wantPath) {
				t.Fatalf("authenticated clone path = %q, want workspace-isolated path containing %q", targetPath, wantPath)
			}
			captured, readErr := os.ReadFile(capturePath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if strings.Contains(string(captured), "secret-pat") {
				t.Fatal("credential leaked into Git child environment")
			}
			expectedScope := "credential.https://dev.azure.com.helper"
			if !strings.Contains(string(captured), expectedScope) {
				t.Fatal("credential helper was not scoped to the authenticated repository host")
			}
			helperOutput, helperErr := os.ReadFile(capturePath + ".helper")
			if helperErr != nil || string(helperOutput) != "username=kandev\npassword=secret-pat\n" {
				t.Fatalf("credential helper output = %q, error = %v", helperOutput, helperErr)
			}
			if os.Getenv("GIT_CONFIG_VALUE_0") != "" {
				t.Fatal("credential escaped into the parent process environment")
			}
		})
	}
}
