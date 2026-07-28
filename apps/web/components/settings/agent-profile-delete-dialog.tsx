"use client";
import { Trans, useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { useAppStore } from "@/components/state-provider";
import type {
  ActiveSessionInfo,
  RoutingTierReference,
  WatcherReference,
} from "@/lib/types/agent-profile-errors";

const WATCHER_KIND_LABELS: Record<WatcherReference["kind"], string> = {
  linear: "Linear",
  jira: "Jira",
  github_issue: "GitHub Issues",
  github_review: "GitHub PR Reviews",
};

type AgentProfileDeleteConfirmDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConfirmDialogProps) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("settings:deleteAgentProfile")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("settings:thisWillPermanentlyDeleteThisProfile")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {t("common:delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// AgentProfileDeleteConflict carries the structured 409 payload from the
// backend. `open` is separate from the lists so a watcher-only conflict
// (no active sessions) still pops the dialog.
export type AgentProfileDeleteConflict = {
  activeSessions: ActiveSessionInfo[];
  watchers: WatcherReference[];
  routingTiers: RoutingTierReference[];
};

type AgentProfileDeleteConflictDialogProps = {
  conflict: AgentProfileDeleteConflict | null;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function AgentProfileDeleteConflictDialog({
  conflict,
  onOpenChange,
  onConfirm,
}: AgentProfileDeleteConflictDialogProps) {
  const { t } = useTranslation();
  const tasks = conflict?.activeSessions.filter((s) => !s.is_ephemeral) ?? [];
  const quickChats = conflict?.activeSessions.filter((s) => s.is_ephemeral) ?? [];
  const watchers = conflict?.watchers ?? [];
  const routingTiers = conflict?.routingTiers ?? [];
  const hasHardBlockers = routingTiers.length > 0;
  const watchersByKind = groupWatchersByKind(watchers);
  const workspaces = useAppStore((s) => s.workspaces.items);
  const providers = useAppStore((s) => s.settingsAgents.items);

  return (
    <AlertDialog open={!!conflict} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {hasHardBlockers
              ? t("settings:cannotDeleteAgentProfile")
              : t("settings:deleteAgentProfile")}
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div>
              <p>{t("settings:thisProfileIsCurrentlyInUse")}</p>
              <SessionConflictSection
                title={t("settings:tasks")}
                sessions={tasks}
                fallback={t("settings:untitledTask")}
              />
              <SessionConflictSection
                title={t("settings:quickChats")}
                sessions={quickChats}
                fallback={t("settings:untitledQuickChat")}
              />
              <WatcherConflictSection watchersByKind={watchersByKind} />
              <RoutingTierConflictSection
                routingTiers={routingTiers}
                workspaceLabels={new Map(workspaces.map((w) => [w.id, w.name]))}
                providerLabels={new Map(providers.map((p) => [p.id, p.name]))}
              />
              {hasHardBlockers ? (
                <p className="mt-2">{t("settings:changeTheseWorkspaceTierMappingsBefore")}</p>
              ) : (
                <p className="mt-2">{t("settings:theseSessionsWillNoLongerBe")}</p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          {hasHardBlockers ? null : (
            <AlertDialogAction
              onClick={onConfirm}
              className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("settings:deleteAnyway")}
            </AlertDialogAction>
          )}
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SessionConflictSection({
  title,
  sessions,
  fallback,
}: {
  title: string;
  sessions: ActiveSessionInfo[];
  fallback: string;
}) {
  if (sessions.length === 0) return null;
  return (
    <div className="mt-2">
      <p className="font-medium text-sm">{title}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
        {sessions.map((t) => (
          <li key={t.task_id} className="text-sm">
            {t.task_title || fallback}
          </li>
        ))}
      </ul>
    </div>
  );
}

function WatcherConflictSection({
  watchersByKind,
}: {
  watchersByKind: Record<string, WatcherReference[]>;
}) {
  const { t } = useTranslation();
  const entries = Object.entries(watchersByKind);
  if (entries.length === 0) return null;
  return (
    <div className="mt-2">
      <p className="font-medium text-sm">{t("settings:watchersWillBeDisabled")}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
        {entries.map(([kind, items]) => (
          <li key={kind} className="text-sm">
            <span className="font-medium">
              {WATCHER_KIND_LABELS[kind as WatcherReference["kind"]] ?? kind}:
            </span>{" "}
            {items.map((w) => w.label || w.id).join(", ")}
          </li>
        ))}
      </ul>
    </div>
  );
}

function RoutingTierConflictSection({
  routingTiers,
  workspaceLabels,
  providerLabels,
}: {
  routingTiers: RoutingTierReference[];
  workspaceLabels: Map<string, string>;
  providerLabels: Map<string, string>;
}) {
  const { t } = useTranslation();
  if (routingTiers.length === 0) return null;
  return (
    <div className="mt-2">
      <p className="font-medium text-sm">{t("settings:workspaceTierMappings")}</p>
      <ul className="list-disc list-inside mt-1 space-y-0.5">
        {routingTiers.map((ref) => {
          const tierLabel = formatRoutingTier(ref.tier);
          const workspaceLabel = formatLookupLabel(workspaceLabels, ref.workspace_id);
          const providerLabel = formatLookupLabel(providerLabels, ref.provider_id);
          return (
            <li key={`${ref.workspace_id}-${ref.provider_id}-${ref.tier}`} className="text-sm">
              <Trans
                i18nKey="settings:tierInFor"
                values={{ providerLabel, tierLabel, workspaceLabel }}
              >
                <span className="font-medium">{tierLabel}</span> tier in {workspaceLabel} for{" "}
                {providerLabel}
              </Trans>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function formatRoutingTier(tier: string): string {
  return tier.charAt(0).toUpperCase() + tier.slice(1);
}

function formatLookupLabel(labels: Map<string, string>, id: string): string {
  const label = labels.get(id);
  return label && label !== id ? `${label} (${id})` : id;
}

function groupWatchersByKind(watchers: WatcherReference[]): Record<string, WatcherReference[]> {
  return watchers.reduce<Record<string, WatcherReference[]>>((acc, w) => {
    (acc[w.kind] ??= []).push(w);
    return acc;
  }, {});
}
