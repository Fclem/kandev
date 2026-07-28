import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import {
  IconBrandGithub,
  IconBrandGitlab,
  IconBrandAzure,
  IconBrandSentry,
  IconBrandSlack,
  IconHexagon,
  IconTicket,
} from "@tabler/icons-react";
import { Card, CardContent } from "@kandev/ui/card";

type IntegrationEntry = {
  slug: string;
  label: string;
  description: string;
  Icon: typeof IconBrandAzure;
};

const INTEGRATIONS: IntegrationEntry[] = [
  {
    slug: "azure-devops",
    label: "Azure DevOps",
    description: "settings:azureBoardsWorkItemsAndAzure",
    Icon: IconBrandAzure,
  },
  {
    slug: "github",
    label: "GitHub",
    description: "settings:prReviewQueuesIssueWatchersAnd",
    Icon: IconBrandGithub,
  },
  {
    slug: "gitlab",
    label: "GitLab",
    description: "settings:mergeRequestCreationDiscussionRepliesAnd",
    Icon: IconBrandGitlab,
  },
  {
    slug: "jira",
    label: "Jira",
    description: "settings:atlassianCloudCredentialsAndJqlIssue",
    Icon: IconTicket,
  },
  {
    slug: "linear",
    label: "Linear",
    description: "settings:personalApiKeyAndTeamDefaults",
    Icon: IconHexagon,
  },
  {
    slug: "sentry",
    label: "Sentry",
    description: "settings:authTokenOrgProjectDefaultsAnd",
    Icon: IconBrandSentry,
  },
  {
    slug: "slack",
    label: "Slack",
    description: "settings:browserSessionCredentialsAndKandevTriage",
    Icon: IconBrandSlack,
  },
];

type IntegrationsIndexPageProps = {
  workspaceId?: string;
};

export default function IntegrationsIndexPage({ workspaceId }: IntegrationsIndexPageProps = {}) {
  const { t } = useTranslation();
  const rootHref = workspaceId
    ? `/settings/workspace/${encodeURIComponent(workspaceId)}/integrations`
    : "/settings/integrations";

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">{t("common:integrations")}</h2>
        <p className="text-sm text-muted-foreground mt-1">
          {t("settings:connectKandevToThirdPartyServices")}
        </p>
      </div>
      <div className="grid auto-rows-fr gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {INTEGRATIONS.map(({ slug, label, description, Icon }) => {
          const href = `${rootHref}/${slug}`;
          return (
            <Link key={href} href={href} className="flex h-full cursor-pointer">
              <Card className="h-full w-full transition-colors hover:border-primary/40">
                <CardContent className="space-y-2">
                  <div className="flex items-center gap-2 text-base font-semibold">
                    <Icon className="h-5 w-5" />
                    {label}
                  </div>
                  <p className="text-sm text-muted-foreground">{t(description)}</p>
                </CardContent>
              </Card>
            </Link>
          );
        })}
      </div>
    </div>
  );
}
