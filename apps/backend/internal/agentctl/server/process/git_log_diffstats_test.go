package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fileStats pulls the additions/deletions a parsed file entry carries.
func fileStats(t *testing.T, files map[string]interface{}, path string) (int, int) {
	t.Helper()
	entry, ok := files[path].(map[string]interface{})
	if !ok {
		t.Fatalf("no file entry for %q (have %v)", path, keysOf(files))
	}
	additions, ok := entry["additions"].(int)
	if !ok {
		t.Fatalf("file %q: additions is not an int: %#v", path, entry["additions"])
	}
	deletions, ok := entry["deletions"].(int)
	if !ok {
		t.Fatalf("file %q: deletions is not an int: %#v", path, entry["deletions"])
	}
	return additions, deletions
}

func keysOf(files map[string]interface{}) []string {
	out := make([]string, 0, len(files))
	for k := range files {
		out = append(out, k)
	}
	return out
}

func TestNumstatByPath(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  map[string]numstatEntry
	}{
		{name: "empty block", block: "", want: map[string]numstatEntry{}},
		{
			name:  "single row",
			block: "3\t1\tapps/a.go\n",
			want:  map[string]numstatEntry{"apps/a.go": {path: "apps/a.go", additions: 3, deletions: 1}},
		},
		{
			name:  "binary file reports dashes and yields a zero pair",
			block: "-\t-\tassets/logo.png\n",
			want:  map[string]numstatEntry{"assets/logo.png": {path: "assets/logo.png"}},
		},
		{
			name: "stat table lines are ignored",
			// `git show --stat --numstat` emits both blocks; only the tab-separated
			// numstat rows are ours to read.
			block: "1\t1\tseed.sql\n schema.sql | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n",
			want:  map[string]numstatEntry{"seed.sql": {path: "seed.sql", additions: 1, deletions: 1}},
		},
		{
			name:  "rename row is keyed by its new-side path",
			block: "2\t0\tapps/{old => new}/file.go\n",
			want: map[string]numstatEntry{
				"apps/new/file.go": {path: "apps/new/file.go", additions: 2, deletions: 0},
			},
		},
		{
			name:  "two rows",
			block: "1\t1\ta.go\n0\t4\tb.go\n",
			want: map[string]numstatEntry{
				"a.go": {path: "a.go", additions: 1, deletions: 1},
				"b.go": {path: "b.go", additions: 0, deletions: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := numstatByPath(tt.block)
			if len(got) != len(tt.want) {
				t.Fatalf("numstatByPath() = %+v, want %+v", got, tt.want)
			}
			for path, want := range tt.want {
				if got[path] != want {
					t.Errorf("row %q = %+v, want %+v", path, got[path], want)
				}
			}
		})
	}
}

