"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { copySecret, moveSecret } from "@/lib/api/domains/secrets-api";
import { useSecretDestinationNames } from "@/hooks/domains/settings/use-secret-destination-names";
import { useWorkspaceDestinations } from "@/hooks/domains/settings/use-workspace-destinations";
import type { SecretListItem } from "@/lib/types/http-secrets";

export const MAX_SECRET_NAME_BYTES = 100;

export type CopyMoveMode = "copy" | "move";

export type SecretDestination = { scope: "global" } | { scope: "workspace"; workspaceId: string };

/**
 * Truncates a string so its UTF-8 byte length is at most `limit`, iterating
 * code points (never splitting surrogate pairs or producing replacement
 * characters). The backend name limit is 100 UTF-8 bytes.
 */
export function truncateUtf8Bytes(value: string, limit: number): string {
  const encoder = new TextEncoder();
  if (encoder.encode(value).length <= limit) {
    return value;
  }
  let result = "";
  for (const char of value) {
    const next = result + char;
    if (encoder.encode(next).length > limit) {
      break;
    }
    result = next;
  }
  return result;
}

/** Default target name: `<name> (from <origin>)`, truncated to the byte limit. */
export function buildDefaultTargetName(name: string, originToken: string): string {
  return truncateUtf8Bytes(`${name} (from ${originToken})`, MAX_SECRET_NAME_BYTES);
}

type CopyMoveSecretDialogProps = {
  secret: SecretListItem;
  /** `general` for a Global source, the workspace name otherwise (literal, locale-independent). */
  originToken: string;
  onClose: () => void;
  onCompleted: (item: SecretListItem, mode: CopyMoveMode) => void;
};

export function CopyMoveSecretDialog({
  secret,
  originToken,
  onClose,
  onCompleted,
}: CopyMoveSecretDialogProps) {
  const { t } = useTranslation();
  const {
    loading: destinationsLoading,
    error: destinationsError,
    retry: retryDestinations,
  } = useWorkspaceDestinations();
  const workspaces = useAppStore((state) => state.workspaces.items);
  const [mode, setMode] = useState<CopyMoveMode>("copy");
  const [destination, setDestination] = useState<SecretDestination | null>(null);
  const [name, setName] = useState(() => buildDefaultTargetName(secret.name, originToken));
  const [nameError, setNameError] = useState<string | null>(null);

  const { destinations, workspaceNameById, destinationValid } = useDestinationOptions(
    secret,
    workspaces,
    destination,
    setDestination,
  );
  const destinationNames = useSecretDestinationNames(
    destination?.scope ?? "global",
    destination?.scope === "workspace" ? destination.workspaceId : undefined,
  );
  const conflict = nameError === null && destination !== null && destinationNames.conflict(name);

  const { busy, canSubmit, formError, run } = useTransferSubmit({
    secret,
    mode,
    destination,
    destinationValid,
    name,
    nameError,
    conflict,
    destinationsLoading,
    destinationsError: destinationsError !== null,
    setNameError,
    onCompleted,
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        showCloseButton={false}
        className="bottom-0 top-auto left-0 right-0 translate-x-0 translate-y-0 rounded-b-none pb-[env(safe-area-inset-bottom)] sm:top-1/2 sm:left-1/2 sm:bottom-auto sm:translate-x-[-50%] sm:translate-y-[-50%] sm:rounded-b-lg"
      >
        <DialogHeader>
          <DialogTitle>{t("settings:copyMoveSecretNamed", { name: secret.name })}</DialogTitle>
          <DialogDescription>{t("settings:copyMoveDialogHint")}</DialogDescription>
        </DialogHeader>

        <TransferModeField mode={mode} onModeChange={setMode} originToken={originToken} />

        <DestinationField
          destinations={destinations}
          workspaceNameById={workspaceNameById}
          destination={destination}
          onDestinationChange={(next) => {
            // A 409 conflict is destination-specific: switching targets must
            // clear it so the name field is not stuck invalid on a destination
            // where the name is free.
            if (nameError !== null) {
              setNameError(null);
            }
            setDestination(next);
          }}
          destinationsLoading={destinationsLoading}
          destinationsError={destinationsError}
          onRetryDestinations={retryDestinations}
          disabled={busy}
        />

        <TargetNameField
          name={name}
          onNameChange={setName}
          nameError={nameError}
          setNameError={setNameError}
          conflict={conflict}
          disabled={busy}
        />

        {formError !== null && (
          <p className="text-xs text-destructive" role="alert">
            {formError}
          </p>
        )}

        <TransferDialogFooter
          mode={mode}
          canSubmit={canSubmit}
          busy={busy}
          onSubmit={run}
          onClose={onClose}
        />
      </DialogContent>
    </Dialog>
  );
}

function TransferDialogFooter({
  mode,
  canSubmit,
  busy,
  onSubmit,
  onClose,
}: {
  mode: CopyMoveMode;
  canSubmit: boolean;
  busy: boolean;
  onSubmit: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter>
      <Button
        type="button"
        variant="outline"
        onClick={onClose}
        disabled={busy}
        className="min-h-11 cursor-pointer"
      >
        {t("settings:cancel")}
      </Button>
      <Button
        type="button"
        onClick={onSubmit}
        disabled={!canSubmit}
        className="min-h-11 cursor-pointer"
      >
        {mode === "copy" ? t("settings:copySecretAction") : t("settings:moveSecretAction")}
      </Button>
      <DialogClose asChild>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("common:close")}
          className="absolute top-2 right-2 min-h-11 min-w-11 cursor-pointer"
        >
          <IconX className="h-4 w-4" />
        </Button>
      </DialogClose>
    </DialogFooter>
  );
}

