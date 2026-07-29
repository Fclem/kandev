"use client";

// One editable source row inside the "Add to workspace" surface, plus the
// per-source-type editors it delegates to (local path / folder and remote
// repository). Saved repositories have their own row component.

import { useEffect } from "react";
import { IconX } from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { useBranchesByURL } from "@/hooks/domains/github/use-branches-by-url";
import { usePRInfoByURL } from "@/hooks/domains/github/use-pr-info-by-url";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { FolderPicker } from "@/components/folder-picker";
import { RemoteRepoChip } from "@/components/task-create-dialog-remote-repo-chip";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import type { LocalRepository, Repository } from "@/lib/types/http";
import { type WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import type { getWorkspaceSourceCapabilities } from "@/components/workspace-source-picker/executor-capabilities";
import { SavedRepositorySourceRow } from "./saved-repository-source-row";
import { useTranslation } from "react-i18next";

export function SourceRow({
  row,
  repositories,
  discoveredRepositories,
  workspaceId,
  repositoriesRefreshing,
  onRefreshRepositories,
  capabilities,
  error,
  onRemove,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  workspaceId: string | null;
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  error?: string;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const { t } = useTranslation();
  const type = row.sourceType ?? (row.kind === "folder" ? "folder" : "saved_repository");
  return (
    <fieldset className="space-y-2 rounded border p-3" data-testid="workspace-source-row">
      <div className="flex items-center justify-between">
        <legend className="text-sm font-medium">{labelFor(type)}</legend>
        <button
          type="button"
          aria-label={t("task:removeSource")}
          className="min-h-11 min-w-11 cursor-pointer text-muted-foreground"
          onClick={() => onRemove(row.key)}
        >
          <IconX className="mx-auto h-4 w-4" />
        </button>
      </div>
      {type === "saved_repository" && (
        <SavedRepositorySourceRow
          row={row}
          repositories={repositories}
          discoveredRepositories={discoveredRepositories}
          workspaceId={workspaceId}
          canCreateRepository={!capabilities.requiresCloneableLocalRepository}
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={onRefreshRepositories}
          onUpdate={onUpdate}
        />
      )}
      {type === "local_repository" && (
        <LocalPathRow
          row={row}
          label="Choose local Git repository"
          requiresCloneableOrigin={capabilities.requiresCloneableLocalRepository}
          onUpdate={onUpdate}
        />
      )}
      {type === "remote_repository" && (
        <RemoteRepositoryRow row={row} workspaceId={workspaceId} onUpdate={onUpdate} />
      )}
      {type === "folder" && (
        <LocalPathRow row={row} label="Choose local folder" onUpdate={onUpdate} />
      )}
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </fieldset>
  );
}

function labelFor(type: NonNullable<WorkspaceSourceRow["sourceType"]>) {
  switch (type) {
    case "saved_repository":
      return "Workspace repository";
    case "local_repository":
      return "Local Git repository";
    case "remote_repository":
      return "Remote repository";
    case "folder":
      return "Folder";
  }
}

function LocalPathRow({
  row,
  label,
  requiresCloneableOrigin = false,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  label: string;
  requiresCloneableOrigin?: boolean;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FolderPicker
        value={row.localPath ?? ""}
        onChange={(localPath) =>
          onUpdate(row.key, { localPath, repositoryId: undefined, remoteUrl: undefined })
        }
        placeholder={label}
      />
      {row.sourceType === "folder" && (
        <Input
          aria-label={t("task:folderDisplayName")}
          placeholder={t("task:displayNameOptional")}
          value={row.displayName ?? ""}
          onChange={(event) => onUpdate(row.key, { displayName: event.target.value })}
        />
      )}
      {row.sourceType === "local_repository" && (
        <>
          <Input
            aria-label={t("task:baseBranch")}
            placeholder={t("task:baseBranch")}
            value={row.baseBranch ?? ""}
            onChange={(event) => onUpdate(row.key, { baseBranch: event.target.value })}
          />
          <p className="text-sm text-muted-foreground">
            {requiresCloneableOrigin
              ? "This repository must have a cloneable origin; Kandev will verify it before adding."
              : "Uses the current checkout. Kandev does not switch your local repository branch."}
          </p>
        </>
      )}
    </>
  );
}

function RemoteRepositoryRow({
  row,
  workspaceId,
  onUpdate,
}: {
  row: WorkspaceSourceRow;
  workspaceId: string | null;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
}) {
  const branches = useBranchesByURL(workspaceId);
  const prInfo = usePRInfoByURL(workspaceId);
  const accessibleRepos = useRemoteRepositories(workspaceId ?? "");
  useEffect(() => {
    if (row.remoteUrl) branches.ensure(row.remoteUrl);
  }, [branches, row.remoteUrl, workspaceId]);
  const remoteRow: TaskRemoteRepoRow = {
    key: row.key,
    url: row.remoteUrl ?? "",
    branch: row.baseBranch ?? "",
    source: "paste",
    provider: row.provider,
    providerRepoId: row.providerRepoId,
    providerOwner: row.providerOwner,
    providerName: row.providerName,
  };
  return (
    <>
      <RemoteRepoChip
        row={remoteRow}
        branches={branches.branches(remoteRow.url)}
        branchesLoading={branches.loading(remoteRow.url)}
        prInfo={prInfo.info(remoteRow.url)}
        accessibleRepos={accessibleRepos}
        onURLChange={(remoteUrl, _, metadata) =>
          onUpdate(row.key, {
            remoteUrl,
            repositoryId: undefined,
            localPath: undefined,
            provider: metadata?.provider,
            providerRepoId: metadata?.providerRepoId,
            providerOwner: metadata?.providerOwner,
            providerName: metadata?.providerName,
            baseBranch: metadata?.defaultBranch ?? "",
          })
        }
        onBranchChange={(baseBranch) => onUpdate(row.key, { baseBranch })}
        onRemove={() => {}}
      />
    </>
  );
}
