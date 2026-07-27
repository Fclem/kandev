"use client";

import { Trans } from "@lingui/react/macro";
import Link from "@/components/routing/app-link";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { IconRefresh, IconArrowRight } from "@tabler/icons-react";
import { useUpdates } from "@/hooks/domains/system/use-updates";

export function VersionSummaryCard() {
  const { updates } = useUpdates();
  const current = updates?.current ?? "-";
  const latest = updates?.latest ?? "-";
  const updateAvailable = updates?.update_available ?? false;

  return (
    <Card data-testid="system-version-summary-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconRefresh className="h-4 w-4" />
          <Trans>Version</Trans>
          {updateAvailable && (
            <Badge variant="default" className="text-[10px]">
              <Trans>Update available</Trans>
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid grid-cols-2 gap-3 text-sm">
          <div>
            <div className="text-xs text-muted-foreground">
              <Trans>Current</Trans>
            </div>
            <div className="font-mono text-sm" data-testid="system-version-current">
              {current}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">
              <Trans>Latest</Trans>
            </div>
            <div className="font-mono text-sm" data-testid="system-version-latest">
              {latest}
            </div>
          </div>
        </div>
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <Link href="/settings/system/updates" data-testid="system-version-updates-link">
            <Trans>View updates</Trans>
            <IconArrowRight className="h-3.5 w-3.5 ml-1" />
          </Link>
        </Button>
      </CardContent>
    </Card>
  );
}
