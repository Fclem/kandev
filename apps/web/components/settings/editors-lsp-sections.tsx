"use client";

import { t } from "@lingui/core/macro";
import { Trans } from "@lingui/react/macro";
import { IconChevronDown, IconExternalLink, IconInfoCircle } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Checkbox } from "@kandev/ui/checkbox";
import { Switch } from "@kandev/ui/switch";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { LSP_DEFAULT_CONFIGS } from "@/lib/lsp/lsp-client-manager";
import { isDraftEntryDirty } from "./settings-dirty";

const LSP_LANGUAGE_OPTIONS = [
  {
    id: "typescript",
    label: "TypeScript / JavaScript",
    binary: "typescript-language-server",
    docsUrl:
      "https://github.com/typescript-language-server/typescript-language-server#workspace-configuration",
  },
  {
    id: "go",
    label: "Go",
    binary: "gopls",
    docsUrl: "https://github.com/golang/tools/blob/master/gopls/doc/settings.md",
  },
  {
    id: "rust",
    label: "Rust",
    binary: "rust-analyzer",
    docsUrl: "https://rust-analyzer.github.io/book/configuration.html",
  },
  {
    id: "python",
    label: "Python",
    binary: "pyright-langserver",
    docsUrl: "https://microsoft.github.io/pyright/#/settings",
  },
];

/** Localized install hint per language server (see LSP_LANGUAGE_OPTIONS ids). */
function lspInstallHint(langId: string): string {
  switch (langId) {
    case "typescript":
      return t`Installs typescript-language-server and typescript via npm into ~/.kandev/lsp-servers/`;
    case "go":
      return t`Runs "go install golang.org/x/tools/gopls@latest". Requires Go to be installed.`;
    case "rust":
      return t`Downloads the rust-analyzer binary from GitHub releases into ~/.kandev/lsp-servers/`;
    case "python":
      return t`Installs pyright via npm into ~/.kandev/lsp-servers/`;
    default:
      return "";
  }
}

type LspLanguageCardsProps = {
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  baselineLspAutoStart: string[];
  baselineLspAutoInstall: string[];
  toggleAutoStart: (langId: string, checked: boolean) => void;
  toggleAutoInstall: (langId: string, checked: boolean) => void;
};

