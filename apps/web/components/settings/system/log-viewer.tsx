"use client";
import { Trans, useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Badge } from "@kandev/ui/badge";
import { IconCopy, IconDownload, IconFileText, IconRefresh } from "@tabler/icons-react";
import { useLogFiles } from "@/hooks/domains/system/use-log-files";
import { useLogTail } from "@/hooks/domains/system/use-log-tail";
import { buildLogDownloadUrl } from "@/lib/api/domains/system-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { useActionFeedback, type ActionFeedbackState } from "@/hooks/use-action-feedback";
import { ActionButtonContent } from "./action-button-content";

function formatTimestamp(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function TailContent({
  tail,
  loading,
  inMemoryOnly,
}: {
  tail: string[];
  loading: boolean;
  inMemoryOnly: boolean;
}) {
  const { t } = useTranslation();
  if (loading && tail.length === 0) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" /> {t("settings:loadingLog")}
      </div>
    );
  }
  if (tail.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="system-log-tail-empty">
        {t("settings:noRecentLogActivityCapturedYet")}
      </p>
    );
  }
  return (
    <div className="space-y-2">
      {inMemoryOnly && (
        <p className="text-xs text-muted-foreground" data-testid="system-log-tail-source">
          <Trans i18nKey="settings:showingTheInMemoryLogBuffer">
            Showing the in-memory log buffer (last ~2000 entries). Kandev is currently logging to
            the terminal, not to a file - file rotation is disabled. Set{" "}
            <code>logging.outputPath</code> in <code>config.yaml</code> to a file path to enable
            downloadable log files.
          </Trans>
        </p>
      )}
      <pre
        className="max-h-[28rem] overflow-auto rounded-md border bg-muted/30 p-3 text-[11px] leading-relaxed font-mono whitespace-pre"
        data-testid="system-log-tail-content"
      >
        {tail.join("\n")}
      </pre>
    </div>
  );
}

function buildTailFilename(): string {
  // YYYY-MM-DDTHH-MM-SS to mirror lumberjack's rotation naming, minus the
  // colons (which are invalid on Windows). The user can rename freely.
  const stamp = new Date().toISOString().replace(/:/g, "-").replace(/\..+$/, "");
  return `kandev-tail-${stamp}.log`;
}

