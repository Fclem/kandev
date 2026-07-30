"use client";

import { useCallback } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconArrowsLeftRight, IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Trans, useTranslation } from "react-i18next";

export function ConfigSection() {
  const { t } = useTranslation();
  const router = useRouter();

  const handleSync = useCallback(() => {
    router.push("/office/workspace/settings/sync");
  }, [router]);

  const handleExport = useCallback(() => {
    router.push("/office/workspace/settings/export");
  }, [router]);

  return (
    <section className="space-y-4">
      <h2 className="text-sm font-semibold">{t("office:configuration")}</h2>
      <p className="text-xs text-muted-foreground">{t("office:syncTheWorkspaceDatabaseWithOn")}</p>
      <div className="flex gap-2">
        <Button variant="outline" onClick={handleSync} className="cursor-pointer">
          <Trans i18nKey="office:sync2">
            <IconArrowsLeftRight className="h-4 w-4 mr-1" />
            {t("office:sync")}
          </Trans>
        </Button>
        <Button variant="outline" onClick={handleExport} className="cursor-pointer">
          <Trans i18nKey="office:export">
            <IconDownload className="h-4 w-4 mr-1" />
            {t("office:export2")}
          </Trans>
        </Button>
      </div>
    </section>
  );
}
