import type { TaskPlan } from "@/lib/types/http";
import { t } from "@/lib/i18n";

type PlanToolbarImplementArgs = {
  draftContent: string;
  plan: TaskPlan | null;
};

export type PlanToolbarImplementState = {
  visible: boolean;
  disabled: boolean;
  disabledReason?: string;
};

export function getPlanToolbarImplementState({
  draftContent,
  plan,
}: PlanToolbarImplementArgs): PlanToolbarImplementState {
  if (plan?.implementation_started_at) {
    return {
      visible: true,
      disabled: true,
      disabledReason: t("task:thisPlanHasAlreadyBeenSent"),
    };
  }
  if (draftContent.trim() === "") {
    return { visible: false, disabled: false };
  }
  return { visible: true, disabled: false };
}
