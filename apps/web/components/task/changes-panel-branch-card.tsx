"use client";

// Branch details surface for the Changes panel header: the hover card (or touch
// drawer) that shows each repo's task branch, its merge target, the inline
// rename dialog, and the resolved git credential.

import { useState } from "react";
import { IconArrowRight, IconEdit, IconGitBranch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import type { PerRepoStatus } from "@/components/vcs-multi-repo-menu";
import { BaseBranchPicker } from "./base-branch-picker";
import type { GitCredentialDisplay } from "./changes-git-credential-display";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTranslation } from "react-i18next";

export type BranchRow = {
  repoLabel: string | null;
  branch: string;
  baseBranch: string;
  /** Name agentctl emits for this repo (= worktree dir basename). Empty for
   *  single-repo workspaces; passed to the BaseBranchPicker so it can resolve
   *  the task_repositories row to PATCH. */
  repositoryName: string;
};

export type RenameBranchResult = {
  success: boolean;
  error?: string;
};

/**
 * Builds per-repo rows for the branch hover card. Returns [] for single-repo
 * workspaces (callers fall back to the single-row layout); otherwise one row
 * per named repo with that repo's task base_branch (or the workspace-level
 * fallback when none was recorded).
 */
export function buildBranchRows(
  perRepoStatus: PerRepoStatus[],
  baseBranchByRepo: Record<string, string> | undefined,
  baseBranchFallback: string,
  repoDisplayName: ((name: string) => string | undefined) | undefined,
): BranchRow[] {
  const named = perRepoStatus.filter((s) => s.repository_name !== "" && s.branch);
  if (named.length <= 1) return [];
  return named.map((s) => ({
    repoLabel: repoDisplayName?.(s.repository_name) || s.repository_name,
    branch: s.branch ?? "",
    baseBranch: baseBranchByRepo?.[s.repository_name] || baseBranchFallback,
    repositoryName: s.repository_name,
  }));
}

function RenameBranchButton({
  branch,
  repositoryName,
  onRenameBranch,
  isRenaming,
}: {
  branch: string;
  repositoryName: string;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [newBranchName, setNewBranchName] = useState(branch);
  const [error, setError] = useState<string | null>(null);
  const trimmedBranchName = newBranchName.trim();
  const canRename = !!onRenameBranch && trimmedBranchName !== "" && trimmedBranchName !== branch;
  const submitRename = async () => {
    if (!onRenameBranch || !canRename) return;
    setError(null);
    try {
      const result = await onRenameBranch(trimmedBranchName, repositoryName);
      if (!result.success) {
        setError(result.error || "Failed to rename branch");
        return;
      }
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to rename branch");
    }
  };
  const openDialog = () => {
    setNewBranchName(branch);
    setError(null);
    setOpen(true);
  };
  return (
    <>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="h-6 w-6 shrink-0 text-muted-foreground hover:text-foreground"
        disabled={!onRenameBranch || isRenaming}
        aria-label={t("task:editBranch2", { branch })}
        onClick={openDialog}
      >
        <IconEdit className="h-3.5 w-3.5" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("task:editBranch")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor={`branch-name-${repositoryName || "default"}`}>
              {t("task:branchName")}
            </Label>
            <Input
              id={`branch-name-${repositoryName || "default"}`}
              value={newBranchName}
              onChange={(event) => setNewBranchName(event.target.value)}
              autoFocus
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              {t("common:cancel")}
            </Button>
            <Button
              type="button"
              data-dialog-default-action
              disabled={!canRename || isRenaming}
              onClick={submitRename}
            >
              {t("task:save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function BranchRowView({
  repoLabel,
  branch,
  baseBranch,
  repositoryName,
  taskId,
  onRenameBranch,
  isRenaming,
}: BranchRow & {
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
}) {
  return (
    <div className="flex items-center gap-2">
      {repoLabel && (
        <span className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[10px] font-medium text-muted-foreground max-w-[8rem] truncate">
          {repoLabel}
        </span>
      )}
      <span className="flex min-w-0 items-center gap-1.5 text-foreground font-medium">
        <IconGitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{branch}</span>
      </span>
      <RenameBranchButton
        branch={branch}
        repositoryName={repositoryName}
        onRenameBranch={onRenameBranch}
        isRenaming={isRenaming}
      />
      <div className="flex min-w-8 flex-1 items-center text-muted-foreground/40">
        <div className="h-px flex-1 bg-muted-foreground/20" />
        <IconArrowRight className="-ml-px h-3 w-3 shrink-0" />
      </div>
      <BaseBranchPicker
        taskId={taskId}
        repositoryName={repositoryName}
        fallbackBaseBranch={baseBranch}
      />
    </div>
  );
}

export function BranchHoverCard({
  displayBranch,
  baseBranchDisplay,
  rows,
  taskId,
  onRenameBranch,
  isRenaming,
  credentialDisplay,
}: {
  displayBranch: string;
  baseBranchDisplay: string;
  /** When non-empty, the card renders one row per repo instead of the single
   *  workspace-level pair. Single-repo workspaces leave this undefined. */
  rows?: BranchRow[];
  /** Active task id, plumbed into BaseBranchPicker for the PATCH call. */
  taskId: string | null;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  isRenaming: boolean;
  credentialDisplay: GitCredentialDisplay | null;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const isMulti = rows && rows.length > 0;
  const headerLabel = isMulti ? "Your branches:" : "Your code lives in:";
  const trailerLabel = "comparing against:";
  const trigger = (
    <button
      type="button"
      className="flex items-center justify-center size-5 rounded hover:bg-muted/60 text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
      aria-label={t("task:showBranchAndGitCredentialDetails")}
    >
      <IconGitBranch className="h-3.5 w-3.5" />
    </button>
  );
  const content = (
    <div className="flex flex-col gap-2.5 text-xs">
      <div className="flex items-center justify-between gap-6">
        <span className="text-muted-foreground/60">{headerLabel}</span>
        <span className="text-muted-foreground/60">{trailerLabel}</span>
      </div>
      {isMulti ? (
        <div className="flex flex-col gap-1.5">
          {rows!.map((row) => (
            <BranchRowView
              key={row.repoLabel ?? row.branch}
              {...row}
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={isRenaming}
            />
          ))}
        </div>
      ) : (
        <BranchRowView
          repoLabel={null}
          branch={displayBranch}
          baseBranch={baseBranchDisplay}
          repositoryName=""
          taskId={taskId}
          onRenameBranch={onRenameBranch}
          isRenaming={isRenaming}
        />
      )}
      {credentialDisplay && (
        <div className="border-t pt-2 text-muted-foreground" data-testid="changes-git-credential">
          <p className="font-medium text-foreground">{credentialDisplay.source}</p>
          <p>{credentialDisplay.detail}</p>
          <p>{credentialDisplay.transport}</p>
        </div>
      )}
    </div>
  );
  if (usesTouchDrawer) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>{t("task:branchDetails")}</DrawerTitle>
            <DrawerDescription>{t("task:branchComparisonAndTaskGitCredentials")}</DrawerDescription>
          </DrawerHeader>
          <div className="px-4 pb-6">{content}</div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <HoverCard openDelay={200} closeDelay={100}>
      <HoverCardTrigger asChild>{trigger}</HoverCardTrigger>
      <HoverCardContent forceMount side="bottom" align="end" className="w-auto p-3">
        {content}
      </HoverCardContent>
    </HoverCard>
  );
}