// TestParseCommitDiffWithOptions_LineCounts covers both counting routes: the
// authoritative --numstat rows when git supplied them, and the in-hunk fallback
// when it did not.
func TestParseCommitDiffWithOptions_LineCounts(t *testing.T) {
	// Removing the SQL line `-- seed data` emits `--- seed data`; adding the C
	// line `++counter;` emits `+++counter;`. A prefix test drops both and reports
	// +0 -0 where git reports +1 -1.
	dashAndPlusPatch := "diff --git a/seed.sql b/seed.sql\n" +
		"index 64cf6a5..2559267 100644\n" +
		"--- a/seed.sql\n" +
		"+++ b/seed.sql\n" +
		"@@ -1,2 +1,2 @@\n" +
		"--- seed data\n" +
		" counter;\n" +
		"+++counter;\n"

	tests := []struct {
		name     string
		output   string
		wantFile string
		wantAdd  int
		wantDel  int
	}{
		{
			name:     "content starting with dashes and pluses, no numstat",
			output:   dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name:     "content starting with dashes and pluses, with numstat",
			output:   "1\t1\tseed.sql\n" + dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "file headers of a real multi-file diff are not counted",
			output: "diff --git a/a.go b/a.go\n" +
				"index 111..222 100644\n" +
				"--- a/a.go\n" +
				"+++ b/a.go\n" +
				"@@ -1,3 +1,3 @@\n" +
				" package a\n" +
				"-old\n" +
				"+new\n" +
				"diff --git a/b.go b/b.go\n" +
				"index 333..444 100644\n" +
				"--- a/b.go\n" +
				"+++ b/b.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package b\n" +
				"+added\n",
			wantFile: "a.go",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "no newline at end of file marker is not content",
			output: "diff --git a/a.txt b/a.txt\n" +
				"--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"\\ No newline at end of file\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			wantFile: "a.txt",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "binary file from numstat dashes",
			output: "-\t-\tlogo.png\n" +
				"diff --git a/logo.png b/logo.png\n" +
				"index 111..222 100644\n" +
				"Binary files a/logo.png and b/logo.png differ\n",
			wantFile: "logo.png",
			wantAdd:  0,
			wantDel:  0,
		},
		{
			name: "diff with no hunk header at all",
			output: "diff --git a/mode.sh b/mode.sh\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantFile: "mode.sh",
			wantAdd:  0,
			wantDel:  0,
		},
		{
			name: "numstat rows for other paths fall back to counting this file",
			output: "5\t5\tsomewhere-else.go\n" +
				"7\t7\tand-another.go\n" +
				dashAndPlusPatch,
			wantFile: "seed.sql",
			wantAdd:  1,
			wantDel:  1,
		},
		{
			name: "renamed file is paired via git's brace notation",
			output: "1\t0\tapps/{old => new}/file.go\n" +
				"diff --git a/apps/old/file.go b/apps/new/file.go\n" +
				"similarity index 90%\n" +
				"rename from apps/old/file.go\n" +
				"rename to apps/new/file.go\n" +
				"--- a/apps/old/file.go\n" +
				"+++ b/apps/new/file.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package p\n" +
				"+added\n",
			wantFile: "apps/new/file.go",
			wantAdd:  1,
			wantDel:  0,
		},
	}

	gitOp := &GitOperator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := gitOp.parseCommitDiffWithOptions(tt.output, parseCommitDiffOptions{})
			additions, deletions := fileStats(t, files, tt.wantFile)
			if additions != tt.wantAdd || deletions != tt.wantDel {
				t.Errorf("%s: got +%d -%d, want +%d -%d",
					tt.wantFile, additions, deletions, tt.wantAdd, tt.wantDel)
			}
		})
	}
}

func TestParseCommitDiffWithOptions_EmptyDiff(t *testing.T) {
	gitOp := &GitOperator{}
	for _, output := range []string{"", "\n", "1\t1\tseed.sql\n"} {
		files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
		if len(files) != 0 {
			t.Errorf("parseCommitDiffWithOptions(%q) returned %d files, want 0", output, len(files))
		}
	}
}

// TestParseCommitDiffWithOptions_MultiFileTotals checks the rolled-up
// Insertions/Deletions that feed the commit-detail header.
func TestParseCommitDiffWithOptions_MultiFileTotals(t *testing.T) {
	output := "1\t1\ta.go\n" +
		"1\t0\tb.go\n" +
		"diff --git a/a.go b/a.go\n" +
		"--- a/a.go\n" +
		"+++ b/a.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" package a\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/b.go b/b.go\n" +
		"--- a/b.go\n" +
		"+++ b/b.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" package b\n" +
		"+added\n"

	gitOp := &GitOperator{}
	files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2 (%v)", len(files), keysOf(files))
	}
	insertions, deletions := sumFileDiffStats(files)
	if insertions != 2 || deletions != 1 {
		t.Errorf("sumFileDiffStats() = +%d -%d, want +2 -1", insertions, deletions)
	}
}

// seedDashAndPlusCommit writes the pathological file, commits a baseline, then
// rewrites it so the patch contains a removed line starting with "--" and an
// added line starting with "++". git reports +1 -1 for the resulting commit.
func seedDashAndPlusCommit(t *testing.T, repoDir string) string {
	t.Helper()
	writeFile(t, repoDir, "seed.sql", "-- seed data\ncounter;\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: baseline")
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	writeFile(t, repoDir, "seed.sql", "counter;\n++counter;\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: rewrite")
	return base
}