type TransferCanSubmitArgs = {
  isBusy: boolean;
  destination: SecretDestination | null;
  destinationValid: boolean;
  name: string;
  nameError: string | null;
  conflict: boolean;
  destinationsLoading: boolean;
  destinationsError: boolean;
};

function transferCanSubmit({
  isBusy,
  destination,
  destinationValid,
  name,
  nameError,
  conflict,
  destinationsLoading,
  destinationsError,
}: TransferCanSubmitArgs): boolean {
  return (
    !isBusy &&
    destination !== null &&
    destinationValid &&
    name.trim().length > 0 &&
    nameError === null &&
    !conflict &&
    !destinationsLoading &&
    !destinationsError
  );
}

type TransferSubmitArgs = {
  secret: SecretListItem;
  mode: CopyMoveMode;
  destination: SecretDestination | null;
  destinationValid: boolean;
  name: string;
  nameError: string | null;
  conflict: boolean;
  destinationsLoading: boolean;
  destinationsError: boolean;
  setNameError: (error: string | null) => void;
  onCompleted: (item: SecretListItem, mode: CopyMoveMode) => void;
};

function useTransferSubmit({
  secret,
  mode,
  destination,
  destinationValid,
  name,
  nameError,
  conflict,
  destinationsLoading,
  destinationsError,
  setNameError,
  onCompleted,
}: TransferSubmitArgs) {
  const { t } = useTranslation();
  const [isBusy, setIsBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const canSubmit = transferCanSubmit({
    isBusy,
    destination,
    destinationValid,
    name,
    nameError,
    conflict,
    destinationsLoading,
    destinationsError,
  });

  const run = async () => {
    if (!canSubmit || destination === null) {
      return;
    }
    setIsBusy(true);
    setFormError(null);
    const trimmedName = name.trim();
    const payload = {
      target_scope: destination.scope,
      ...(destination.scope === "workspace"
        ? { target_workspace_id: destination.workspaceId }
        : {}),
      name: trimmedName,
    } as const;
    try {
      // A workspace-scoped source requires its workspace id in the query.
      const sourceOptions =
        secret.scope === "workspace" && secret.workspace_id
          ? { workspaceId: secret.workspace_id }
          : undefined;
      const item =
        mode === "copy"
          ? await copySecret(secret.id, payload, sourceOptions)
          : await moveSecret(secret.id, payload, sourceOptions);
      onCompleted(item, mode);
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setNameError(t("settings:secretNameConflictInDestination", { name: trimmedName }));
      } else {
        // 404 is deliberately ambiguous (missing source OR missing/unauthorized
        // destination) to avoid disclosure; never remove the source row on it.
        setFormError(t("settings:transferFailed"));
      }
    } finally {
      setIsBusy(false);
    }
  };

  return { busy: isBusy, canSubmit, formError, run };
}

