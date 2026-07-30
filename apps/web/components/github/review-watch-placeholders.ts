import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

export const REVIEW_WATCH_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "pr.link",
    descriptionKey: "github:prUrl",
    example: "https://github.com/org/repo/pull/123",
    executor_types: [],
  },
  {
    key: "pr.number",
    descriptionKey: "github:prNumber",
    example: "123",
    executor_types: [],
  },
  {
    key: "pr.title",
    descriptionKey: "github:prTitle",
    example: "Add user authentication",
    executor_types: [],
  },
  {
    key: "pr.author",
    descriptionKey: "github:prAuthorUsername",
    example: "octocat",
    executor_types: [],
  },
  {
    key: "pr.repo",
    descriptionKey: "github:repositoryOwnerName",
    example: "org/repo",
    executor_types: [],
  },
  {
    key: "pr.branch",
    descriptionKey: "github:sourceBranch",
    example: "feature/auth",
    executor_types: [],
  },
  {
    key: "pr.base_branch",
    descriptionKey: "github:targetBranch",
    example: "main",
    executor_types: [],
  },
];

export const DEFAULT_REVIEW_WATCH_PROMPT = `Review Pull Request #{{pr.number}}: {{pr.title}}
Repository: {{pr.repo}}
PR: {{pr.link}}
Author: {{pr.author}}
Branch: {{pr.branch}} → {{pr.base_branch}}

To see ONLY the PR changes, use:
- git diff origin/{{pr.base_branch}}...HEAD (three-dot = only changes on the PR branch)
- git log --oneline origin/{{pr.base_branch}}..HEAD (list PR commits)
Do NOT review files outside this diff.`;