export function LspLanguageCards({
  lspAutoStartLanguages,
  lspAutoInstallLanguages,
  baselineLspAutoStart,
  baselineLspAutoInstall,
  toggleAutoStart,
  toggleAutoInstall,
}: LspLanguageCardsProps) {
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">
          <Trans>Language Servers</Trans>
        </div>
        <div className="text-xs text-muted-foreground">
          <Trans>
            Auto-start language servers when opening files to get diagnostics, hover info, and
            go-to-definition. You can also toggle each server on/off per file.
            <br />
            When enabled, install your project&apos;s dependencies (e.g.{" "}
            <code className="text-[11px] bg-muted px-1 rounded">npm install</code> via repository
            setup scripts) to avoid missing type errors.
          </Trans>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {LSP_LANGUAGE_OPTIONS.map((lang) => {
          const autoStartDirty =
            lspAutoStartLanguages.includes(lang.id) !== baselineLspAutoStart.includes(lang.id);
          const autoInstallDirty =
            lspAutoInstallLanguages.includes(lang.id) !== baselineLspAutoInstall.includes(lang.id);
          return (
            <div
              key={lang.id}
              className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-2.5"
              data-settings-dirty={autoStartDirty || autoInstallDirty}
            >
              <div>
                <div className="text-sm font-medium text-foreground">{lang.label}</div>
                <div className="text-xs text-muted-foreground">{lang.binary}</div>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">
                  <Trans>Auto-start</Trans>
                </span>
                <Switch
                  checked={lspAutoStartLanguages.includes(lang.id)}
                  onCheckedChange={(checked) => toggleAutoStart(lang.id, checked === true)}
                  data-settings-dirty={autoStartDirty}
                />
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`lsp-install-${lang.id}`}
                  checked={lspAutoInstallLanguages.includes(lang.id)}
                  onCheckedChange={(checked) => toggleAutoInstall(lang.id, checked === true)}
                  className="h-3.5 w-3.5"
                  data-settings-dirty={autoInstallDirty}
                />
                <label
                  htmlFor={`lsp-install-${lang.id}`}
                  className="text-xs text-muted-foreground cursor-pointer"
                >
                  <Trans>Auto-install if not found</Trans>
                </label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-[260px] text-xs">
                    {lspInstallHint(lang.id)}
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

type LspServerConfigSectionProps = {
  lspConfigStrings: Record<string, string>;
  baselineLspConfigStrings: Record<string, string>;
  lspConfigErrors: Record<string, string>;
  expandedConfigLang: string | null;
  setExpandedConfigLang: (lang: string | null) => void;
  updateLspConfigString: (langId: string, value: string) => void;
};

export function LspServerConfigSection({
  lspConfigStrings,
  baselineLspConfigStrings,
  lspConfigErrors,
  expandedConfigLang,
  setExpandedConfigLang,
  updateLspConfigString,
}: LspServerConfigSectionProps) {
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">
          <Trans>Server Configuration</Trans>
        </div>
        <div className="text-xs text-muted-foreground">
          <Trans>
            Override settings sent to each language server via{" "}
            <code className="text-[11px] bg-muted px-1 rounded">workspace/configuration</code>. JSON
            format.
          </Trans>
        </div>
      </div>
      {LSP_LANGUAGE_OPTIONS.map((lang) => {
        const isExpanded = expandedConfigLang === lang.id;
        const configStr = lspConfigStrings[lang.id] ?? "";
        const defaultConfig = LSP_DEFAULT_CONFIGS[lang.id];
        const hasDefaults = defaultConfig && Object.keys(defaultConfig).length > 0;
        const defaultConfigJson = JSON.stringify(defaultConfig);
        const error = lspConfigErrors[lang.id];
        const isDirty = isDraftEntryDirty(lspConfigStrings, baselineLspConfigStrings, lang.id);
        return (
          <div
            key={lang.id}
            className="rounded-lg border border-border/60 bg-background overflow-hidden"
            data-settings-dirty={isDirty}
          >
            <button
              type="button"
              className="flex w-full items-center justify-between px-4 py-2.5 text-left hover:bg-muted/50 transition-colors"
              onClick={() => setExpandedConfigLang(isExpanded ? null : lang.id)}
            >
              <div className="flex items-center gap-2">
                <span className="text-sm text-foreground">{lang.label}</span>
                {configStr.trim() && (
                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                    <Trans>custom</Trans>
                  </Badge>
                )}
              </div>
              <IconChevronDown
                className={`h-4 w-4 text-muted-foreground transition-transform ${isExpanded ? "rotate-180" : ""}`}
              />
            </button>
            {isExpanded && (
              <div className="border-t border-border/60 px-4 py-3 space-y-2">
                {hasDefaults && (
                  <div className="text-[11px] text-muted-foreground">
                    <Trans>
                      Defaults: <code className="bg-muted px-1 rounded">{defaultConfigJson}</code>
                    </Trans>
                  </div>
                )}
                <a
                  href={lang.docsUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
                >
                  <Trans>View available settings</Trans>
                  <IconExternalLink className="h-3 w-3" />
                </a>
                <Textarea
                  value={configStr}
                  onChange={(e) => updateLspConfigString(lang.id, e.target.value)}
                  placeholder={hasDefaults ? JSON.stringify(defaultConfig, null, 2) : "{\n  \n}"}
                  className="font-mono text-xs min-h-[80px] resize-y"
                  rows={4}
                  data-settings-dirty={isDirty}
                />
                {error && <div className="text-xs text-destructive">{error}</div>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
