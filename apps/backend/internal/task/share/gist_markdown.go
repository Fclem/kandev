package share

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/i18n"
)

// BuildGistREADME renders the README that appears at the top of the gist
// page on github.com. The primary user-facing rendering is share.html
// (served via gist.githack.com); the README is mostly a "go to the pretty
// view" pointer plus a markdown fallback for anyone who lands on the gist
// directly. renderedURL is the gist.githack.com link to share.html; pass
// "" if it's not known yet (the README is regenerated post-upload with
// the real link).
func BuildGistREADME(snap *Snapshot, renderedURL string) string {
	if snap == nil {
		return "# " + i18n.T(i18n.DefaultLocale, "share.pageTitle") + "\n"
	}
	locale := i18n.Normalize(snap.Locale)
	var b strings.Builder
	writeHero(&b, snap)
	writeRenderedCTA(&b, renderedURL, locale)
	writeMetaDetails(&b, snap)
	writeRedactionNote(&b, snap)
	b.WriteString("---\n\n")
	writeConversation(&b, snap.Messages, locale)
	writeFooter(&b, snap)
	return b.String()
}

// writeRenderedCTA injects a prominent link to the styled HTML view so
// visitors who land on the raw gist know where to go.
func writeRenderedCTA(b *strings.Builder, renderedURL, locale string) {
	if renderedURL == "" {
		fmt.Fprintf(b, "> 📖 %s\n\n", i18n.T(locale, "share.openShareHtml"))
		return
	}
	fmt.Fprintf(b, "> ✨ %s\n\n",
		i18n.Tf(locale, "share.openRenderedView", map[string]string{"url": renderedURL}))
}

func writeHero(b *strings.Builder, snap *Snapshot) {
	locale := i18n.Normalize(snap.Locale)
	fmt.Fprintf(b, "# %s\n\n", nonEmpty(snap.Task.Title, i18n.T(locale, "share.untitledTask")))
	// Compact badge line: short, dense, scannable.
	badges := []string{}
	if v := snap.Session.AgentType; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	if v := snap.Session.Model; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	if v := snap.Session.ExecutorType; v != "" {
		badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", v))
	}
	badges = append(badges, fmt.Sprintf("<kbd>%s</kbd>", i18n.Tf(locale, "share.messagesBadge",
		map[string]string{"count": fmt.Sprint(len(snap.Messages))})))
	fmt.Fprintf(b, "<sub>%s</sub>\n\n", strings.Join(badges, " · "))
	// Pitch line — also doubles as marketing.
	fmt.Fprintf(b, "> 🚀 %s\n\n", i18n.T(locale, "share.sharedFrom"))
}

func writeMetaDetails(b *strings.Builder, snap *Snapshot) {
	locale := i18n.Normalize(snap.Locale)
	rows := []struct{ k, v string }{
		{i18n.T(locale, "share.metaAgent"), snap.Session.AgentType},
		{i18n.T(locale, "share.metaModel"), snap.Session.Model},
		{i18n.T(locale, "share.metaExecutor"), snap.Session.ExecutorType},
		{i18n.T(locale, "share.metaStarted"), formatTime(snap.Session.StartedAt)},
		{i18n.T(locale, "share.metaCompleted"), formatPtrTime(snap.Session.CompletedAt)},
		{i18n.T(locale, "share.workflowStep"), snap.Task.WorkflowStep},
	}
	written := 0
	fmt.Fprintf(b, "<details>\n<summary>📊 %s</summary>\n\n", i18n.T(locale, "share.sessionDetails"))
	b.WriteString("|   |   |\n|---|---|\n")
	for _, r := range rows {
		if r.v == "" {
			continue
		}
		fmt.Fprintf(b, "| **%s** | %s |\n", r.k, r.v)
		written++
	}
	if written == 0 {
		fmt.Fprintf(b, "| %s | |\n", i18n.T(locale, "share.noMetadata"))
	}
	b.WriteString("\n</details>\n\n")
}

func writeRedactionNote(b *strings.Builder, snap *Snapshot) {
	if len(snap.Redaction.AppliedRules) == 0 {
		return
	}
	parts := make([]string, len(snap.Redaction.AppliedRules))
	for i, r := range snap.Redaction.AppliedRules {
		parts[i] = "`" + r + "`"
	}
	fmt.Fprintf(b, "> 🛡️ %s\n\n", i18n.Tf(i18n.Normalize(snap.Locale),
		"share.redactedBeforePublish", map[string]string{"items": strings.Join(parts, ", ")}))
}

func writeConversation(b *strings.Builder, messages []Message, locale string) {
	if len(messages) == 0 {
		fmt.Fprintf(b, "%s\n\n", i18n.T(locale, "share.noMessagesMarkdown"))
		return
	}
	for i, msg := range messages {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMessage(b, msg, locale)
	}
}

