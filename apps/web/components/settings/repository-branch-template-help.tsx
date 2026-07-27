"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { msg } from "@lingui/core/macro";
import type { MessageDescriptor } from "@lingui/core";
import { IconInfoCircle } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";

const branchTemplatePlaceholders: Array<[string, MessageDescriptor]> = [
  [
    "{title}",
    msg`Task title sanitized to lowercase ASCII, hyphen-separated, max 20 chars. Example: fix-login-flow.`,
  ],
  [
    "{title_full}",
    msg`Same sanitizing as title, but max 80 chars. Example: fix-login-flow-after-session-timeout.`,
  ],
  [
    "{ticket}",
    msg`Task identifier first; otherwise Jira, Linear, GitHub issue, or GitHub PR metadata. Examples: KAN-123, #42.`,
  ],
  ["{issue_key}", msg`Alias for ticket. Use whichever name reads better in your template.`],
  [
    "{task_id}",
    msg`Kandev task UUID, sanitized for branch names. Example: 1f1cf094-db3c-4f42-b425-2cc14a2f7c74.`,
  ],
  [
    "{suffix}",
    msg`Short random suffix. Optional, but recommended to avoid branch name clashes. Example: x7p9.`,
  ],
];

const branchTemplateExample = "feature/{ticket}-{title}";

export function RepositoryBranchTemplateHelp() {
  const { t } = useLingui();
  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          aria-label={t`Branch template placeholders`}
          className="cursor-help text-muted-foreground hover:text-foreground"
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-96 text-xs">
        <div className="space-y-2">
          <p className="text-muted-foreground">
            <Trans>
              Write literal prefixes directly, for example{" "}
              <code className="rounded bg-muted px-1 py-0.5">{branchTemplateExample}</code>.
            </Trans>
          </p>
          <dl className="space-y-1.5">
            {branchTemplatePlaceholders.map(([name, description]) => (
              <div key={name} className="grid grid-cols-[5.5rem_1fr] gap-2">
                <dt className="font-mono text-foreground">{name}</dt>
                <dd className="text-muted-foreground">{t(description)}</dd>
              </div>
            ))}
          </dl>
        </div>
      </HoverCardContent>
    </HoverCard>
  );
}
