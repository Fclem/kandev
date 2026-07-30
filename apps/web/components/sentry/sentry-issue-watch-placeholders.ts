import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const SENTRY_ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.short_id",
    descriptionKey: "sentry:sentryShortId",
    example: "PROJ-123",
    executor_types: [],
  },
  {
    key: "issue.title",
    descriptionKey: "sentry:issueTitle",
    example: "TypeError: cannot read property of undefined",
    executor_types: [],
  },
  {
    key: "issue.url",
    descriptionKey: "sentry:sentryIssuePermalink",
    example: "https://sentry.io/organizations/acme/issues/123/",
    executor_types: [],
  },
  {
    key: "issue.project",
    descriptionKey: "sentry:sentryProjectSlug",
    example: "frontend",
    executor_types: [],
  },
  {
    key: "issue.level",
    descriptionKey: "sentry:issueLevel",
    example: "error",
    executor_types: [],
  },
  {
    key: "issue.status",
    descriptionKey: "sentry:issueStatus",
    example: "unresolved",
    executor_types: [],
  },
  {
    key: "issue.culprit",
    descriptionKey: "sentry:culpritFunctionOrFile",
    example: "app/routes/checkout.tsx in handleSubmit",
    executor_types: [],
  },
  {
    key: "issue.assignee",
    descriptionKey: "sentry:assigneeDisplayName",
    example: "Alice",
    executor_types: [],
  },
  {
    key: "issue.count",
    descriptionKey: "sentry:totalEventCount",
    example: "42",
    executor_types: [],
  },
  {
    key: "issue.user_count",
    descriptionKey: "sentry:uniqueUsersAffected",
    example: "7",
    executor_types: [],
  },
  {
    key: "issue.first_seen",
    descriptionKey: "sentry:whenTheIssueWasFirstSeen",
    example: "2026-05-20T11:30:00Z",
    executor_types: [],
  },
  {
    key: "issue.last_seen",
    descriptionKey: "sentry:whenTheIssueWasLastSeen",
    example: "2026-05-23T08:14:00Z",
    executor_types: [],
  },
];

// DEFAULT_SENTRY_ISSUE_WATCH_PROMPT mirrors apps/backend/config/prompts/sentry-issue-watch-default.md.
// Kept in sync by hand: the UI shows this when the user clears the field, and
// the backend reads the .md when the saved prompt is empty.
export const DEFAULT_SENTRY_ISSUE_WATCH_PROMPT = `You have been assigned a Sentry issue to triage and fix.

**Issue:** {{issue.url}}
**Short ID:** {{issue.short_id}}
**Title:** {{issue.title}}
**Project:** {{issue.project}}
**Level:** {{issue.level}}
**Status:** {{issue.status}}
**Assignee:** {{issue.assignee}}
**Culprit:** {{issue.culprit}}
**Events:** {{issue.count}} ({{issue.user_count}} users affected)
**First seen:** {{issue.first_seen}}
**Last seen:** {{issue.last_seen}}

## Instructions

1. Open the issue in Sentry and read the full stack trace.
2. Reproduce the bug locally if possible.
3. Implement the fix and add a regression test.
4. Run the test suite to ensure nothing else broke.
5. Commit your changes with a descriptive message referencing {{issue.short_id}}.`;
