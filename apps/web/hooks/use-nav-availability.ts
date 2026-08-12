"use client";

import { useAppStore } from "@/components/state-provider";
import { useAzureDevOpsAvailable } from "@/hooks/domains/azure-devops/use-azure-devops-availability";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabAvailable } from "@/hooks/domains/gitlab/use-task-mr";
import { useJiraAuthed } from "@/hooks/domains/jira/use-jira-availability";
import { useLinearAuthed } from "@/hooks/domains/linear/use-linear-availability";
import type { AvailabilityMap } from "@/lib/navigation/types";
import type { GitHubStatus } from "@/lib/types/github";

/** Human-readable status label for a not-yet-connected integration destination. */
function getStatusLabel(loading: boolean | undefined): string {
  return loading ? "Checking" : "Setup";
}

/** Derives the GitHub nav destination's ready/label pair from its auth status. */
export function getGitHubIntegrationStatus(status: GitHubStatus | null, loading: boolean) {
  if (status?.authenticated) return { ready: true, label: "Connected" };
  if (status?.token_configured) return { ready: true, label: "Configured" };
  return { ready: false, label: getStatusLabel(loading) };
}

/**
 * Availability map the navigation manifest filters on
 * (`lib/navigation/`) — currently which integrations are
 * configured. Extend here when a destination gains a gate.
 *
 * Extracted from `useConfiguredIntegrationLinks` so the sidebar link list and the
 * mobile menu gate on one implementation instead of each calling the five domain
 * hooks themselves.
 *
 * A destination is visible when it is **configured** (credentials saved,
 * auth healthy); the per-integration "enabled" toggle does not affect nav
 * visibility. That toggle still gates everything it gates elsewhere (import
 * popovers, Kanban external-link buttons, task-top-bar buttons) via each
 * integration's `useXAvailable`, unaffected by this hook. The "Hide disabled
 * integrations from left panel navigation" setting and its enabled filter were
 * removed — see `docs/specs/ui/remove-hide-disabled-nav-options.md`.
 *
 * Availability is **not** deduplicated across consumers. GitHub and GitLab are
 * store-backed, but Jira, Linear and Azure DevOps go through
 * `useIntegrationAuthed`, which owns a fetch plus a 90s `setInterval` per mounted
 * consumer (`hooks/domains/integrations/use-integration-availability.ts`). So each
 * mounted caller of this hook may add its own polling subscription: keep the
 * callers to the surfaces that render gated sections, and use
 * `useStaticDestinations` everywhere else. Deduplicating this properly means
 * hoisting the probes into the store, which is a change to those domain hooks
 * rather than to the navigation manifest.
 *
 * Not to be confused with `hooks/domains/integrations/use-integration-availability`,
 * which is the generic per-integration primitive these hooks are built on.
 */
export function useNavAvailability(): AvailabilityMap {
  // Jira and Linear are per-workspace integrations, so their availability must
  // be checked against the active workspace. Omitting the id makes the backend
  // fall back to a legacy default-workspace resolver that can point at the
  // wrong workspace, hiding a configured integration from the sidebar. GitHub,
  // Jira, and Linear all follow the active workspace; GitLab remains install-wide.
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const activeWorkspaceExists = useAppStore((s) =>
    s.workspaces.items.some((item) => item.id === s.workspaces.activeId),
  );
  // Guard against a stale active id: if the active workspace was removed but
  // activeId was not reconciled (e.g. setWorkspaces keeps a non-null id),
  // scoping to the deleted id would return no config and hide the links even
  // when another workspace is configured. Fall back to null so the backend's
  // default-workspace resolution applies instead.
  const scopedWorkspaceId = activeWorkspaceExists ? activeWorkspaceId : null;
  const { status, loading } = useGitHubStatus();
  const gitlabConfigured = useGitLabAvailable();
  const jiraConfigured = useJiraAuthed(scopedWorkspaceId);
  const linearConfigured = useLinearAuthed(scopedWorkspaceId);
  const azureDevOpsConfigured = useAzureDevOpsAvailable(scopedWorkspaceId);
  const githubConfigured = getGitHubIntegrationStatus(status, loading).ready;

  return {
    "azure-devops": !!azureDevOpsConfigured,
    github: githubConfigured,
    gitlab: gitlabConfigured,
    jira: jiraConfigured,
    linear: linearConfigured,
  };
}
