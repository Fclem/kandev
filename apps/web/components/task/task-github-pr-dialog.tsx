"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useToast } from "@/components/toast-provider";
import { createTaskPR } from "@/lib/api/domains/github-api";
import type { Repository } from "@/lib/types/http";
import {
  githubReposForTask,
  pullRequestPayload,
  type TaskPullRequestLinkTarget,
} from "./task-github-pr-url";
import { useTranslation } from "react-i18next";

type TaskGitHubPRDialogProps = {
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  task: TaskPullRequestLinkTarget;
  repositories: Repository[];
};

/** Owns the link-PR field state, reset-on-open, and the create-PR submit flow. */
function useLinkPullRequestForm({
  open,
  workspaceId,
  taskId,
  githubRepos,
  onLinked,
}: {
  open: boolean;
  workspaceId: string | null;
  taskId: string;
  githubRepos: ReturnType<typeof githubReposForTask>;
  onLinked: () => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setInput("");
      setError(null);
    }
  }, [open]);

  const submit = async () => {
    if (!workspaceId) {
      setError("Select a workspace before linking a GitHub pull request.");
      return;
    }
    if (!input.trim()) {
      setError("Enter a GitHub pull request URL or number.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const payload = pullRequestPayload(input, githubRepos);
      await createTaskPR({
        workspace_id: workspaceId,
        task_id: taskId,
        pr_url: payload.pr_url,
        ...(payload.repository_id ? { repository_id: payload.repository_id } : {}),
      });
      toast({ description: t("task:githubPullRequestLinked"), variant: "success" });
      onLinked();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to link GitHub pull request.");
    } finally {
      setSubmitting(false);
    }
  };

  return { input, setInput, submitting, error, submit };
}

export function TaskGitHubPRDialog({
  workspaceId,
  open,
  onOpenChange,
  task,
  repositories,
}: TaskGitHubPRDialogProps) {
  const { t } = useTranslation();
  const githubRepos = useMemo(() => githubReposForTask(task, repositories), [task, repositories]);
  const inferredRepo = githubRepos.length === 1 ? githubRepos[0] : null;
  const placeholder = inferredRepo
    ? "#1471 or github.com/owner/repo/pull/1471"
    : "github.com/owner/repo/pull/1471";
  const { input, setInput, submitting, error, submit } = useLinkPullRequestForm({
    open,
    workspaceId,
    taskId: task.id,
    githubRepos,
    onLinked: () => onOpenChange(false),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("task:linkGithubPullRequest")}</DialogTitle>
          <DialogDescription>
            {inferredRepo
              ? `Use a full pull request URL or number for ${inferredRepo.owner}/${inferredRepo.repo}.`
              : t("task:useAFullGithubPullRequest")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="task-github-pr-input">{t("task:pullRequest")}</Label>
          <Input
            id="task-github-pr-input"
            data-testid="task-github-pr-input"
            value={input}
            onChange={(event) => setInput(event.target.value)}
            placeholder={placeholder}
            disabled={submitting}
          />
          {error && (
            <p className="text-xs text-destructive" data-testid="task-github-pr-error">
              {error}
            </p>
          )}
        </div>
        <DialogFooter className="gap-2">
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            className="cursor-pointer"
            onClick={submit}
            disabled={submitting}
            data-testid="task-github-pr-submit"
          >
            {submitting ? t("task:saving2") : t("task:save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