// TestShowCommit_CountsDashAndPlusPrefixedContent is the end-to-end guard on the
// commit-detail view: real git output, real numbers, compared against git's own
// --numstat for the same commit.
func TestShowCommit_CountsDashAndPlusPrefixedContent(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	seedDashAndPlusCommit(t, repoDir)
	head := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.ShowCommit(context.Background(), head)
	if err != nil {
		t.Fatalf("ShowCommit returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("ShowCommit failed: %s", result.Error)
	}

	// git's own answer for this commit.
	wantNumstat := "1\t1\tseed.sql"
	if got := strings.TrimSpace(runGit(t, repoDir, "show", "--format=", "--numstat", head)); got != wantNumstat {
		t.Fatalf("test fixture drifted: git numstat = %q, want %q", got, wantNumstat)
	}

	additions, deletions := fileStats(t, result.Files, "seed.sql")
	if additions != 1 || deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", additions, deletions)
	}
	if result.Insertions != 1 || result.Deletions != 1 {
		t.Errorf("rolled-up stats = +%d -%d, want +1 -1", result.Insertions, result.Deletions)
	}
}

// TestParseCommitDiffWithOptions_EntriesCarryPath guards the file-entry
// contract the frontend's changes tree depends on: every entry must include
// its own `path` field, not just sit under the map key. The DB-snapshot
// fallback replays these entries as a git status_update, and the frontend
// builds its file tree from each entry's `path` — an entry without one
// crashes the changes panel (`Cannot read properties of undefined (reading
// 'split')`) for archived tasks whose live execution is gone.
func TestParseCommitDiffWithOptions_EntriesCarryPath(t *testing.T) {
	output := "diff --git a/apps/a.go b/apps/a.go\n" +
		"--- a/apps/a.go\n" +
		"+++ b/apps/a.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" package a\n" +
		"-old\n" +
		"+new\n" +
		"diff --git a/readme.md b/readme.md\n" +
		"--- a/readme.md\n" +
		"+++ b/readme.md\n" +
		"@@ -1 +1 @@\n" +
		"-hello\n" +
		"+world\n"

	gitOp := &GitOperator{}
	files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2 (%v)", len(files), keysOf(files))
	}
	for _, path := range []string{"apps/a.go", "readme.md"} {
		entry, ok := files[path].(map[string]interface{})
		if !ok {
			t.Fatalf("no file entry for %q", path)
		}
		if got, _ := entry["path"].(string); got != path {
			t.Errorf("entry %q path = %#v, want %q", path, entry["path"], path)
		}
	}
}

