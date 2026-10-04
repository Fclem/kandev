export const MESSAGE_METADATA_FIELDS = [
  "session_config",
  "timestamp",
  "duration",
  "ttft",
  "tps",
  "tokens",
  "cost",
] as const;

export type MessageMetadataField = (typeof MESSAGE_METADATA_FIELDS)[number];

export const DEFAULT_MESSAGE_METADATA_FIELDS: readonly MessageMetadataField[] = [
  "session_config",
  "timestamp",
  "duration",
];

const messageMetadataFieldSet: Record<MessageMetadataField, true> = {
  session_config: true,
  timestamp: true,
  duration: true,
  ttft: true,
  tps: true,
  tokens: true,
  cost: true,
};

export function isMessageMetadataField(value: unknown): value is MessageMetadataField {
  return typeof value === "string" && Object.hasOwn(messageMetadataFieldSet, value);
}

export function parseMessageMetadataFields(
  value: readonly string[] | undefined,
): MessageMetadataField[] {
  if (value === undefined) return [...DEFAULT_MESSAGE_METADATA_FIELDS];
  const fields = new Set<MessageMetadataField>();
  for (const field of value) {
    if (isMessageMetadataField(field)) fields.add(field);
  }
  return MESSAGE_METADATA_FIELDS.filter((field) => fields.has(field));
}
