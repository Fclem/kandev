"use client";

import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import type { Workspace } from "@/lib/types/http";

export function WorkspaceSettingsHeader({
  workspace,
  description,
}: {
  workspace: Workspace;
  description: string;
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 className="text-2xl font-bold">{workspace.name}</h2>
        <p className="text-sm text-muted-foreground mt-1">{description}</p>
      </div>
      <Button asChild variant="outline" size="sm">
        <Link href={`/settings/workspace/${workspace.id}`}>Workspace settings</Link>
      </Button>
    </div>
  );
}
