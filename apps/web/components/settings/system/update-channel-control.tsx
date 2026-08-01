"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { UpdatesChannel, UpdatesResponse } from "@/lib/types/system";
import { useSettingsSaveContributor } from "../settings-save-provider";

export function useUpdateChannelDraft(
  updates: UpdatesResponse | null | undefined,
  saveChannel: (channel: UpdatesChannel) => Promise<UpdatesResponse>,
  serviceUpdater: boolean,
) {
  const authoritative = updates?.channel ?? "stable";
  const editable = serviceUpdater && updates?.channel_editable === true;
  const [saved, setSaved] = useState<UpdatesChannel>(authoritative);
  const savedRef = useRef(saved);
  const [draft, setDraftState] = useState<UpdatesChannel>(authoritative);
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = editable && draft !== saved;

  const setDraft = (channel: UpdatesChannel) => {
    draftRef.current = channel;
    setDraftState(channel);
  };

  useLayoutEffect(() => {
    const previous = savedRef.current;
    savedRef.current = authoritative;
    setSaved(authoritative);
    if (!editable || draftRef.current === previous) {
      draftRef.current = authoritative;
      setDraftState(authoritative);
    }
  }, [authoritative, editable]);

  useSettingsSaveContributor({
    id: "system-updates-channel",
    order: 10,
    revision: draft,
    isDirty,
    save: async (revision) => {
      const submitted = revision as UpdatesChannel;
      const response = await saveChannel(submitted);
      savedRef.current = response.channel;
      setSaved(response.channel);
      if (draftRef.current === submitted) setDraft(response.channel);
    },
    discard: () => setDraft(savedRef.current),
  });

  return {
    draft,
    editable,
    isDirty,
    unsupportedReason: updates?.channel_unsupported_reason ?? "",
    setDraft,
  };
}

type UpdateChannelControlProps = ReturnType<typeof useUpdateChannelDraft>;

export function UpdateChannelControl({
  draft,
  editable,
  isDirty,
  unsupportedReason,
  setDraft,
}: UpdateChannelControlProps) {
  const reasonId = "system-updates-channel-reason";
  return (
    <div
      className="min-w-0 space-y-2"
      data-testid="system-updates-channel"
      data-settings-dirty={isDirty}
    >
      <div>
        <div className="text-sm font-medium">Update channel</div>
        <p className="text-xs text-muted-foreground">
          Choose which releases this managed service checks and applies.
        </p>
      </div>
      <RadioGroup
        aria-label="Update channel"
        value={draft}
        onValueChange={(value) => {
          if (editable && (value === "stable" || value === "nightly")) setDraft(value);
        }}
        className="gap-2"
        data-settings-dirty={isDirty}
      >
        <UpdateChannelOption
          channel="stable"
          label="Stable"
          description="Signed GitHub releases. Recommended for most users."
          disabled={!editable}
        />
        <UpdateChannelOption
          channel="nightly"
          label="Nightly"
          description="Prerelease builds from main, delivered through npm."
          disabled={!editable}
          reasonId={!editable && unsupportedReason ? reasonId : undefined}
        />
      </RadioGroup>
      {!editable && unsupportedReason && (
        <p
          id={reasonId}
          className="break-words text-xs text-muted-foreground"
          data-testid="system-updates-channel-reason"
        >
          {unsupportedReason}
        </p>
      )}
    </div>
  );
}

function UpdateChannelOption({
  channel,
  label,
  description,
  disabled,
  reasonId,
}: {
  channel: UpdatesChannel;
  label: string;
  description: string;
  disabled: boolean;
  reasonId?: string;
}) {
  const inputId = `system-updates-channel-${channel}-input`;
  const labelId = `system-updates-channel-${channel}-label`;
  const descriptionId = `system-updates-channel-${channel}-description`;
  const describedBy = reasonId ? `${descriptionId} ${reasonId}` : descriptionId;
  return (
    <Label
      htmlFor={inputId}
      className={`flex min-h-11 w-full min-w-0 items-start gap-3 rounded-md border p-3 ${
        disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer hover:bg-muted/30"
      }`}
      data-testid={`system-updates-channel-${channel}`}
    >
      <RadioGroupItem
        id={inputId}
        value={channel}
        disabled={disabled}
        aria-labelledby={labelId}
        aria-describedby={describedBy}
        className="mt-0.5"
      />
      <span className="min-w-0 space-y-1">
        <span id={labelId} className="block text-sm font-medium">
          {label}
        </span>
        <span
          id={descriptionId}
          className="block whitespace-normal break-words text-xs text-muted-foreground"
        >
          {description}
        </span>
      </span>
    </Label>
  );
}
