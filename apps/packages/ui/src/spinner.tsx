import * as React from "react";
import { cn } from "./lib/utils";
import { useUIStrings } from "./lib/ui-strings";
import { IconLoader } from "@tabler/icons-react";

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  const uiStrings = useUIStrings();
  return (
    <IconLoader
      role="status"
      aria-label={uiStrings.loading}
      className={cn("size-4 animate-spin", className)}
      {...props}
    />
  );
}

export { Spinner };