func writeMessage(b *strings.Builder, msg Message, locale string) {
	fmt.Fprintf(b, "### %s\n\n", messageHeading(msg.Role, locale))
	wroteAny := false
	for _, block := range msg.Blocks {
		if writeBlock(b, block, msg.Role, locale) {
			wroteAny = true
		}
	}
	if !wroteAny {
		b.WriteString("_(empty)_\n\n")
	}
}

// messageHeading pairs a fixed emoji with the translated role name. An
// unrecognized role falls back to the raw value — that is data, not copy.
func messageHeading(role, locale string) string {
	switch role {
	case roleUser:
		return "🧑 " + i18n.T(locale, "share.headingUser")
	case roleAssistant:
		return "🤖 " + i18n.T(locale, "share.roleAssistant")
	case roleSystem:
		return "⚙️ " + i18n.T(locale, "share.roleSystem")
	}
	return role
}

// writeBlock writes a single block. Returns true if it produced any output —
// callers use this to detect "all blocks were empty" so they can show a
// placeholder instead of a bare heading.
func writeBlock(b *strings.Builder, block Block, role, locale string) bool {
	switch block.Kind {
	case blockKindText:
		return writeText(b, block.Text, role)
	case blockKindToolCall:
		return writeToolCall(b, block)
	case blockKindToolResult:
		return writeToolResult(b, block, locale)
	case blockKindDiff:
		return writeDiff(b, block)
	}
	return false
}

// writeText renders prose. User text is wrapped in a blockquote so the
// reader gets a visual accent that distinguishes the question from the
// agent's answer; assistant text is rendered as plain markdown so its
// own headings/lists/code blocks survive the round trip.
func writeText(b *strings.Builder, text, role string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	if role == roleUser {
		for _, line := range strings.Split(t, "\n") {
			fmt.Fprintf(b, "> %s\n", line)
		}
		b.WriteString("\n")
		return true
	}
	b.WriteString(t)
	b.WriteString("\n\n")
	return true
}

func writeToolCall(b *strings.Builder, block Block) bool {
	name := escapeHTML(nonEmpty(block.ToolName, "tool"))
	summary := strings.TrimSpace(block.Text)
	if summary != "" {
		fmt.Fprintf(b, "<details>\n<summary>🔧 <strong>%s</strong> — %s</summary>\n\n", name, escapeHTML(summary))
	} else {
		fmt.Fprintf(b, "<details>\n<summary>🔧 <strong>%s</strong></summary>\n\n", name)
	}
	if len(block.Args) > 0 {
		// Render via an HTML <pre> rather than a triple-backtick fence so a
		// JSON arg containing literal ``` sequences (commands, code snippets)
		// can't break out of the code block and corrupt downstream rendering.
		fmt.Fprintf(b, "<pre><code class=\"language-json\">%s</code></pre>\n",
			escapeHTML(prettyJSON(block.Args)))
	}
	b.WriteString("\n</details>\n\n")
	return true
}

func writeToolResult(b *strings.Builder, block Block, locale string) bool {
	if strings.TrimSpace(block.Output) == "" {
		return false
	}
	label := "📤 " + i18n.T(locale, "share.toolOutput")
	if block.Truncated {
		label = "📤 <em>" + i18n.T(locale, "share.toolOutputTruncated") + "</em>"
	}
	// HTML <pre> avoids the triple-backtick collision when the tool output
	// itself contains a code fence.
	fmt.Fprintf(b, "<details>\n<summary>%s</summary>\n\n<pre><code>%s</code></pre>\n\n</details>\n\n",
		label, escapeHTML(strings.TrimRight(block.Output, "\n")))
	return true
}

func writeDiff(b *strings.Builder, block Block) bool {
	if strings.TrimSpace(block.UnifiedDiff) == "" {
		return false
	}
	path := nonEmpty(block.Path, "diff")
	fmt.Fprintf(b, "**📝 `%s`**\n\n```diff\n%s\n```\n\n", path, strings.TrimRight(block.UnifiedDiff, "\n"))
	return true
}

func writeFooter(b *strings.Builder, snap *Snapshot) {
	locale := i18n.Normalize(snap.Locale)
	b.WriteString("\n---\n\n")
	fmt.Fprintf(b, "<sub>📦 %s · ", i18n.T(locale, "share.rawExport"))
	b.WriteString(i18n.T(locale, "share.builtWith"))
	if snap.KandevVersion != "" {
		fmt.Fprintf(b, " %s", snap.KandevVersion)
	}
	b.WriteString("</sub>\n")
}

// escapeHTML escapes characters that would break out of a <summary> tag.
// Markdown inside HTML is mostly inert, so this is enough.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func formatPtrTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

func prettyJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
