"use client";

import { useState } from "react";
import { t } from "@lingui/core/macro";
import { Trans } from "@lingui/react/macro";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { createInvite } from "@/lib/api/domains/auth-api";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
};

function InviteForm({
  email,
  setEmail,
  role,
  setRole,
  error,
  submitting,
  onCancel,
  onSubmit,
}: {
  email: string;
  setEmail: (v: string) => void;
  role: string;
  setRole: (v: string) => void;
  error: string | null;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>
          <Trans>Invite a user</Trans>
        </DialogTitle>
        <DialogDescription>
          <Trans>
            Generates a one-time invite link. Leave the email blank to create a link anyone can use
            to sign up with the selected role; set an email to pin the invite to that address.
          </Trans>
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        <div className="flex flex-col gap-1">
          <label htmlFor="invite-dialog-email" className="text-xs text-muted-foreground">
            <Trans>Email (optional)</Trans>
          </label>
          <Input
            id="invite-dialog-email"
            data-testid="invite-dialog-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="invite-dialog-role" className="text-xs text-muted-foreground">
            <Trans>Role</Trans>
          </label>
          <Select value={role} onValueChange={setRole}>
            <SelectTrigger id="invite-dialog-role" data-testid="invite-dialog-role">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="member">
                <Trans>Member</Trans>
              </SelectItem>
              <SelectItem value="admin">
                <Trans>Admin</Trans>
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        {error && (
          <p className="text-xs text-destructive" data-testid="invite-dialog-error">
            {error}
          </p>
        )}
      </div>
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onCancel}>
          <Trans>Cancel</Trans>
        </Button>
        <Button
          className="cursor-pointer"
          disabled={submitting}
          onClick={onSubmit}
          data-testid="invite-dialog-submit"
        >
          {submitting ? <Trans>Creating...</Trans> : <Trans>Create invite link</Trans>}
        </Button>
      </DialogFooter>
    </>
  );
}

function InviteLinkResult({ url, onDone }: { url: string; onDone: () => void }) {
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    await navigator.clipboard.writeText(url);
    setCopied(true);
  };
  return (
    <>
      <DialogHeader>
        <DialogTitle>
          <Trans>Invite link ready</Trans>
        </DialogTitle>
        <DialogDescription>
          <Trans>Share this link with the invitee. It is shown only once — copy it now.</Trans>
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        <Input readOnly value={url} data-testid="invite-dialog-url" className="font-mono text-xs" />
        <Button
          size="icon"
          variant="outline"
          className="cursor-pointer"
          onClick={() => void onCopy()}
          data-testid="invite-dialog-copy"
        >
          {copied ? <IconCheck className="h-4 w-4" /> : <IconCopy className="h-4 w-4" />}
        </Button>
      </div>
      <DialogFooter>
        <Button className="cursor-pointer" onClick={onDone} data-dialog-default-action>
          <Trans>Done</Trans>
        </Button>
      </DialogFooter>
    </>
  );
}

export function InviteDialog({ open, onOpenChange, onCreated }: Props) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resultUrl, setResultUrl] = useState<string | null>(null);

  const reset = () => {
    setEmail("");
    setRole("member");
    setError(null);
    setResultUrl(null);
  };

  const onSubmit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const { url } = await createInvite({ email: email || undefined, role });
      setResultUrl(`${window.location.origin}${url}`);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t`Could not create invite.`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent data-testid="invite-dialog">
        {resultUrl ? (
          <InviteLinkResult url={resultUrl} onDone={() => onOpenChange(false)} />
        ) : (
          <InviteForm
            email={email}
            setEmail={setEmail}
            role={role}
            setRole={setRole}
            error={error}
            submitting={submitting}
            onCancel={() => onOpenChange(false)}
            onSubmit={() => void onSubmit()}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
