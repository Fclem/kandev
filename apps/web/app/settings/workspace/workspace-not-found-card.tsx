"use client";

import { Trans } from "@lingui/react/macro";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";

export function WorkspaceNotFoundCard({ onBack }: { onBack: () => void }) {
  return (
    <div>
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">
            <Trans>Workspace not found</Trans>
          </p>
          <Button className="mt-4" onClick={onBack}>
            <Trans>Back to Workspaces</Trans>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
