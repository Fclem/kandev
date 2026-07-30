"use client";
import { useTranslation } from "react-i18next";

/**
 * Sticky per-repository section header rendered above each group of files in
 * the changes panel. The grouping logic lives in @/lib/group-by-repo so the
 * Review dialog and the dockview Changes tab share the same Map-based
 * bucketer. Only this presentational header is review-specific.
 */
export function RepoGroupHeader({ name, fileCount }: { name: string; fileCount: number }) {
  const { t } = useTranslation();
  // The empty-name group ("uncategorised" — no repository_name on its files)
  // gets a generic label so the user understands what they're looking at.
  const label = name || "Other changes";
  return (
    <div
      className="sticky top-0 z-20 flex items-center gap-2 px-4 py-1.5 bg-muted/40 backdrop-blur-sm border-y border-border/60 text-xs font-medium text-foreground"
      data-testid="changes-repo-header"
    >
      <span className="truncate">{label}</span>
      <span className="text-muted-foreground/70">
        {fileCount} {fileCount === 1 ? t("review:file") : t("review:files")}
      </span>
    </div>
  );
}