function TargetNameField({
  name,
  onNameChange,
  nameError,
  setNameError,
  conflict,
  disabled,
}: {
  name: string;
  onNameChange: (name: string) => void;
  nameError: string | null;
  setNameError: (error: string | null) => void;
  conflict: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const invalid = conflict || nameError !== null;
  return (
    <div className="space-y-2">
      <Label htmlFor="copy-move-name">{t("settings:secretTargetName")}</Label>
      <Input
        id="copy-move-name"
        value={name}
        onChange={(e) => {
          onNameChange(e.target.value);
          if (nameError !== null) {
            setNameError(null);
          }
        }}
        disabled={disabled}
        aria-invalid={invalid}
        aria-describedby={invalid ? "copy-move-name-error" : undefined}
        className="min-h-11"
      />
      {invalid && (
        <p id="copy-move-name-error" className="text-xs text-destructive">
          {nameError ?? t("settings:secretNameConflictInDestination", { name: name.trim() })}
        </p>
      )}
    </div>
  );
}

type WorkspaceOption = { id: string; name: string };

/**
 * Computes the valid destinations for a secret (General plus every other
 * workspace) and keeps the selection in sync: auto-selects the first option
 * when nothing is chosen, and clears a selection that vanished from the list
 * (deleted workspace or failed list) so submission can never target a stale
 * destination. `null` keeps canSubmit false when nothing valid remains (a
 * Global source must never fall back to a same-scope target).
 */
function useDestinationOptions(
  secret: SecretListItem,
  workspaces: WorkspaceOption[],
  destination: SecretDestination | null,
  onDestinationChange: (destination: SecretDestination) => void,
) {
  const workspaceNameById = Object.fromEntries(
    workspaces.map((workspace) => [workspace.id, workspace.name]),
  );
  const destinations = useMemo(() => {
    const list: SecretDestination[] = [];
    if (secret.scope !== "global") {
      list.push({ scope: "global" });
    }
    for (const workspace of workspaces) {
      if (secret.scope === "workspace" && workspace.id === secret.workspace_id) {
        continue;
      }
      list.push({ scope: "workspace", workspaceId: workspace.id });
    }
    return list;
  }, [secret, workspaces]);

  // Synchronous validity: the selected destination must be present in the
  // CURRENT list. canSubmit consumes this on the same render, so a workspace
  // that vanished (deleted or failed list) can never be submitted even
  // between renders; the effect below only performs state cleanup.
  const destinationValid =
    destination !== null &&
    destinations.some((option) => {
      if (option.scope !== destination.scope) {
        return false;
      }
      if (destination.scope === "workspace") {
        return option.scope === "workspace" && option.workspaceId === destination.workspaceId;
      }
      return true;
    });

  useEffect(() => {
    if (destination === null && destinations.length > 0) {
      onDestinationChange(destinations[0]);
      return;
    }
    if (destination !== null && !destinationValid) {
      // The selected destination disappeared from the list: clear it so the
      // UI cannot show a stale selection. null keeps canSubmit false when
      // nothing valid remains (a Global source must never fall back to a
      // same-scope target).
      onDestinationChange(destinations[0] ?? null);
    }
  }, [destination, destinations, destinationValid, onDestinationChange]);

  return { destinations, workspaceNameById, destinationValid };
}

function TransferModeField({
  mode,
  onModeChange,
  originToken,
}: {
  mode: CopyMoveMode;
  onModeChange: (mode: CopyMoveMode) => void;
  originToken: string;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-2">
      <legend className="sr-only">{t("settings:copyMoveMode")}</legend>
      <RadioGroup
        value={mode}
        onValueChange={(value) => onModeChange(value as CopyMoveMode)}
        className="flex flex-col gap-2"
      >
        <label className="flex items-start gap-2 rounded-lg border border-border/70 p-3 cursor-pointer">
          <RadioGroupItem value="copy" className="mt-0.5 min-h-11 min-w-11" />
          <span className="space-y-1">
            <span className="block text-sm font-medium">{t("settings:copySecretAction")}</span>
            <span className="block text-xs text-muted-foreground">
              {t("settings:copyModeDescription")}
            </span>
          </span>
        </label>
        <label className="flex items-start gap-2 rounded-lg border border-border/70 p-3 cursor-pointer">
          <RadioGroupItem value="move" className="mt-0.5 min-h-11 min-w-11" />
          <span className="space-y-1">
            <span className="block text-sm font-medium">{t("settings:moveSecretAction")}</span>
            {mode === "move" && (
              <span className="block text-xs text-muted-foreground">
                {t("settings:moveModeWarning", { origin: originToken })}
              </span>
            )}
          </span>
        </label>
      </RadioGroup>
    </fieldset>
  );
}

function DestinationField({
  destinations,
  workspaceNameById,
  destination,
  onDestinationChange,
  destinationsLoading,
  destinationsError,
  onRetryDestinations,
  disabled,
}: {
  destinations: SecretDestination[];
  workspaceNameById: Record<string, string>;
  destination: SecretDestination | null;
  onDestinationChange: (destination: SecretDestination) => void;
  destinationsLoading: boolean;
  destinationsError: string | null;
  onRetryDestinations: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const noDestinations =
    destinations.length === 0 && !destinationsLoading && destinationsError === null;

  return (
    <div className="space-y-2">
      <Label htmlFor="copy-move-destination">{t("settings:destination")}</Label>
      {destinationsLoading && (
        <p className="text-xs text-muted-foreground">{t("settings:destinationsLoading")}</p>
      )}
      {destinationsError !== null && (
        <div className="flex items-center gap-2">
          <p className="text-xs text-destructive">{t("settings:destinationsLoadFailed")}</p>
          <Button
            variant="outline"
            size="sm"
            onClick={onRetryDestinations}
            className="min-h-11 cursor-pointer"
          >
            {t("settings:retryDestinations")}
          </Button>
        </div>
      )}
      {noDestinations && (
        <p className="text-xs text-destructive">{t("settings:noValidDestinations")}</p>
      )}
      {destination !== null && (
        <Select
          value={destination.scope === "global" ? "global" : `workspace:${destination.workspaceId}`}
          onValueChange={(value) =>
            onDestinationChange(
              value === "global"
                ? { scope: "global" }
                : { scope: "workspace", workspaceId: value.slice("workspace:".length) },
            )
          }
        >
          <SelectTrigger
            id="copy-move-destination"
            disabled={disabled}
            className="min-h-11 w-full cursor-pointer"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {destinations.map((option) =>
              option.scope === "global" ? (
                <SelectItem key="global" value="global">
                  {t("settings:destinationGeneral")}
                </SelectItem>
              ) : (
                <SelectItem key={option.workspaceId} value={`workspace:${option.workspaceId}`}>
                  {workspaceNameById[option.workspaceId] ?? option.workspaceId}
                </SelectItem>
              ),
            )}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}
