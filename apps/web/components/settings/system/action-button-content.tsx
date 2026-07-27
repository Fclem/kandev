"use client";

import { useLingui } from "@lingui/react/macro";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconCheck } from "@tabler/icons-react";
import type { ReactNode } from "react";
import type { ActionFeedbackState } from "@/hooks/use-action-feedback";

type Props = {
  state: ActionFeedbackState;
  /** Icon to show in the idle state (button's resting icon). */
  idleIcon: ReactNode;
  /** Label to show in the idle state (e.g. "Run vacuum"). */
  idleLabel: string;
  /** Label to show while pending. Defaults to "Running...". */
  pendingLabel?: string;
  /** Label to show on success. Defaults to "Done". */
  successLabel?: string;
  /** Label to show on error. Defaults to "Failed". */
  errorLabel?: string;
};

/**
 * Renders the icon + label inside an action button, switching by feedback
 * state. Pair with useActionFeedback to make fast operations visible to the
 * user (the spinner is held for a minimum perceptible window, then success
 * tick stays for ~1.8s before reverting to idle).
 */
export function ActionButtonContent({
  state,
  idleIcon,
  idleLabel,
  pendingLabel,
  successLabel,
  errorLabel,
}: Props) {
  const { t } = useLingui();
  if (state === "pending") {
    return (
      <>
        <Spinner className="size-3.5 mr-1" />
        {pendingLabel ?? t`Running...`}
      </>
    );
  }
  if (state === "success") {
    return (
      <>
        <IconCheck className="h-3.5 w-3.5 mr-1 text-emerald-500" />
        {successLabel ?? t`Done`}
      </>
    );
  }
  if (state === "error") {
    return (
      <>
        <IconAlertTriangle className="h-3.5 w-3.5 mr-1 text-red-500" />
        {errorLabel ?? t`Failed`}
      </>
    );
  }
  return (
    <>
      {idleIcon}
      {idleLabel}
    </>
  );
}
