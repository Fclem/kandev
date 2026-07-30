import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const LINEAR_ISSUE_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "issue.url",
    descriptionKey: "linear:linearIssueUrl",
    example: "https://linear.app/acme/issue/ENG-7",
    executor_types: [],
  },
  {
    key: "issue.identifier",
    descriptionKey: "linear:issueIdentifier",
    example: "ENG-7",
    executor_types: [],
  },
  {
    key: "issue.title",
    descriptionKey: "linear:issueTitle",
    example: "Login fails on mobile",
    executor_types: [],
  },
  {
    key: "issue.team",
    descriptionKey: "linear:teamKey",
    example: "ENG",
    executor_types: [],
  },
  {
    key: "issue.state",
    descriptionKey: "linear:workflowStateName",
    example: "In Progress",
    executor_types: [],
  },
  {
    key: "issue.priority",
    descriptionKey: "linear:priorityLabel",
    example: "High",
    executor_types: [],
  },
  {
    key: "issue.assignee",
    descriptionKey: "linear:assigneeDisplayName",
    example: "Alice",
    executor_types: [],
  },
  {
    key: "issue.creator",
    descriptionKey: "linear:issueCreatorDisplayName",
    example: "Bob",
    executor_types: [],
  },
  {
    key: "issue.description",
    descriptionKey: "linear:issueDescriptionBody",
    example: "Tap submit, nothing happens.",
    executor_types: [],
  },
];

// DEFAULT_LINEAR_ISSUE_WATCH_PROMPT mirrors apps/backend/config/prompts/linear-issue-watch-default.md.
// Kept in sync by hand: the UI shows this when the user clears the field, and
// the backend reads the .md when the saved prompt is empty. Diverging would
// surprise the user — they'd see one default in the dialog and another get
// sent to the agent.
export const DEFAULT_LINEAR_ISSUE_WATCH_PROMPT = `You have been assigned a Linear issue to work on.

**Issue:** {{issue.url}}
**Identifier:** {{issue.identifier}}
**Title:** {{issue.title}}
**Team:** {{issue.team}}
**State:** {{issue.state}}
**Priority:** {{issue.priority}}
**Assignee:** {{issue.assignee}}

## Description

{{issue.description}}

## Instructions

1. Read the issue description carefully and understand the requirements.
2. Explore the codebase to understand the relevant code and architecture.
3. Implement the changes described in the issue.
4. Write or update tests to cover the changes.
5. Run the test suite to ensure nothing is broken.
6. Commit your changes with a descriptive commit message referencing {{issue.identifier}}.`;
