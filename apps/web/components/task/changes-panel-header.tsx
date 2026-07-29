"use client";

import {
  IconCloudDownload,
  IconEye,
  IconChevronDown,
  IconGitCherryPick,
  IconGitMerge,
  IconLoader2,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import type { PerRepoStatus } from "@/components/vcs-multi-repo-menu";
import { PanelHeaderBarSplit } from "./panel-primitives";
import {
  BranchHoverCard,
  buildBranchRows,
  type RenameBranchResult,
} from "./changes-panel-branch-card";
import type { GitCredentialDisplay } from "./changes-git-credential-display";
import { useTranslation } from "react-i18next";

function PullTriggerContent({
  behindCount,
  isPulling,
  isRebasing,
}: {
  behindCount: number;
  isPulling: boolean;
  isRebasing: boolean;
}) {
  const isPullRelated = isPulling || isRebasing;
  let label: string;
  if (isPulling) label = "Pulling…";
  else if (isRebasing) label = "Rebasing…";
  else label = "Pull";
  return (
    <>
      {isPullRelated ? (
        <IconLoader2 className="h-3 w-3 animate-spin" />
      ) : (
        <IconCloudDownload className="h-3 w-3" />
      )}
      {label}
      {behindCount > 0 && !isPullRelated && (
        <span className="text-yellow-500 text-[10px]">{behindCount}</span>
      )}
      {!isPullRelated && <IconChevronDown className="h-2.5 w-2.5 text-muted-foreground" />}
    </>
  );
}

function PullDropdown({
  behindCount,
  isLoading,
  loadingOperation,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
}: {
  behindCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  /** Maps a repository_name to its display label. */
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const isPulling = loadingOperation === "pull";
  const isRebasing = loadingOperation === "rebase";
  // For single-repo (empty repo entry), the trigger label uses the global
  // behindCount; for multi-repo we show the per-repo behinds inside the menu
  // labels and the trigger summarises with the max.
  const triggerBehind =
    perRepoStatus.length > 0
      ? Math.max(behindCount, ...perRepoStatus.map((s) => s.behind))
      : behindCount;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
          disabled={isLoading}
        >
          <PullTriggerContent
            behindCount={triggerBehind}
            isPulling={isPulling}
            isRebasing={isRebasing}
          />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <PerRepoPullMenu
          repoNames={repoNames}
          perRepoStatus={perRepoStatus}
          onRepoPull={onRepoPull}
          onRepoRebase={onRepoRebase}
          onRepoMerge={onRepoMerge}
          repoDisplayName={repoDisplayName}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function PerRepoPullMenu({
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
}: {
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  repoDisplayName?: (repositoryName: string) => string | undefined;
}) {
  const statusByName = new Map(perRepoStatus.map((s) => [s.repository_name, s]));
  return (
    <>
      {repoNames.map((repo, idx) => {
        const s = statusByName.get(repo);
        const behind = s?.behind ?? 0;
        const label = repoDisplayName?.(repo) || repo || "Repository";
        return (
          <div key={repo || "__no_repo__"}>
            {idx > 0 && <DropdownMenuSeparator />}
            <DropdownMenuLabel className="text-[10px] text-muted-foreground/70 uppercase tracking-wide flex items-center justify-between">
              <span className="truncate">{label}</span>
              {behind > 0 && (
                <span className="text-yellow-500 normal-case tracking-normal">{behind} behind</span>
              )}
            </DropdownMenuLabel>
            <DropdownMenuItem
              onClick={() => onRepoPull(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconCloudDownload className="h-3.5 w-3.5 text-muted-foreground" />
              Pull
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoRebase(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitCherryPick className="h-3.5 w-3.5 text-muted-foreground" />
              Rebase
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => onRepoMerge(repo)}
              className="cursor-pointer text-xs gap-2"
            >
              <IconGitMerge className="h-3.5 w-3.5 text-muted-foreground" />
              Merge
            </DropdownMenuItem>
          </div>
        );
      })}
    </>
  );
}

function ChangesPanelHeaderLeft({
  showDiffReview,
  onOpenDiffAll,
  onOpenReview,
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  showDiffReview: boolean;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  if (!showDiffReview) return null;
  return (
    <>
      <Button
        size="sm"
        variant="ghost"
        className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
        onClick={onOpenDiffAll}
      >
        <IconGitMerge className="h-3 w-3" />
        Diff
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
        onClick={onOpenReview}
      >
        <IconEye className="h-3 w-3" />
        Review
      </Button>
      {onRequestWalkthrough ? (
        <ChangesPanelWalkthroughButton
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      ) : null}
    </>
  );
}

function ChangesPanelWalkthroughButton({
  onRequestWalkthrough,
  requestWalkthroughDisabled,
}: {
  onRequestWalkthrough: () => void;
  requestWalkthroughDisabled?: boolean;
}) {
  const { t } = useTranslation();
  const tooltip = requestWalkthroughDisabled
    ? "Loading changed files..."
    : "Walk me through these changes";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex" tabIndex={requestWalkthroughDisabled ? 0 : undefined}>
          <Button
            size="sm"
            variant="ghost"
            className="h-5 text-[11px] px-1.5 gap-1 cursor-pointer"
            aria-label={t("task:walkMeThroughTheseChanges")}
            data-testid="changes-request-walkthrough"
            disabled={requestWalkthroughDisabled}
            onClick={onRequestWalkthrough}
          >
            <IconRoute className="h-3 w-3" />
            <span className="hidden min-[430px]:inline sm:inline">{t("task:walkthrough")}</span>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function ChangesPanelHeader({
  hasChanges,
  hasCommits,
  hasPRFiles,
  displayBranch,
  baseBranchDisplay,
  baseBranchByRepo,
  behindCount,
  isLoading,
  loadingOperation,
  onOpenDiffAll,
  onOpenReview,
  onRequestWalkthrough,
  requestWalkthroughDisabled,
  repoNames,
  perRepoStatus,
  onRepoPull,
  onRepoRebase,
  onRepoMerge,
  repoDisplayName,
  taskId,
  onRenameBranch,
  credentialDisplay,
}: {
  hasChanges: boolean;
  hasCommits: boolean;
  hasPRFiles?: boolean;
  displayBranch: string | null;
  baseBranchDisplay: string;
  /** Per-repo merge target, keyed by repository_name. Undefined entries fall
   *  back to baseBranchDisplay. Empty/missing for single-repo workspaces. */
  baseBranchByRepo?: Record<string, string>;
  behindCount: number;
  isLoading: boolean;
  loadingOperation: string | null;
  onOpenDiffAll?: () => void;
  onOpenReview?: () => void;
  onRequestWalkthrough?: () => void;
  requestWalkthroughDisabled?: boolean;
  /** Always non-empty (single-repo includes the empty-name entry). */
  repoNames: string[];
  perRepoStatus: PerRepoStatus[];
  onRepoPull: (repo: string) => void;
  onRepoRebase: (repo: string) => void;
  onRepoMerge: (repo: string) => void;
  onRenameBranch?: (newName: string, repo: string) => Promise<RenameBranchResult>;
  credentialDisplay: GitCredentialDisplay | null;
  repoDisplayName?: (repositoryName: string) => string | undefined;
  /** Active task id; piped into the base-branch picker so it can resolve
   *  the right task_repositories row to PATCH. Null while task data is
   *  hydrating — the picker falls back to a static label. */
  taskId: string | null;
}) {
  const branchRows = buildBranchRows(
    perRepoStatus,
    baseBranchByRepo,
    baseBranchDisplay,
    repoDisplayName,
  );
  const showDiffReview = hasChanges || hasCommits || !!hasPRFiles;
  return (
    <PanelHeaderBarSplit
      left={
        <ChangesPanelHeaderLeft
          showDiffReview={showDiffReview}
          onOpenDiffAll={onOpenDiffAll}
          onOpenReview={onOpenReview}
          onRequestWalkthrough={onRequestWalkthrough}
          requestWalkthroughDisabled={requestWalkthroughDisabled}
        />
      }
      right={
        <>
          {(displayBranch || branchRows.length > 0) && (
            <BranchHoverCard
              displayBranch={displayBranch ?? ""}
              baseBranchDisplay={baseBranchDisplay}
              rows={branchRows}
              taskId={taskId}
              onRenameBranch={onRenameBranch}
              isRenaming={loadingOperation === "rename_branch"}
              credentialDisplay={credentialDisplay}
            />
          )}
          <PullDropdown
            behindCount={behindCount}
            isLoading={isLoading}
            loadingOperation={loadingOperation}
            repoNames={repoNames}
            perRepoStatus={perRepoStatus}
            onRepoPull={onRepoPull}
            onRepoRebase={onRepoRebase}
            onRepoMerge={onRepoMerge}
            repoDisplayName={repoDisplayName}
          />
        </>
      }
    />
  );
}
