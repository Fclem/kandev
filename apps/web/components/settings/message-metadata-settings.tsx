"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { updateUserSettings } from "@/lib/api";
import {
  MESSAGE_METADATA_FIELDS,
  parseMessageMetadataFields,
  type MessageMetadataField,
} from "@/lib/message-metadata-fields";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { SettingsCard } from "./settings-card";
import { useSettingsSaveContributor } from "./settings-save-provider";

const fieldLabelKeys: Record<MessageMetadataField, string> = {
  session_config: "settings:messageMetadataSessionOptions",
  timestamp: "settings:messageMetadataTimestamp",
  duration: "settings:messageMetadataDuration",
  ttft: "settings:messageMetadataTTFT",
  tps: "settings:messageMetadataTokensPerSecond",
  tokens: "settings:messageMetadataTokens",
  cost: "settings:messageMetadataCost",
};

function serializeFields(fields: readonly MessageMetadataField[]) {
  return parseMessageMetadataFields(fields).join(",");
}

export function MessageMetadataSettings() {
  const { t } = useTranslation();
  const savedFields = useAppStore((state) => state.userSettings.messageMetadataFields);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const storeApi = useAppStoreApi();
  const [saved, setSaved] = useState(() => parseMessageMetadataFields(savedFields));
  const [draft, setDraft] = useState(() => parseMessageMetadataFields(savedFields));
  const draftRef = useRef(draft);
  draftRef.current = draft;
  const isDirty = serializeFields(draft) !== serializeFields(saved);

  useEffect(() => {
    const nextSaved = parseMessageMetadataFields(savedFields);
    setSaved((previous) => {
      if (serializeFields(draftRef.current) === serializeFields(previous)) setDraft(nextSaved);
      return nextSaved;
    });
  }, [savedFields]);

  useSettingsSaveContributor({
    id: "general-message-metadata-fields",
    revision: serializeFields(draft),
    isDirty,
    save: async (revision) => {
      const submitted = parseMessageMetadataFields(String(revision).split(",").filter(Boolean));
      await updateUserSettings({ message_metadata_fields: submitted });
      setSaved(submitted);
      setUserSettings({
        ...storeApi.getState().userSettings,
        messageMetadataFields: submitted,
      });
    },
    discard: () => setDraft(saved),
  });

  return (
    <SettingsCard
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.messageMetadata}
      isDirty={isDirty}
      data-testid="message-metadata-settings"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:messageMetadata")}</CardTitle>
        <p className="text-sm text-muted-foreground">{t("settings:messageMetadataDescription")}</p>
      </CardHeader>
      <CardContent>
        <div className="grid min-w-0 grid-cols-1 gap-x-4 sm:grid-cols-2">
          {MESSAGE_METADATA_FIELDS.map((field) => {
            const checked = draft.includes(field);
            return (
              <label
                key={field}
                className="flex min-h-11 min-w-0 cursor-pointer items-center gap-3 rounded-md px-2 py-2 [@media(pointer:coarse)]:min-h-11"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  data-state={checked ? "checked" : "unchecked"}
                  data-settings-dirty={isDirty}
                  onChange={(event) =>
                    setDraft((current) =>
                      parseMessageMetadataFields(
                        event.target.checked
                          ? [...current, field]
                          : current.filter((selected) => selected !== field),
                      ),
                    )
                  }
                  className="size-4 shrink-0 accent-primary"
                />
                <span className="min-w-0 break-words text-sm">{t(fieldLabelKeys[field])}</span>
              </label>
            );
          })}
        </div>
      </CardContent>
    </SettingsCard>
  );
}
