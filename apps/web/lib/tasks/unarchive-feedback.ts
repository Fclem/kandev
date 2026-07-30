import type { UnarchiveTaskResponse } from "@/lib/api/domains/kanban-api";
import { t } from "@/lib/i18n";

export type UnarchiveToastPayload = {
  title: string;
  description: string;
  variant?: "success";
};

// Builds the toast shown after a successful unarchive. When the archived
// branch no longer exists anywhere (never pushed, local copy deleted by
// archive), the prior work is unrecoverable and the user must be told the
// next session starts fresh.
export function unarchiveToastPayload(result: UnarchiveTaskResponse): UnarchiveToastPayload {
  const missing = (result.recovery ?? []).filter((r) => r.status === "missing");
  if (missing.length > 0) {
    const branches = missing.map((r) => r.branch).join(", ");
    const plural = missing.length > 1;
    return {
      title: t("common:taskUnarchived"),
      description: `${plural ? "Branches" : "Branch"} ${branches} no longer ${plural ? "exist" : "exists"} locally or on the remote — the next session starts fresh from the base branch.`,
    };
  }
  return {
    title: t("common:taskUnarchived"),
    description: t("common:theTaskHasBeenRestored"),
    variant: "success",
  };
}
