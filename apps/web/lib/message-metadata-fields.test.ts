import { createDefaultUserSettings, mapUserSettingsData } from "./ssr/user-settings";
import { describe, expect, it } from "vitest";
import {
  DEFAULT_MESSAGE_METADATA_FIELDS,
  MESSAGE_METADATA_FIELDS,
  parseMessageMetadataFields,
} from "./message-metadata-fields";

describe("message metadata fields", () => {
  it("uses the established items when no preference exists", () => {
    expect(parseMessageMetadataFields(undefined)).toEqual([
      "session_config",
      "timestamp",
      "duration",
    ]);
    expect(DEFAULT_MESSAGE_METADATA_FIELDS).toEqual(parseMessageMetadataFields(undefined));
  });

  it("preserves an empty selection and drops unknown values in fixed order", () => {
    expect(parseMessageMetadataFields([])).toEqual([]);
    expect(parseMessageMetadataFields(["cost", "ttft", "future"])).toEqual(["ttft", "cost"]);
    expect(MESSAGE_METADATA_FIELDS).toEqual([
      "session_config",
      "timestamp",
      "duration",
      "ttft",
      "tps",
      "tokens",
      "cost",
    ]);
  });

  it("hydrates defaults and preserves explicit empty values", () => {
    expect(createDefaultUserSettings().messageMetadataFields).toEqual([
      "session_config",
      "timestamp",
      "duration",
    ]);
    expect(
      mapUserSettingsData({ message_metadata_fields: [] }, createDefaultUserSettings())
        .messageMetadataFields,
    ).toEqual([]);
  });
});