function downloadTailAsFile(tail: string[]) {
  if (typeof window === "undefined" || tail.length === 0) return;
  const blob = new Blob([tail.join("\n") + "\n"], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = buildTailFilename();
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

function TailHeader({
  tail,
  current,
  refreshState,
  copyState,
  onRefresh,
  onCopy,
}: {
  tail: string[];
  current: ReturnType<typeof useLogFiles>["files"][number] | undefined;
  refreshState: ActionFeedbackState;
  copyState: ActionFeedbackState;
  onRefresh: () => void;
  onCopy: () => void;
}) {
  const { t } = useTranslation();
  const hasTail = tail.length > 0;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={!hasTail || copyState === "pending"}
        onClick={onCopy}
        className="cursor-pointer min-w-[5.5rem] justify-center"
        data-testid="system-log-tail-copy"
        data-state={copyState}
      >
        <ActionButtonContent
          state={copyState}
          idleIcon={<IconCopy className="h-3.5 w-3.5 mr-1" />}
          idleLabel={t("common:copy")}
          pendingLabel={t("settings:copying")}
          successLabel={t("settings:copied")}
        />
      </Button>
      {current ? (
        <Button
          asChild
          variant="outline"
          size="sm"
          className="cursor-pointer"
          data-testid="system-log-current-download"
        >
          <a href={buildLogDownloadUrl(current.name)} download>
            <IconDownload className="h-3.5 w-3.5 mr-1" />
            {t("settings:downloadFile")}
          </a>
        </Button>
      ) : (
        <Button
          variant="outline"
          size="sm"
          disabled={!hasTail}
          onClick={() => downloadTailAsFile(tail)}
          className="cursor-pointer"
          data-testid="system-log-tail-download"
        >
          <IconDownload className="h-3.5 w-3.5 mr-1" />
          {t("settings:downloadTail")}
        </Button>
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={refreshState === "pending"}
        onClick={onRefresh}
        className="cursor-pointer min-w-[6.5rem] justify-center"
        data-testid="system-log-tail-refresh"
        data-state={refreshState}
      >
        <ActionButtonContent
          state={refreshState}
          idleIcon={<IconRefresh className="h-3.5 w-3.5 mr-1" />}
          idleLabel={t("settings:refresh")}
          pendingLabel={t("settings:refreshing")}
          successLabel={t("settings:refreshed")}
        />
      </Button>
    </div>
  );
}

export function LogViewer() {
  const { t } = useTranslation();
  const { files, isLoading: filesLoading } = useLogFiles();
  const { tail, isLoading: tailLoading, reload: reloadTail } = useLogTail(1000);
  const refreshFeedback = useActionFeedback();
  const copyFeedback = useActionFeedback();

  const onRefresh = () =>
    void refreshFeedback.run(async () => {
      await reloadTail();
    });

  const onCopy = () =>
    void copyFeedback.run(async () => {
      if (typeof navigator === "undefined" || !navigator.clipboard) {
        throw new Error("Clipboard API not available");
      }
      await navigator.clipboard.writeText(tail.join("\n") + "\n");
    });

  const current = files.find((f) => f.current);
  const inMemoryOnly = !filesLoading && files.length === 0;

  return (
    <div className="space-y-6">
      <Card data-testid="system-log-tail-card">
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle className="text-base flex items-center gap-2">
            <IconFileText className="h-4 w-4" /> {t("settings:recentLogOutput")}
          </CardTitle>
          <TailHeader
            tail={tail}
            current={current}
            refreshState={refreshFeedback.state}
            copyState={copyFeedback.state}
            onRefresh={onRefresh}
            onCopy={onCopy}
          />
        </CardHeader>
        <CardContent>
          <TailContent tail={tail} loading={tailLoading} inMemoryOnly={inMemoryOnly} />
        </CardContent>
      </Card>

      <LogFilesCard files={files} filesLoading={filesLoading} />
    </div>
  );
}

function LogFilesCard({
  files,
  filesLoading,
}: {
  files: ReturnType<typeof useLogFiles>["files"];
  filesLoading: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Card data-testid="system-log-files-card">
      <CardHeader>
        <CardTitle className="text-base">{t("settings:logFiles")}</CardTitle>
      </CardHeader>
      <CardContent>
        {!files.length && filesLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("settings:loadingFiles")}
          </div>
        )}
        {files.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t("settings:name")}</TableHead>
                <TableHead>{t("settings:kind")}</TableHead>
                <TableHead className="text-right">{t("settings:size")}</TableHead>
                <TableHead>{t("settings:modified")}</TableHead>
                <TableHead className="text-right">{t("settings:actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {files.map((f) => (
                <TableRow key={f.name} data-testid="system-log-file-row" data-name={f.name}>
                  <TableCell className="font-mono text-xs break-all">{f.name}</TableCell>
                  <TableCell>
                    <Badge variant={f.current ? "default" : "secondary"} className="text-[10px]">
                      {f.current ? t("settings:current") : t("settings:rotated")}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-right">{formatBytes(f.size)}</TableCell>
                  <TableCell className="text-xs">{formatTimestamp(f.mtime)}</TableCell>
                  <TableCell className="text-right">
                    <Button
                      asChild
                      size="sm"
                      variant="ghost"
                      className="cursor-pointer"
                      data-testid="system-log-download"
                    >
                      <a href={buildLogDownloadUrl(f.name)} download>
                        <IconDownload className="h-3.5 w-3.5" />
                      </a>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {!filesLoading && files.length === 0 && (
          <p className="text-sm text-muted-foreground" data-testid="system-log-files-empty">
            {t("settings:noLogFilesFoundKandevMay")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
