"use client";

import Link from "@/components/routing/app-link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { IconAlertTriangle, IconStethoscope, IconCheck } from "@tabler/icons-react";

import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { bootstrapImproveKandev } from "@/lib/api/domains/improve-kandev-api";
import { listRepositories } from "@/lib/api/domains/workspace-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { fetchSystemHealth } from "@/lib/api/domains/health-api";
import type { Task } from "@/lib/types/http";
import { buildImproveKandevDescription } from "./improve-kandev-dialog-helpers";
import { CreateModeView, type BootstrapState } from "./improve-kandev-dialog-create";
import {
  IMPROVE_KANDEV_WORKSPACE_NAME,
  initialImproveKandevMode,
  getImproveKandevBrowserStorage,
  readImproveKandevSkipIntro,
  writeImproveKandevSkipIntro,
} from "./improve-kandev-dialog-model";

type ImproveKandevDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  onSuccess?: (task: Task) => void;
};

type Mode = "intro" | "create";

type AuthState =
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "missing"; message: string; fixUrl: string; fixLabel: string };

export function ImproveKandevDialog(props: ImproveKandevDialogProps) {
  const { open, onOpenChange, workspaceId, onSuccess } = props;
  const [mode, setMode] = useState<Mode>(() => initialImproveKandevMode(readSkipIntro()));
  const [skipIntro, setSkipIntro] = useState(() => readSkipIntro());
  const [auth, setAuth] = useState<AuthState>({ kind: "checking" });
  const [bootstrap, setBootstrap] = useState<BootstrapState>({ kind: "idle" });
  const [captureLogs, setCaptureLogs] = useState(true);
  const [createWorkspace, setCreateWorkspace] = useState(true);
  const [workspaceChoiceConfirmed, setWorkspaceChoiceConfirmed] = useState(false);
  const improveWorkspaceMissing = useAppStore(
    (s) =>
      !s.workspaces.items.some((workspace) => workspace.name === IMPROVE_KANDEV_WORKSPACE_NAME),
  );

  // When the dedicated workspace exists, bootstrap proceeds immediately.
  useEffect(() => {
    if (!improveWorkspaceMissing) {
      setWorkspaceChoiceConfirmed(true);
    }
  }, [improveWorkspaceMissing]);

  useResetOnClose(open, {
    setMode,
    setSkipIntro,
    setAuth,
    setBootstrap,
    setCaptureLogs,
    setCreateWorkspace,
    setWorkspaceChoiceConfirmed,
  });

  useGitHubAuthCheck(open, workspaceId, setAuth);
  useBootstrapKandev(
    open,
    mode,
    workspaceId,
    { workspaceChoiceConfirmed, createWorkspace },
    setBootstrap,
  );
  useEffect(() => {
    if (!open || mode !== "create" || auth.kind !== "missing") return;
    // A saved intro preference must not bypass the GitHub-auth recovery UI.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMode("intro");
  }, [auth.kind, mode, open]);

  if (mode === "create") {
    return (
      <ImproveKandevCreateStage
        open={open}
        onOpenChange={onOpenChange}
        workspaceId={workspaceId}
        bootstrap={bootstrap}
        captureLogs={captureLogs}
        setCaptureLogs={setCaptureLogs}
        onSuccess={onSuccess}
        externalBlockedReason={auth.kind === "checking" ? "Checking GitHub authentication…" : null}
        improveWorkspaceMissing={improveWorkspaceMissing}
        workspaceChoiceConfirmed={workspaceChoiceConfirmed}
        createWorkspace={createWorkspace}
        onCreateWorkspaceChange={setCreateWorkspace}
        onConfirmWorkspaceChoice={() => setWorkspaceChoiceConfirmed(true)}
        onCancel={() => onOpenChange(false)}
      />
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconStethoscope className="h-5 w-5" />
            Improve Kandev
          </DialogTitle>
        </DialogHeader>
        <IntroBody
          auth={auth}
          skipIntro={skipIntro}
          onSkipIntroChange={(checked) => {
            setSkipIntro(checked);
            writeImproveKandevSkipIntro(getImproveKandevBrowserStorage(), checked);
          }}
          improveWorkspaceMissing={improveWorkspaceMissing}
          createWorkspace={createWorkspace}
          onCreateWorkspaceChange={setCreateWorkspace}
          onCancel={() => onOpenChange(false)}
          onProceed={() => {
            setWorkspaceChoiceConfirmed(true);
            setMode("create");
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

function useGitHubAuthCheck(
  open: boolean,
  workspaceId: string | null,
  setAuth: (s: AuthState) => void,
) {
  useEffect(() => {
    if (!open || !workspaceId) return;
    let cancelled = false;
    (async () => {
      try {
        const health = await fetchSystemHealth();
        if (cancelled) return;
        const ghIssue = health.issues.find((i) => i.category === "github");
        if (!ghIssue) {
          setAuth({ kind: "ok" });
          return;
        }
        setAuth({
          kind: "missing",
          message: ghIssue.message,
          fixUrl: ghIssue.fix_url.replace("{workspaceId}", workspaceId),
          fixLabel: ghIssue.fix_label || "Configure GitHub",
        });
      } catch {
        if (!cancelled) setAuth({ kind: "ok" }); // Fail open — bootstrap will surface real errors.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, setAuth]);
}

function useBootstrapKandev(
  open: boolean,
  mode: Mode,
  workspaceId: string | null,
  gate: { workspaceChoiceConfirmed: boolean; createWorkspace: boolean },
  setBootstrap: (s: BootstrapState) => void,
) {
  const { toast } = useToast();
  const setRepositories = useAppStore((state) => state.setRepositories);
  const { workspaceChoiceConfirmed, createWorkspace } = gate;
  useEffect(() => {
    if (!open || mode !== "create" || !workspaceId || !workspaceChoiceConfirmed) return;
    let cancelled = false;
    setBootstrap({ kind: "loading" });
    (async () => {
      try {
        const data = await bootstrapImproveKandev(workspaceId, {
          createWorkspace,
        });
        // Refresh the dedicated workspace's repository list so the newly-created
        // kandev repo is in the store; otherwise the locked repo dropdown can't
        // resolve a label for the bootstrapped repository_id. All improve work
        // is scoped to the workspace the bootstrap returns, not the active one.
        const [stepsRes, issueStepsRes, reposRes] = await Promise.all([
          listWorkflowSteps(data.workflow_id),
          listWorkflowSteps(data.issue_workflow_id),
          listRepositories(data.workspace_id, undefined, { cache: "no-store" }),
        ]);
        if (cancelled) return;
        setRepositories(data.workspace_id, reposRes.repositories);
        setBootstrap({
          kind: "ready",
          data,
          steps: stepsRes.steps,
          issueSteps: issueStepsRes.steps,
        });
      } catch (err) {
        if (cancelled) return;
        const message = err instanceof Error ? err.message : "Bootstrap failed";
        setBootstrap({ kind: "error", message });
        toast({
          title: "Could not prepare Improve Kandev",
          description: message,
          variant: "error",
        });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [
    open,
    mode,
    workspaceId,
    workspaceChoiceConfirmed,
    createWorkspace,
    setBootstrap,
    setRepositories,
    toast,
  ]);
}

function IntroBody({
  auth,
  skipIntro,
  onSkipIntroChange,
  improveWorkspaceMissing,
  createWorkspace,
  onCreateWorkspaceChange,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  skipIntro: boolean;
  onSkipIntroChange: (checked: boolean) => void;
  improveWorkspaceMissing: boolean;
  createWorkspace: boolean;
  onCreateWorkspaceChange: (checked: boolean) => void;
  onCancel: () => void;
  onProceed: () => void;
}) {
  if (auth.kind === "missing") {
    return <GhAuthMissing auth={auth} onCancel={onCancel} />;
  }
  return (
    <IntroExplanation
      auth={auth}
      skipIntro={skipIntro}
      onSkipIntroChange={onSkipIntroChange}
      improveWorkspaceMissing={improveWorkspaceMissing}
      createWorkspace={createWorkspace}
      onCreateWorkspaceChange={onCreateWorkspaceChange}
      onCancel={onCancel}
      onProceed={onProceed}
    />
  );
}

function GhAuthMissing({
  auth,
  onCancel,
}: {
  auth: Extract<AuthState, { kind: "missing" }>;
  onCancel: () => void;
}) {
  return (
    <div className="space-y-4 py-2">
      <div className="flex items-start gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
        <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-500" />
        <div>
          <p className="font-medium text-foreground">GitHub CLI not authenticated</p>
          <p className="mt-1 text-muted-foreground">
            The final step of this workflow opens a pull request, which needs the <code>gh</code>{" "}
            CLI to be authenticated. {auth.message}
          </p>
        </div>
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          Cancel
        </Button>
        <Button asChild className="cursor-pointer">
          <Link href={auth.fixUrl} onClick={onCancel}>
            {auth.fixLabel}
          </Link>
        </Button>
      </div>
    </div>
  );
}

function IntroExplanation({
  auth,
  skipIntro,
  onSkipIntroChange,
  improveWorkspaceMissing,
  createWorkspace,
  onCreateWorkspaceChange,
  onCancel,
  onProceed,
}: {
  auth: AuthState;
  skipIntro: boolean;
  onSkipIntroChange: (checked: boolean) => void;
  improveWorkspaceMissing: boolean;
  createWorkspace: boolean;
  onCreateWorkspaceChange: (checked: boolean) => void;
  onCancel: () => void;
  onProceed: () => void;
}) {
  return (
    <div className="space-y-5 py-2">
      <p className="text-sm leading-relaxed text-muted-foreground">
        Kandev is open source, and you can help make it better.
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        Describe a bug you hit or a feature you&apos;d like, and we&apos;ll create a task on your
        own agent to implement it in the kandev codebase.
      </p>

      <p className="text-sm leading-relaxed text-muted-foreground">
        When it&apos;s done, the agent opens a pull request to{" "}
        <code className="font-mono text-xs">kdlbs/kandev</code> for the maintainers to review,
        saving them time and shipping the improvement to everyone.
      </p>

      <ul className="space-y-2 text-sm text-muted-foreground">
        <IntroBullet>Create a task describing your bug or feature request</IntroBullet>
        <IntroBullet>Your agent implements it in the kandev repository, with tests</IntroBullet>
        <IntroBullet>You verify and test the change in a second kandev instance</IntroBullet>
        <IntroBullet>
          The agent forks <code className="font-mono text-xs">kdlbs/kandev</code> to your GitHub
          account and opens a PR from your fork, credited to you
        </IntroBullet>
      </ul>
      {improveWorkspaceMissing && (
        <CreateWorkspaceCheckbox
          checked={createWorkspace}
          onCheckedChange={onCreateWorkspaceChange}
        />
      )}
      <label
        className="flex min-h-12 cursor-pointer items-center gap-2 text-sm text-muted-foreground"
        data-testid="improve-kandev-skip-intro"
      >
        <Checkbox
          checked={skipIntro}
          onCheckedChange={(checked) => onSkipIntroChange(checked === true)}
        />
        Do not show this again
      </label>
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
          Cancel
        </Button>
        <Button
          onClick={onProceed}
          disabled={auth.kind === "checking"}
          className="cursor-pointer"
          data-testid="improve-kandev-proceed"
        >
          Contribute
        </Button>
      </div>
    </div>
  );
}

// CreateWorkspaceCheckbox is the opt-in for creating the dedicated Improve
// Kandev workspace. Shown whenever the workspace does not exist yet: in the
// intro, and as a gate before the create form for users who skipped the intro.
function CreateWorkspaceCheckbox({
  checked,
  onCheckedChange,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <label
      className="flex min-h-12 cursor-pointer items-start gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm text-muted-foreground"
      data-testid="improve-kandev-create-workspace"
    >
      <Checkbox
        className="mt-0.5"
        checked={checked}
        onCheckedChange={(next) => onCheckedChange(next === true)}
      />
      <span>
        Create a dedicated <span className="font-medium text-foreground">Improve Kandev</span>{" "}
        workspace to keep improve tasks isolated from your regular work
      </span>
    </label>
  );
}

// WorkspaceChoicePanel gates the create form when the dedicated workspace does
// not exist yet and the user skipped the intro: they choose whether to create
// it before bootstrap runs. Once confirmed, the task lands in the dedicated
// workspace (checked) or in the active workspace (unchecked, legacy).
function WorkspaceChoicePanel({
  open,
  onOpenChange,
  createWorkspace,
  onCreateWorkspaceChange,
  onCancel,
  onContinue,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createWorkspace: boolean;
  onCreateWorkspaceChange: (checked: boolean) => void;
  onCancel: () => void;
  onContinue: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconStethoscope className="h-5 w-5" />
            Improve Kandev
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-5 py-2">
          <p className="text-sm leading-relaxed text-muted-foreground">
            The dedicated <span className="font-medium text-foreground">Improve Kandev</span>{" "}
            workspace does not exist yet. Improve tasks are kept isolated from your regular work in
            their own workspace.
          </p>
          <CreateWorkspaceCheckbox
            checked={createWorkspace}
            onCheckedChange={onCreateWorkspaceChange}
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={onCancel} className="cursor-pointer">
              Cancel
            </Button>
            <Button
              onClick={onContinue}
              className="cursor-pointer"
              data-testid="improve-kandev-create-workspace-confirm"
            >
              Continue
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function IntroBullet({ children }: { children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-2">
      <IconCheck className="h-4 w-4 mt-0.5 shrink-0 text-emerald-500" />
      <span>{children}</span>
    </li>
  );
}

function readSkipIntro(): boolean {
  try {
    return readImproveKandevSkipIntro(globalThis.localStorage);
  } catch {
    return false;
  }
}

// useResetOnClose restores the dialog to a fresh state whenever the parent
// closes it, so a re-open re-runs the auth check and workspace-choice flow.
// The setters are read through a ref so the effect only runs on `open` changes
// (the setters object literal would otherwise re-trigger it every render).
function useResetOnClose(
  open: boolean,
  setters: {
    setMode: (mode: Mode) => void;
    setSkipIntro: (skip: boolean) => void;
    setAuth: (auth: AuthState) => void;
    setBootstrap: (bootstrap: BootstrapState) => void;
    setCaptureLogs: (capture: boolean) => void;
    setCreateWorkspace: (create: boolean) => void;
    setWorkspaceChoiceConfirmed: (confirmed: boolean) => void;
  },
) {
  const settersRef = useRef(setters);
  settersRef.current = setters;
  useEffect(() => {
    if (open) return;
    const s = settersRef.current;
    /* eslint-disable react-hooks/set-state-in-effect */
    s.setMode(initialImproveKandevMode(readSkipIntro()));
    s.setSkipIntro(readSkipIntro());
    s.setAuth({ kind: "checking" });
    s.setBootstrap({ kind: "idle" });
    s.setCaptureLogs(true);
    s.setCreateWorkspace(true);
    s.setWorkspaceChoiceConfirmed(false);
    /* eslint-enable react-hooks/set-state-in-effect */
  }, [open]);
}

// ImproveKandevCreateStage renders the create stage: the workspace-choice gate
// when the dedicated workspace does not exist yet, otherwise the create form.
function ImproveKandevCreateStage({
  open,
  onOpenChange,
  workspaceId,
  bootstrap,
  captureLogs,
  setCaptureLogs,
  onSuccess,
  externalBlockedReason,
  improveWorkspaceMissing,
  workspaceChoiceConfirmed,
  createWorkspace,
  onCreateWorkspaceChange,
  onConfirmWorkspaceChoice,
  onCancel,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  bootstrap: BootstrapState;
  captureLogs: boolean;
  setCaptureLogs: (capture: boolean) => void;
  onSuccess?: (task: Task) => void;
  externalBlockedReason: string | null;
  improveWorkspaceMissing: boolean;
  workspaceChoiceConfirmed: boolean;
  createWorkspace: boolean;
  onCreateWorkspaceChange: (checked: boolean) => void;
  onConfirmWorkspaceChoice: () => void;
  onCancel: () => void;
}) {
  const handleSuccess = useCallback(
    (task: Task) => {
      onOpenChange(false);
      onSuccess?.(task);
    },
    [onOpenChange, onSuccess],
  );

  const transformDescription = useCallback(
    async (description: string) => {
      if (bootstrap.kind !== "ready") return description;
      return buildImproveKandevDescription(description, bootstrap.data, captureLogs);
    },
    [bootstrap, captureLogs],
  );

  if (improveWorkspaceMissing && !workspaceChoiceConfirmed) {
    return (
      <WorkspaceChoicePanel
        open={open}
        onOpenChange={onOpenChange}
        createWorkspace={createWorkspace}
        onCreateWorkspaceChange={onCreateWorkspaceChange}
        onCancel={onCancel}
        onContinue={onConfirmWorkspaceChoice}
      />
    );
  }
  return (
    <CreateModeView
      open={open}
      onOpenChange={onOpenChange}
      workspaceId={workspaceId}
      bootstrap={bootstrap}
      captureLogs={captureLogs}
      setCaptureLogs={setCaptureLogs}
      transformDescription={transformDescription}
      onTaskCreated={handleSuccess}
      externalBlockedReason={externalBlockedReason}
    />
  );
}
