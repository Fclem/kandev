"use client";

import { useEffect } from "react";
import { Trans } from "@lingui/react/macro";
import { useRouter } from "@/lib/routing/client-router";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";

export default function AgentEditPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/settings/agents");
  }, [router]);

  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-sm text-muted-foreground">
          <Trans>Manage agents from the main Agents page.</Trans>
        </p>
        <Button className="mt-4" onClick={() => router.push("/settings/agents")}>
          <Trans>Go to Agents</Trans>
        </Button>
      </CardContent>
    </Card>
  );
}
