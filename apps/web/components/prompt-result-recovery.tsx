"use client";

import { Trans } from "@lingui/react/macro";
import { Button } from "@kandev/ui/button";

import type { UtilityGenerationResult } from "@/hooks/use-utility-agent-generator";

type PromptResultRecoveryProps = {
  pendingResult: UtilityGenerationResult | null;
  onApply: () => void;
  onCopy: () => Promise<void> | void;
};

export function PromptResultRecovery({
  pendingResult,
  onApply,
  onCopy,
}: PromptResultRecoveryProps) {
  if (!pendingResult) {
    return null;
  }

  return (
    <div
      aria-live="polite"
      data-testid="prompt-result-recovery"
      className="flex flex-col gap-2 rounded-md border border-border/60 bg-muted/30 p-3"
    >
      <p className="text-sm text-muted-foreground">
        <Trans>An enhanced prompt is available.</Trans>
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          onClick={() => void onCopy()}
        >
          <Trans>Copy</Trans>
        </Button>
        <Button type="button" className="cursor-pointer" onClick={onApply}>
          <Trans>Apply</Trans>
        </Button>
      </div>
    </div>
  );
}
