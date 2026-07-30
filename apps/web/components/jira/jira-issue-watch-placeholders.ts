import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const JIRA_ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.key",
    descriptionKey: "jira:ticketKey",
    example: "PROJ-42",
    executor_types: [],
  },
  {
    key: "issue.summary",
    descriptionKey: "jira:ticketSummary",
    example: "Login fails on mobile",
    executor_types: [],
  },
  {
    key: "issue.url",
    descriptionKey: "jira:ticketUrl",
    example: "https://acme.atlassian.net/browse/PROJ-42",
    executor_types: [],
  },
  {
    key: "issue.status",
    descriptionKey: "jira:statusName",
    example: "In Progress",
    executor_types: [],
  },
  {
    key: "issue.priority",
    descriptionKey: "jira:priority",
    example: "High",
    executor_types: [],
  },
  {
    key: "issue.type",
    descriptionKey: "jira:issueType",
    example: "Bug",
    executor_types: [],
  },
  {
    key: "issue.assignee",
    descriptionKey: "jira:assigneeDisplayName",
    example: "Alice Example",
    executor_types: [],
  },
  {
    key: "issue.reporter",
    descriptionKey: "jira:reporterDisplayName",
    example: "Bob Example",
    executor_types: [],
  },
  {
    key: "issue.project",
    descriptionKey: "jira:projectKey",
    example: "PROJ",
    executor_types: [],
  },
  {
    key: "issue.description",
    descriptionKey: "jira:ticketDescription",
    example: "When clicking login...",
    executor_types: [],
  },
];

export const DEFAULT_JIRA_ISSUE_WATCH_PROMPT = `You have been assigned a JIRA ticket to work on.

**Ticket:** {{issue.url}}
**Key:** {{issue.key}}
**Summary:** {{issue.summary}}
**Type:** {{issue.type}}
**Status:** {{issue.status}}
**Priority:** {{issue.priority}}
**Assignee:** {{issue.assignee}}
**Reporter:** {{issue.reporter}}
**Project:** {{issue.project}}

## Description

{{issue.description}}

## Instructions

1. Read the ticket carefully and understand the requirements.
2. Explore the codebase to understand the relevant code and architecture.
3. Implement the changes described in the ticket.
4. Write or update tests to cover the changes.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing the ticket key.`;
