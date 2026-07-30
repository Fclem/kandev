import type { FilterDimension, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";

export type DimensionValueKind = "boolean" | "enum" | "text";

export type DimensionMeta = {
  dimension: FilterDimension;
  labelKey: string;
  valueKind: DimensionValueKind;
  ops: FilterOp[];
  enumOptions?: Array<{ value: string; labelKey: string }>;
  placeholderKey?: string;
  defaultOp: FilterOp;
  defaultValue: string | string[] | boolean;
};

const STATE_OPTIONS = [
  { value: "review", labelKey: "task:review2" },
  { value: "in_progress", labelKey: "task:inProgress2" },
  { value: "backlog", labelKey: "task:backlog" },
];

export const DIMENSION_METAS: DimensionMeta[] = [
  {
    dimension: "isPRReview",
    labelKey: "task:prReview",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "isIssueWatch",
    labelKey: "task:issueWatch",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "archived",
    labelKey: "task:archived",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasDiff",
    labelKey: "task:hasDiff",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "hasPR",
    labelKey: "task:hasPr",
    valueKind: "boolean",
    ops: ["is", "is_not"],
    defaultOp: "is",
    defaultValue: true,
  },
  {
    dimension: "state",
    labelKey: "task:state3",
    valueKind: "enum",
    ops: ["in", "not_in", "is", "is_not"],
    enumOptions: STATE_OPTIONS,
    defaultOp: "in",
    defaultValue: ["review", "in_progress"],
  },
  {
    dimension: "workflow",
    labelKey: "task:workflow3",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "workflowStep",
    labelKey: "task:workflowStep",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "executorType",
    labelKey: "task:executorType",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "repository",
    labelKey: "task:repositoryFallback",
    valueKind: "enum",
    ops: ["is", "is_not", "in", "not_in"],
    defaultOp: "is",
    defaultValue: "",
  },
  {
    dimension: "titleMatch",
    labelKey: "task:title",
    valueKind: "text",
    ops: ["matches", "not_matches"],
    placeholderKey: "task:substring",
    defaultOp: "matches",
    defaultValue: "",
  },
];

export function getDimensionMeta(dim: FilterDimension): DimensionMeta {
  const meta = DIMENSION_METAS.find((m) => m.dimension === dim);
  if (!meta) throw new Error(`Unknown filter dimension: ${dim}`);
  return meta;
}

export const OP_LABELS: Record<FilterOp, string> = {
  is: "is",
  is_not: "is not",
  in: "in",
  not_in: "not in",
  matches: "contains",
  not_matches: "does not contain",
};

export function getOpLabel(op: FilterOp, valueKind: DimensionValueKind): string {
  if (valueKind === "boolean") {
    if (op === "is") return "Show";
    if (op === "is_not") return "Hide";
  }
  return OP_LABELS[op];
}
