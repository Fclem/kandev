"use client";
import { Trans, useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";

const branchTemplatePlaceholders: Array<[string, string]> = [
  ["{title}", "settings:taskTitleSanitizedToLowercaseAscii"],
  ["{title_full}", "settings:sameSanitizingAsTitleButMax"],
  ["{ticket}", "settings:taskIdentifierFirstOtherwiseJiraLinear"],
  ["{issue_key}", "settings:aliasForTicketUseWhicheverName"],
  ["{task_id}", "settings:kandevTaskUuidSanitizedForBranch"],
  ["{suffix}", "settings:shortRandomSuffixOptionalButRecommended"],
];

const branchTemplateExample = "feature/{ticket}-{title}";

export function RepositoryBranchTemplateHelp() {
  const { t } = useTranslation();
  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          aria-label={t("settings:branchTemplatePlaceholders")}
          className="cursor-help text-muted-foreground hover:text-foreground"
        >
          <IconInfoCircle className="h-3.5 w-3.5" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-96 text-xs">
        <div className="space-y-2">
          <p className="text-muted-foreground">
            <Trans
              i18nKey="settings:writeLiteralPrefixesDirectlyForExample"
              values={{ branchTemplateExample }}
            >
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
