"use client";

import { IconStar } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { usePluginRepositoryStar } from "@/hooks/domains/plugins/use-plugin-repository-stars";
import { cn } from "@/lib/utils";

/**
 * The shared star affordance for a plugin repository, used by the marketplace
 * catalog, the installed-plugin row and detail, and the Canvases detail and
 * install dialog.
 *
 * It owns no data: the hook resolves the connection tuple and the scoped star
 * state, and this component only decides what the tuple may assert. An
 * unsupported deployment renders nothing at all; an inactive or incapable
 * connection renders a localized Connect/Reconnect CTA and never claims the
 * repository is unstarred; an unknown or failed status hides the toggle rather
 * than rendering it as "not starred".
 */
export function PluginRepositoryStar({
  workspaceId,
  repositoryUrl,
  className,
}: {
  /** The surface's owning (or selected) workspace; never an implicit global. */
  workspaceId?: string | null;
  repositoryUrl?: string | null;
  className?: string;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const star = usePluginRepositoryStar({ workspaceId, repositoryUrl });

  // Touch contexts (a phone viewport, or a coarse pointer at any width) keep a
  // 44px target; only a non-mobile fine pointer gets the compact inline size.
  const isTouchContext = isMobile || !isFinePointer;
  const sizeClass = isTouchContext ? "h-11 min-h-11 px-3" : "h-6 px-2";

  if (star.mode === "omit") return null;

  if (star.mode === "stars") {
    return (
      <span className={cn("inline-flex items-center gap-1.5", className)}>
        {star.starred !== null && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-pressed={star.starred}
            disabled={star.busy}
            onClick={star.toggle}
            data-testid="plugin-star-toggle"
            className={cn("cursor-pointer", sizeClass)}
          >
            <IconStar aria-hidden="true" className={star.starred ? "fill-current" : undefined} />
            {star.starred ? t("plugins:unstar") : t("plugins:star")}
          </Button>
        )}
        {star.reconnectRequired && (
          <GitHubCta label={t("github:reconnectGithub")} busy={star.starting} onClick={star.reconnect} className={cn("cursor-pointer", sizeClass)} />
        )}
        <span role="status" className="sr-only">
          {star.busy ? t("plugins:starUpdating") : ""}
        </span>
      </span>
    );
  }

  return (
    <GitHubCta
      label={
        star.mode === "reconnect" ? t("github:reconnectGithub") : t("github:connectGithub")
      }
      busy={star.starting}
      onClick={star.connect}
      className={cn("cursor-pointer", sizeClass, className)}
    />
  );
}

/** The consent/reconnect entry point into the dedicated personal star flow. */
function GitHubCta({
  label,
  busy,
  onClick,
  className,
}: {
  label: string;
  busy: boolean;
  onClick: () => void;
  className?: string;
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={busy}
      onClick={onClick}
      data-testid="plugin-star-connect"
      className={className}
    >
      {label}
    </Button>
  );
}
