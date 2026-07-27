"use client";

import { useState } from "react";
import { Trans, useLingui } from "@lingui/react/macro";
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

type TUIAgentFormData = { display_name: string; model?: string; command: string };

type AddTUIAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: TUIAgentFormData) => Promise<void>;
};

type DialogHandlersParams = {
  displayName: string;
  model: string;
  command: string;
  setError: React.Dispatch<React.SetStateAction<string | null>>;
  setLoading: React.Dispatch<React.SetStateAction<boolean>>;
  onSubmit: (data: TUIAgentFormData) => Promise<void>;
  onOpenChange: (open: boolean) => void;
  reset: () => void;
};

function useDialogHandlers({
  displayName,
  model,
  command,
  setError,
  setLoading,
  onSubmit,
  onOpenChange,
  reset,
}: DialogHandlersParams) {
  const { t } = useLingui();
  const handleSubmit = async () => {
    if (!displayName.trim()) {
      setError(t`Display name is required`);
      return;
    }
    if (!command.trim()) {
      setError(t`Command is required`);
      return;
    }
    setError(null);
    setLoading(true);
    try {
      await onSubmit({
        display_name: displayName.trim(),
        model: model.trim() || undefined,
        command: command.trim(),
      });
      reset();
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : t`Failed to create agent`);
    } finally {
      setLoading(false);
    }
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  return { handleSubmit, handleOpenChange };
}

export function AddTUIAgentDialog({ open, onOpenChange, onSubmit }: AddTUIAgentDialogProps) {
  const { t } = useLingui();
  const [displayName, setDisplayName] = useState("");
  const [model, setModel] = useState("");
  const [command, setCommand] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setDisplayName("");
    setModel("");
    setCommand("");
    setError(null);
    setLoading(false);
  };

  const { handleSubmit, handleOpenChange } = useDialogHandlers({
    displayName,
    model,
    command,
    setError,
    setLoading,
    onSubmit,
    onOpenChange,
    reset,
  });

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            <Trans>Add TUI Agent</Trans>
          </DialogTitle>
          <DialogDescription>
            <Trans>
              Register a CLI tool as a TUI agent. It will appear in the agent list with a default
              passthrough profile.
            </Trans>
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="tui-display-name">
              <Trans>Display Name</Trans>
            </Label>
            <Input
              id="tui-display-name"
              placeholder={t`e.g. superclaude`}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="tui-model">
              <Trans>Model</Trans>
            </Label>
            <Input
              id="tui-model"
              placeholder={t`e.g. best`}
              value={model}
              onChange={(e) => setModel(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              <Trans>Profile label shown in the agent selector dropdown</Trans>
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="tui-command">
              <Trans>Command</Trans>
            </Label>
            <Input
              id="tui-command"
              placeholder="e.g. superclaude --yolo --model {{model}}"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              <Trans>
                Binary name looked up on PATH. Use{" "}
                <code className="rounded bg-muted px-1 py-0.5">{"{{model}}"}</code> to insert the
                model value.
              </Trans>
            </p>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            <Trans>Cancel</Trans>
          </Button>
          <Button onClick={handleSubmit} disabled={loading} className="cursor-pointer">
            {loading ? t`Creating...` : t`Create`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
