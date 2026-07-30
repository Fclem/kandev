package share

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/i18n"
)

// localizedSnapshot exercises every localized surface of the share artifacts:
// the hero, the metadata table, the role headings, and a truncated tool result.
func localizedSnapshot(locale string) *Snapshot {
	return &Snapshot{
		Locale: locale,
		Task:   TaskMeta{Title: "Fix login", WorkflowStep: "Review"},
		Session: SessionMeta{
			AgentType:    "claude",
			Model:        "opus",
			ExecutorType: "local_pc",
		},
		Messages: []Message{
			{Role: roleUser, Blocks: []Block{{Kind: blockKindText, Text: "hello"}}},
			{Role: roleAssistant, Blocks: []Block{
				{Kind: blockKindToolResult, Output: "out", Truncated: true},
			}},
		},
	}
}

// The pseudo locale is the completeness oracle: every catalog message renders
// accented, so any copy still reading as plain ASCII was never localized.
func TestBuildShareHTML_UsesSnapshotLocale(t *testing.T) {
	t.Parallel()
	doc := BuildShareHTML(localizedSnapshot("pseudo"))

	if !strings.Contains(doc, `<html lang="pseudo">`) {
		t.Error(`<html lang> should reflect the snapshot locale`)
	}
	for _, english := range []string{
		"shared task", "Try kandev on GitHub", "Assistant", "Tool output (truncated)",
	} {
		if strings.Contains(doc, english) {
			t.Errorf("share.html still contains untranslated %q", english)
		}
	}
	for _, key := range []string{"share.brandTag", "share.roleAssistant", "share.cta"} {
		if want := i18n.T("pseudo", key); !strings.Contains(doc, want) {
			t.Errorf("share.html missing pseudo copy for %s (%q)", key, want)
		}
	}
}

func TestBuildGistREADME_UsesSnapshotLocale(t *testing.T) {
	t.Parallel()
	doc := BuildGistREADME(localizedSnapshot("pseudo"), "https://example.test/view")

	for _, english := range []string{
		"Session details", "Workflow step", "Shared from", "Built with", "Tool output",
	} {
		if strings.Contains(doc, english) {
			t.Errorf("README still contains untranslated %q", english)
		}
	}
	// The interpolated URL must survive translation verbatim.
	if !strings.Contains(doc, "https://example.test/view") {
		t.Error("README dropped the rendered-view URL")
	}
}

func TestBuildShareHTML_DefaultsToEnglishWithoutLocale(t *testing.T) {
	t.Parallel()
	// Snapshots written before the Locale field existed carry "".
	doc := BuildShareHTML(localizedSnapshot(""))
	if !strings.Contains(doc, `<html lang="en">`) {
		t.Error("an empty locale should fall back to en, not render an empty lang")
	}
	if !strings.Contains(doc, "shared task") {
		t.Error("English copy should still render for an empty locale")
	}
}

func TestGistDescription_UsesSnapshotLocale(t *testing.T) {
	t.Parallel()
	if got := gistDescription(localizedSnapshot("pseudo")); strings.Contains(got, "kandev share:") {
		t.Errorf("description not localized: %q", got)
	}
	// The task title is user data and must pass through untranslated.
	if got := gistDescription(localizedSnapshot("pseudo")); !strings.Contains(got, "Fix login") {
		t.Errorf("description dropped the task title: %q", got)
	}
	if got := gistDescription(nil); got != i18n.T(i18n.DefaultLocale, "share.gistDescription") {
		t.Errorf("nil snapshot: got %q", got)
	}
}

func TestBuildSnapshotLocale_ComesFromContext(t *testing.T) {
	t.Parallel()
	// The HTTP layer seeds the locale; the builder must pick it up so the
	// rendered artifacts match the locale the sharer was using.
	if got := i18n.FromContext(i18n.ContextWithLocale(context.Background(), "pseudo")); got != "pseudo" {
		t.Fatalf("got %q, want pseudo", got)
	}
}
