"use client";

import { useCallback, useState, type RefObject, type ReactNode } from "react";
import {
  IconChevronDown,
  IconCloudDownload,
  IconFolderPlus,
  IconGitBranch,
  IconPlus,
  IconStack2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import type { LocalRepository, Repository } from "@/lib/types/http";
import { type WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import {
  getWorkspaceSourceCapabilities,
  hasCloneableSavedRepository,
} from "@/components/workspace-source-picker/executor-capabilities";
import { SourceRow } from "./workspace-source-row";
import { useDialogOpenerFocus } from "./use-dialog-opener-focus";
import { useSubmitWorkspaceSources } from "./use-submit-workspace-sources";
import { useWorkspaceRepositoryOptions } from "./use-workspace-repository-options";
import { useWorkspaceSourceRows } from "./use-workspace-source-rows";
import { WorkspaceChangeConsequences } from "./workspace-change-consequences";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  executorType?: string | null;
  workspaceId: string | null;
  /** The toolbar button that explicitly opened this controlled surface. */
  opener?: HTMLElement | null;
  openerRef?: RefObject<HTMLButtonElement | null>;
};

export function AddWorkspaceSourcesDialog({
  open,
  onOpenChange,
  taskId,
  executorType,
  workspaceId,
  opener,
  openerRef,
}: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const { repositories, discoveredRepositories, repositoriesRefreshing, refreshRepositoryOptions } =
    useWorkspaceRepositoryOptions(workspaceId, open);
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const sourceRows = useWorkspaceSourceRows(executorType);
  const reconcileWorkspaceSourcesAdopted = useAppStore(
    (state) => state.reconcileWorkspaceSourcesAdopted,
  );
  const { requestFocusRestoration, restoreOpenerFocus } = useDialogOpenerFocus({
    open,
    opener,
    openerRef,
  });
  const capabilities = getWorkspaceSourceCapabilities(executorType);
  const restartsWorkspace = !isRemoteWorkspaceExecutor(executorType);
  const close = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen && !submitting) {
        requestFocusRestoration();
        sourceRows.resetValidation();
        setSubmitError(null);
        onOpenChange(false);
      }
    },
    [onOpenChange, requestFocusRestoration, sourceRows, submitting],
  );
  const submit = useSubmitWorkspaceSources({
    errors: sourceRows.errors,
    onOpenChange,
    reconcileWorkspaceSourcesAdopted,
    rows: sourceRows.rows,
    submitting,
    taskId,
    onSuccess: () => {
      sourceRows.reset();
      requestFocusRestoration();
    },
    setSubmitting,
    setSubmitError,
  });
  return (
    <AddWorkspaceSourcesSurface
      isMobile={isMobile}
      open={open}
      onOpenChange={close}
      onCloseAutoFocus={restoreOpenerFocus}
      onDrawerCloseAnimationEnd={restoreOpenerFocus}
      error={submitError}
      consequences={<WorkspaceChangeConsequences restartsWorkspace={restartsWorkspace} />}
      form={
        <SourceForm
          rows={sourceRows.rows}
          workspaceId={workspaceId}
          errors={sourceRows.visibleErrors}
          repositories={selectableRepositories(repositories, capabilities)}
          discoveredRepositories={
            capabilities.requiresCloneableLocalRepository ? [] : discoveredRepositories
          }
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={refreshRepositoryOptions}
          capabilities={capabilities}
          onAdd={(kind) => {
            setSubmitError(null);
            sourceRows.add(kind);
          }}
          onRemove={(key) => {
            setSubmitError(null);
            sourceRows.remove(key);
          }}
          onUpdate={(key, patch) => {
            setSubmitError(null);
            sourceRows.update(key, patch);
          }}
          isMobile={isMobile}
        />
      }
      submitting={submitting}
      canSubmit={sourceRows.rows.length > 0}
      onCancel={() => close(false)}
      onSubmit={() => {
        sourceRows.validate();
        void submit();
      }}
    />
  );
}

function selectableRepositories(
  repositories: Repository[],
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>,
) {
  return capabilities.requiresCloneableLocalRepository
    ? repositories.filter(hasCloneableSavedRepository)
    : repositories;
}

function isRemoteWorkspaceExecutor(executorType: string | null | undefined): boolean {
  return ["local_docker", "remote_docker", "ssh", "sprites"].includes(executorType ?? "");
}

type AddWorkspaceSourcesSurfaceProps = {
  isMobile: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus: (event?: { preventDefault(): void }) => void;
  onDrawerCloseAnimationEnd: (event: { preventDefault(): void }) => void;
  error: string | null;
  consequences: ReactNode;
  form: ReactNode;
  submitting: boolean;
  canSubmit: boolean;
  onCancel: () => void;
  onSubmit: () => void;
};