// TestParseCommitDiffWithOptions_SpecialPaths guards path extraction against
// git's quoting and separator rules. git C-quotes paths containing tabs,
// non-ASCII bytes, quotes, or backslashes (core.quotePath, default on) —
// a naive first-` b/` split drops them entirely — and a path containing the
// literal sequence ` b/` mis-splits the header. The parser must read the
// path off the per-file `+++ b/<path>` line, which git writes once per
// section and is unambiguous.
func TestParseCommitDiffWithOptions_SpecialPaths(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantPath string
	}{
		{
			name: "tab in path is C-quoted by git",
			output: "diff --git \"a/a\\tb.txt\" \"b/a\\tb.txt\"\n" +
				"new file mode 100644\n" +
				"index 0000000..e69de29\n" +
				"--- /dev/null\n" +
				"+++ \"b/a\\tb.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "a\tb.txt",
		},
		{
			name: "non-ASCII path is C-quoted with octal escapes",
			output: "diff --git \"a/caf\\303\\251.txt\" \"b/caf\\303\\251.txt\"\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ \"b/caf\\303\\251.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+caf\u00e9\n",
			wantPath: "caf\u00e9.txt",
		},
		{
			name: "quote char in path is C-quoted",
			output: "diff --git \"a/has\\\"quote.txt\" \"b/has\\\"quote.txt\"\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ \"b/has\\\"quote.txt\"\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "has\"quote.txt",
		},
		{
			name: "path containing literal b-slash splits via the +++ line",
			output: "diff --git a/name b/dir.txt b/name b/dir.txt\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ b/name b/dir.txt\n" +
				"@@ -0,0 +1 @@\n" +
				"+content\n",
			wantPath: "name b/dir.txt",
		},
		{
			name: "trailing-whitespace path keeps the tab marker stripped",
			output: "diff --git a/trail.txt  b/trail.txt \n" +
				"index 111..222 100644\n" +
				"--- a/trail.txt \t\n" +
				"+++ b/trail.txt \t\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n",
			wantPath: "trail.txt ",
		},
		{
			name: "quoted path ending in whitespace strips the outer marker",
			output: "diff --git \"a/weird \\\"quote.txt \" \"b/weird \\\"quote.txt \"\n" +
				"index 111..222 100644\n" +
				"--- \"a/weird \\\"quote.txt \"\t\n" +
				"+++ \"b/weird \\\"quote.txt \"\t\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n",
			wantPath: "weird \"quote.txt ",
		},
		{
			name: "renamed file keeps the new path from the +++ line",
			output: "diff --git a/apps/old/file.go b/apps/new/file.go\n" +
				"similarity index 90%\n" +
				"rename from apps/old/file.go\n" +
				"rename to apps/new/file.go\n" +
				"--- a/apps/old/file.go\n" +
				"+++ b/apps/new/file.go\n" +
				"@@ -1,2 +1,3 @@\n" +
				" package p\n" +
				"+added\n",
			wantPath: "apps/new/file.go",
		},
		{
			name: "pure rename without hunks keeps the rename to line path",
			output: "diff --git a/old b.txt b/new b.txt\n" +
				"similarity index 100%\n" +
				"rename from old b.txt\n" +
				"rename to new b.txt\n",
			wantPath: "new b.txt",
		},
		{
			name: "binary section with b-slash path parses the Binary files line",
			output: "diff --git a/name b/binary.bin b/name b/binary.bin\n" +
				"index 6735744..d7bf111 100644\n" +
				"Binary files a/name b/binary.bin and b/name b/binary.bin differ\n",
			wantPath: "name b/binary.bin",
		},
		{
			name: "quoted binary section unquotes the Binary files line",
			output: "diff --git \"a/caf\\303\\251 b.bin\" \"b/caf\\303\\251 b.bin\"\n" +
				"index a6a3e7f..073ea92 100644\n" +
				"Binary files \"a/caf\\303\\251 b.bin\" and \"b/caf\\303\\251 b.bin\" differ\n",
			wantPath: "caf\u00e9 b.bin",
		},
		{
			name: "mode-only change on a quoted path parses the quoted header",
			output: "diff --git \"a/caf\\303\\251 b/mode.sh\" \"b/caf\\303\\251 b/mode.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "caf\u00e9 b/mode.sh",
		},
		{
			name: "mode-only change with escaped quote in path parses the quoted header",
			output: "diff --git \"a/has\\\"quote.sh\" \"b/has\\\"quote.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "has\"quote.sh",
		},
		{
			name: "mode-only change with b-slash path splits on the equal-paths separator",
			output: "diff --git a/mode name b/dir.sh b/mode name b/dir.sh\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "mode name b/dir.sh",
		},
		{
			name: "mode-only change with quote and b-slash in path",
			output: "diff --git \"a/weird\\\"quote b/file.sh\" \"b/weird\\\"quote b/file.sh\"\n" +
				"old mode 100644\n" +
				"new mode 100755\n",
			wantPath: "weird\"quote b/file.sh",
		},
		{
			name: "binary path containing and b-slash splits on the equal-paths separator",
			output: "diff --git a/old and b/thing.bin b/old and b/thing.bin\n" +
				"index 6735744..d7bf111 100644\n" +
				"Binary files a/old and b/thing.bin and b/old and b/thing.bin differ\n",
			wantPath: "old and b/thing.bin",
		},
		{
			name: "binary rename with and b-slash in new path keeps the new path",
			output: "diff --git a/x.bin b/old and b/thing.bin\n" +
				"similarity index 50%\n" +
				"rename from x.bin\n" +
				"rename to old and b/thing.bin\n" +
				"Binary files a/x.bin and b/old and b/thing.bin differ\n",
			wantPath: "old and b/thing.bin",
		},
	}

	gitOp := &GitOperator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := gitOp.parseCommitDiffWithOptions(tt.output, parseCommitDiffOptions{})
			entry, ok := files[tt.wantPath].(map[string]interface{})
			if !ok {
				t.Fatalf("no entry for %q; got keys %v", tt.wantPath, keysOf(files))
			}
			if got, _ := entry["path"].(string); got != tt.wantPath {
				t.Errorf("entry path = %#v, want %#v", got, tt.wantPath)
			}
		})
	}
}

