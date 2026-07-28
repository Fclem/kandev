"use client";

import { IconSend } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type SendCommentButtonProps = {
  disabled: boolean;
  onSubmit: () => Promise<void> | void;
};

export function SendCommentButton({ disabled, onSubmit }: SendCommentButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* While disabled the button leaves the tab order and this span takes
            focus, so the span has to carry the name in that state. */}
        <span
          tabIndex={disabled ? 0 : -1}
          aria-label={disabled ? "Send comment unavailable" : undefined}
          className="inline-flex"
        >
          <Button
            aria-label="Send comment"
            type="button"
            size="icon"
            className="h-7 w-7 cursor-pointer"
            disabled={disabled}
            onClick={() => void onSubmit()}
          >
            <IconSend className="h-3.5 w-3.5" />
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>Send comment</TooltipContent>
    </Tooltip>
  );
}
