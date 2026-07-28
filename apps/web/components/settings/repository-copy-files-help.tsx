"use client";
import { Trans, useTranslation } from "react-i18next";
import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Repository } from "@/lib/types/http";

type CopyFilesFieldProps = {
  repositoryId: string;
  copyFiles: string;
  isDirty?: boolean;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

export function CopyFilesField({
  repositoryId,
  copyFiles,
  isDirty = false,
  onUpdate,
}: CopyFilesFieldProps) {
  const { t } = useTranslation();
  const inputId = `copy-files-${repositoryId}`;
  const helpId = `copy-files-help-${repositoryId}`;
  return (
    <div className="space-y-2">
      <Label htmlFor={inputId}>{t("settings:copyFiles")}</Label>
      <Textarea
        id={inputId}
        data-testid={`copy-files-input-${repositoryId}`}
        aria-describedby={helpId}
        value={copyFiles}
        onChange={(e) => onUpdate(repositoryId, { copy_files: e.target.value })}
        placeholder=".env, .env.*, apps/**/.env, .env.local:symlink"
        rows={2}
        className="font-mono text-sm"
        data-settings-dirty={isDirty}
      />
      <p id={helpId} className="text-xs text-muted-foreground">
        <Trans i18nKey="settings:gitignoredPathsCopiedIntoNewWorktrees">
          Gitignored paths copied into new worktrees. Append{" "}
          <code className="px-1 py-0.5 bg-muted rounded">:symlink</code> to an entry to link it back
          to the main repo. Use <code className="px-1 py-0.5 bg-muted rounded">::symlink</code> for
          a literal filename ending in{" "}
          <code className="px-1 py-0.5 bg-muted rounded">:symlink</code>.
        </Trans>
      </p>
      <p data-testid="copy-files-remote-fallback" className="text-xs text-muted-foreground">
        {t("settings:remoteExecutorsCopyFileContentsInstead")}
      </p>
      <CopyFilesDetails />
    </div>
  );
}

const braceAlternationToken = "{a,b}";
const braceAlternationExample = ".env{,.local}";

function CopyFilesDetails() {
  const { t } = useTranslation();
  return (
    <details className="group text-xs text-muted-foreground">
      <summary className="flex min-h-11 w-fit cursor-pointer list-none items-center gap-1.5 py-2 font-medium text-foreground">
        <IconInfoCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
        {t("settings:patternSyntax")}
        <IconChevronDown
          className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180"
          aria-hidden="true"
        />
      </summary>
      <div className="max-w-sm space-y-2 pb-1">
        <p>{t("settings:pathsAreResolvedRelativeToThe")}</p>
        <p className="font-medium">{t("settings:supportedPatterns")}</p>
        <ul className="space-y-1 pl-3 list-disc">
          <li>
            <Trans i18nKey="settings:envLiteralFileOrDirectoryDirectories">
              <code className="px-1 py-0.5 bg-muted rounded">.env</code> literal file or directory
              (directories copy recursively)
            </Trans>
          </li>
          <li>
            <Trans i18nKey="settings:abcSingleSegmentWildcards">
              <code className="px-1 py-0.5 bg-muted rounded">*</code>,{" "}
              <code className="px-1 py-0.5 bg-muted rounded">?</code>,{" "}
              <code className="px-1 py-0.5 bg-muted rounded">[abc]</code> single-segment wildcards
            </Trans>
          </li>
          <li>
            <Trans i18nKey="settings:matchesAnyNumberOfDirectoriesE">
              <code className="px-1 py-0.5 bg-muted rounded">**</code> matches any number of
              directories, e.g. <code className="px-1 py-0.5 bg-muted rounded">**/.env</code>
            </Trans>
          </li>
          <li>
            <Trans
              i18nKey="settings:braceAlternationEG"
              values={{ braceAlternationExample, braceAlternationToken }}
            >
              <code className="px-1 py-0.5 bg-muted rounded">{braceAlternationToken}</code> brace
              alternation, e.g.{" "}
              <code className="px-1 py-0.5 bg-muted rounded">{braceAlternationExample}</code>
            </Trans>
          </li>
        </ul>
        <p className="text-muted-foreground">{t("settings:filesOver5MibAreSkipped")}</p>
      </div>
    </details>
  );
}