// TestGetCumulativeDiff_ModeOnlyBSlashPath covers the round-5 blocker with
// real git output: a chmod-only change on a path containing the literal
// sequence ` b/` produces an unquoted header with no ---/+++ lines
// (`diff --git a/mode name b/dir.sh b/mode name b/dir.sh` + old mode/new
// mode). The first ` b/` lies inside the old path, so the equal-paths
// separator rule must select the correct split.
func TestGetCumulativeDiff_ModeOnlyBSlashPath(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	if err := os.MkdirAll(filepath.Join(repoDir, "mode name b"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, repoDir, "mode name b/dir.sh", "#!/bin/sh\necho hi\n")
	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "seed: add script")
	base := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD"))

	// Pin core.filemode so the chmod below reliably produces a mode-only
	// diff. Git's default (auto) disables executable-bit tracking on
	// filesystems without chmod support; on those the test cannot produce
	// the fixture it exists to prove, so skip with an explicit reason rather
	// than fail.
	runGit(t, repoDir, "config", "core.filemode", "true")
	if err := os.Chmod(filepath.Join(repoDir, "mode name b/dir.sh"), 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if diff := runGit(t, repoDir, "diff"); !strings.Contains(diff, "old mode") {
		t.Skipf("filesystem does not track executable bits; skipping mode-only regression (diff=%q)", diff)
	}

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %s", result.Error)
	}

	const wantPath = "mode name b/dir.sh"
	if _, ok := result.Files[wantPath].(map[string]interface{}); !ok {
		t.Fatalf("no entry for %q; got keys %v", wantPath, keysOf(result.Files))
	}
}

// TestParseCommitDiffWithOptions_HeaderTokenInsideBody guards the section
// boundary against a patch body (or filename) containing the literal
// `diff --git ` byte sequence. git only emits that token at the start of a
// section header line, so the split must be line-aware: an added line like
// `+diff --git a/example b/example` must not truncate the real section or
// fabricate a bogus one.
func TestParseCommitDiffWithOptions_HeaderTokenInsideBody(t *testing.T) {
	output := "diff --git a/real.txt b/real.txt\n" +
		"index 111..222 100644\n" +
		"--- a/real.txt\n" +
		"+++ b/real.txt\n" +
		"@@ -1 +1,2 @@\n" +
		"-old\n" +
		"+diff --git a/example b/example\n" +
		"+new\n"

	gitOp := &GitOperator{}
	files := gitOp.parseCommitDiffWithOptions(output, parseCommitDiffOptions{})
	if len(files) != 1 {
		t.Fatalf("got %d files, want exactly real.txt (%v)", len(files), keysOf(files))
	}
	entry, ok := files["real.txt"].(map[string]interface{})
	if !ok {
		t.Fatalf("no entry for real.txt; got keys %v", keysOf(files))
	}
	if got, _ := entry["path"].(string); got != "real.txt" {
		t.Errorf("entry path = %#v, want real.txt", got)
	}
	diff, _ := entry["diff"].(string)
	if !strings.Contains(diff, "+diff --git a/example b/example") || !strings.Contains(diff, "+new") {
		t.Errorf("real.txt diff truncated by header-token line: %q", diff)
	}
	if got, _ := entry["additions"].(int); got != 2 {
		t.Errorf("real.txt additions = %d, want 2 (the added header-token line counts)", got)
	}
	if _, ok := files["example"]; ok {
		t.Errorf("bogus example entry created from patch-body token: %v", keysOf(files))
	}
}

// TestGetCumulativeDiff_CountsDashAndPlusPrefixedContent covers the second
// caller, which builds its diff from `git diff <base>` rather than `git show`.
func TestGetCumulativeDiff_CountsDashAndPlusPrefixedContent(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)

	base := seedDashAndPlusCommit(t, repoDir)

	gitOp := NewGitOperator(repoDir, newTestLogger(t), nil)
	result, err := gitOp.GetCumulativeDiff(context.Background(), base)
	if err != nil {
		t.Fatalf("GetCumulativeDiff returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("GetCumulativeDiff failed: %s", result.Error)
	}

	additions, deletions := fileStats(t, result.Files, "seed.sql")
	if additions != 1 || deletions != 1 {
		t.Errorf("seed.sql = +%d -%d, want +1 -1", additions, deletions)
	}
}
