"use client";

import { Trans, useLingui } from "@lingui/react/macro";
import { IconBell } from "@tabler/icons-react";
import { Separator } from "@kandev/ui/separator";
import { SettingsSection } from "@/components/settings/settings-section";
import { ChangelogNotificationCard } from "@/components/settings/changelog-notification-card";
import { ChangelogList } from "@/components/settings/changelog-list";

export function ChangelogSettings() {
  const { t } = useLingui();
  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold">
          <Trans>Changelog</Trans>
        </h2>
        <p className="text-sm text-muted-foreground mt-1">
          <Trans>View all releases and manage notification preferences</Trans>
        </p>
      </div>

      <Separator />

      <SettingsSection
        icon={<IconBell className="h-5 w-5" />}
        title={t`Notifications`}
        description={t`Control the release notes notification in the topbar`}
      >
        <ChangelogNotificationCard />
      </SettingsSection>

      <Separator />

      <SettingsSection
        title={t`Release History`}
        description={t`All versions and their release notes`}
      >
        <ChangelogList />
      </SettingsSection>
    </div>
  );
}