function AddWorkspaceSourcesSurface({
  isMobile,
  open,
  onOpenChange,
  onCloseAutoFocus,
  onDrawerCloseAnimationEnd,
  error,
  consequences,
  form,
  submitting,
  canSubmit,
  onCancel,
  onSubmit,
}: AddWorkspaceSourcesSurfaceProps) {
  const { t } = useTranslation();
  const footer = (
    <div className="flex justify-end gap-2">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer"
        disabled={submitting}
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        data-testid="add-workspace-sources-submit"
        className="min-h-11 cursor-pointer"
        disabled={submitting || !canSubmit}
        onClick={onSubmit}
      >
        {submitting ? "Adding…" : "Add to workspace"}
      </Button>
    </div>
  );
  const errorMessage = error && (
    <p role="alert" className="text-sm text-destructive">
      {error}
    </p>
  );
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          data-testid="add-workspace-sources-drawer"
          onCloseAutoFocus={onCloseAutoFocus}
          onAnimationEnd={(event) => {
            if (event.currentTarget.dataset.state === "closed") onDrawerCloseAnimationEnd(event);
          }}
          className="h-dvh !max-h-dvh rounded-none flex flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:!mt-0"
        >
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("task:addToWorkspace")}</DrawerTitle>
            <DrawerDescription>{t("task:chooseRepositoriesOrFoldersToMake")}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4">
            {errorMessage}
            <div className="mb-4">{consequences}</div>
            {form}
          </div>
          <div className="shrink-0 border-t p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {footer}
          </div>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="add-workspace-sources-dialog"
        className="flex max-h-[calc(100dvh-2rem)] max-w-xl flex-col overflow-hidden"
        onCloseAutoFocus={onCloseAutoFocus}
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>{t("task:addToWorkspace")}</DialogTitle>
          <DialogDescription>{t("task:chooseRepositoriesOrFoldersToMake")}</DialogDescription>
        </DialogHeader>
        <div
          data-testid="add-workspace-sources-dialog-scroll"
          className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pr-1"
        >
          {errorMessage}
          {consequences}
          {form}
        </div>
        <DialogFooter className="shrink-0">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SourceForm({
  rows,
  repositories,
  discoveredRepositories,
  workspaceId,
  repositoriesRefreshing,
  onRefreshRepositories,
  errors,
  capabilities,
  onAdd,
  onRemove,
  onUpdate,
  isMobile,
}: {
  rows: WorkspaceSourceRow[];
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  workspaceId: string | null;
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  errors: Record<string, string>;
  capabilities: ReturnType<typeof getWorkspaceSourceCapabilities>;
  onAdd: (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) => void;
  onRemove: (key: string) => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
  isMobile: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 py-1" data-testid="add-workspace-sources-form">
      <div className="flex flex-wrap items-center gap-2">
        <RepositorySourceMenu isMobile={isMobile} onAdd={onAdd} />
        {capabilities.canAddFolders && (
          <Button
            type="button"
            variant="outline"
            className={cn("cursor-pointer", isMobile ? "min-h-11" : "h-9 px-3")}
            onClick={() => onAdd("folder")}
          >
            <IconFolderPlus className="h-4 w-4" />
            {t("task:addFolder")}
          </Button>
        )}
      </div>
      {capabilities.requiresCloneableLocalRepository && (
        <p className="text-sm text-muted-foreground">
          {t("task:savedAndLocalGitRepositoriesMust")}
        </p>
      )}
      {rows.map((row) => (
        <SourceRow
          key={row.key}
          row={row}
          repositories={repositories}
          discoveredRepositories={discoveredRepositories}
          workspaceId={workspaceId}
          repositoriesRefreshing={repositoriesRefreshing}
          onRefreshRepositories={onRefreshRepositories}
          capabilities={capabilities}
          error={errors[row.key]}
          onRemove={onRemove}
          onUpdate={onUpdate}
        />
      ))}
    </div>
  );
}

function RepositorySourceMenu({
  isMobile,
  onAdd,
}: {
  isMobile: boolean;
  onAdd: (kind: "saved_repository" | "local_repository" | "remote_repository") => void;
}) {
  const { t } = useTranslation();
  const itemClass = cn("cursor-pointer items-start gap-3", isMobile ? "min-h-11" : "py-2");
  return (
    <DropdownMenu modal={!isMobile}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className={cn("cursor-pointer", isMobile ? "min-h-11" : "h-9 px-3")}
        >
          <IconPlus className="h-4 w-4" />
          {t("common:addRepository")}
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80 max-w-[calc(100vw-2rem)]">
        <RepositorySourceMenuItem
          label="Workspace repository"
          description="Choose from saved or discovered repositories."
          icon={<IconStack2 className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("saved_repository")}
        />
        <RepositorySourceMenuItem
          label="Local Git repository"
          description="Use an existing checkout on this machine."
          icon={<IconGitBranch className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("local_repository")}
        />
        <RepositorySourceMenuItem
          label="Remote repository"
          description="Clone from a provider or Git URL."
          icon={<IconCloudDownload className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("remote_repository")}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function RepositorySourceMenuItem({
  label,
  description,
  icon,
  className,
  onSelect,
}: {
  label: string;
  description: string;
  icon: ReactNode;
  className: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem aria-label={label} className={className} onSelect={onSelect}>
      {icon}
      <span className="min-w-0">
        <span className="block text-sm font-medium text-foreground">{label}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </DropdownMenuItem>
  );
}
