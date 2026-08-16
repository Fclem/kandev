"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Badge } from "@kandev/ui/badge";
import { IconDevices, IconKey } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import {
  changePassword,
  listSessions,
  revokeSession,
  type AuthSession,
} from "@/lib/api/domains/auth-api";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsTargetRegistration } from "@/components/settings/settings-target-provider";
import { ACCOUNT_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/account";
import { formatDateTime, formatRelativeTime } from "@/lib/i18n/formats";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsErrorText, SettingsFieldLabel } from "@/components/settings/settings-typography";
import { useNow } from "@/hooks/use-now";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { toast } from "@/lib/toast/sonner";
import type { LastSeenDisplay } from "@/lib/types/http";

const syncLastSeenDisplay = createQueuedUserSettingsSyncWithResponse<LastSeenDisplay>(
  (lastSeenDisplay) => ({ last_seen_display: lastSeenDisplay }),
);

function ChangePasswordCard() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);
    setSubmitting(true);
    try {
      await changePassword({ current_password: current, new_password: next });
      setCurrent("");
      setNext("");
      setSuccess(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("account:couldNotChangePassword"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <SettingsCard
      discoveryTargetId={ACCOUNT_SETTINGS_TARGETS.password}
      data-testid="account-security-password-card"
    >
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconKey className="h-4 w-4" /> {t("account:password")}
          </span>
        }
      />
      <CardContent>
        <form className="flex flex-col gap-3 max-w-sm" onSubmit={(e) => void onSubmit(e)}>
          <div className="flex flex-col gap-1">
            <SettingsFieldLabel htmlFor="account-current-password">
              {t("account:currentPassword")}
            </SettingsFieldLabel>
            <Input
              id="account-current-password"
              data-testid="account-current-password"
              type="password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1">
            <SettingsFieldLabel htmlFor="account-new-password">
              {t("account:newPassword")}
            </SettingsFieldLabel>
            <Input
              id="account-new-password"
              data-testid="account-new-password"
              type="password"
              minLength={8}
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </div>
          {error && (
            <SettingsErrorText data-testid="account-password-error">{error}</SettingsErrorText>
          )}
          {success && (
            <p className="text-xs text-muted-foreground" data-testid="account-password-success">
              {t("account:passwordUpdated")}
            </p>
          )}
          <Button
            type="submit"
            className="cursor-pointer self-start"
            disabled={submitting}
            data-testid="account-password-submit"
          >
            {submitting ? t("account:saving") : t("account:changePassword")}
          </Button>
        </form>
      </CardContent>
    </SettingsCard>
  );
}

function useSessionsList() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const res = await listSessions({ cache: "no-store" });
      setSessions(res.sessions);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("account:failedToLoadSessions"));
    }
  }, [t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { sessions, loaded, error, reload };
}

function parseLastSeenAt(lastSeenAt: string): Date | null {
  const parsed = new Date(lastSeenAt);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

function LastSeenCell({ lastSeenAt, display }: { lastSeenAt: string; display: LastSeenDisplay }) {
  const absolute = useMemo(() => parseLastSeenAt(lastSeenAt), [lastSeenAt]);

  if (!absolute) {
    // Guard formatDateTime, which throws RangeError on invalid dates.
    return <TableCell className="text-xs" data-testid="last-seen-empty" />;
  }
  if (display === "absolute") {
    return <TableCell className="text-xs">{formatDateTime(absolute)}</TableCell>;
  }
  return <RelativeLastSeenCell lastSeenAt={absolute} />;
}

function RelativeLastSeenCell({ lastSeenAt }: { lastSeenAt: Date }) {
  const now = useNow(30_000);
  const absolute = formatDateTime(lastSeenAt);
  return (
    <TableCell className="text-xs">
      <Tooltip>
        <TooltipTrigger asChild>
          <time
            dateTime={lastSeenAt.toISOString()}
            tabIndex={0}
            title={absolute}
            aria-label={absolute}
            className="cursor-default rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="last-seen-relative"
          >
            {formatRelativeTime(lastSeenAt, now)}
          </time>
        </TooltipTrigger>
        <TooltipContent>{absolute}</TooltipContent>
      </Tooltip>
    </TableCell>
  );
}

function LastSeenDisplaySelect({
  value,
  onChange,
}: {
  value: LastSeenDisplay;
  onChange: (value: LastSeenDisplay) => void;
}) {
  const { t } = useTranslation();
  const registerTarget = useSettingsTargetRegistration(ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay);
  return (
    <div ref={registerTarget} className="space-y-2" data-testid="last-seen-display-control">
      <Label htmlFor="last-seen-display">{t("account:lastSeenDisplay")}</Label>
      <Select value={value} onValueChange={(next) => onChange(next as LastSeenDisplay)}>
        <SelectTrigger
          id="last-seen-display"
          data-testid="last-seen-display-select"
          className="min-h-11 cursor-pointer"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="absolute" className="min-h-11">
            {t("account:absoluteTime")}
          </SelectItem>
          <SelectItem value="relative" className="min-h-11">
            {t("account:relativeTime")}
          </SelectItem>
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("account:lastSeenDisplayDescription")}</p>
    </div>
  );
}

function SessionsTable({
  sessions,
  display,
  onRevoke,
}: {
  sessions: AuthSession[];
  display: LastSeenDisplay;
  onRevoke: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Table data-testid="account-sessions-table">
      <TableHeader>
        <TableRow>
          <TableHead>{t("account:device")}</TableHead>
          <TableHead>{t("account:ipAddress")}</TableHead>
          <TableHead>{t("account:lastSeen")}</TableHead>
          <TableHead className="text-right">{t("account:actions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sessions.map((session) => (
          <TableRow key={session.id} data-testid="account-sessions-row">
            <TableCell className="text-xs">
              {session.user_agent}
              {session.current && (
                <Badge variant="default" className="ml-2 text-[10px]">
                  {t("account:thisDevice")}
                </Badge>
              )}
            </TableCell>
            <TableCell className="text-xs">{session.ip}</TableCell>
            <LastSeenCell lastSeenAt={session.last_seen_at} display={display} />
            <TableCell className="text-right">
              {!session.current && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="cursor-pointer text-destructive"
                  onClick={() => void onRevoke(session.id)}
                  data-testid="account-sessions-revoke"
                >
                  {t("account:signOut")}
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function SessionsCard() {
  const { t } = useTranslation();
  const { sessions, loaded, error, reload } = useSessionsList();
  const savedDisplay = useAppStore((state) => state.userSettings.lastSeenDisplay);
  const store = useAppStoreApi();
  const [optimisticDisplay, setOptimisticDisplay] = useState<LastSeenDisplay | null>(null);
  const latestRevision = useRef(0);
  const display = optimisticDisplay ?? savedDisplay;

  const onDisplayChange = useCallback(
    (next: LastSeenDisplay) => {
      const revision = ++latestRevision.current;
      setOptimisticDisplay(next);
      void syncLastSeenDisplay(next)
        .then((response) => {
          const state = store.getState();
          state.setUserSettings(mapUserSettingsResponse(response, state.userSettings));
          if (latestRevision.current === revision) setOptimisticDisplay(null);
        })
        .catch(() => {
          if (latestRevision.current !== revision) return;
          setOptimisticDisplay(null);
          toast.error(t("account:failedToSaveLastSeenDisplay"));
        });
    },
    [store, t],
  );

  const onRevoke = async (id: string) => {
    await revokeSession(id);
    await reload();
  };

  return (
    <SettingsCard
      discoveryTargetId={ACCOUNT_SETTINGS_TARGETS.sessions}
      data-testid="account-sessions-card"
    >
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconDevices className="h-4 w-4" /> {t("account:activeSessions")}
          </span>
        }
      />
      <CardContent className="space-y-3">
        {/* The display preference is independent of session rows, so the
            select (and its discovery target) stays reachable during loading
            and when the sessions API fails. */}
        <LastSeenDisplaySelect value={display} onChange={onDisplayChange} />
        {error && (
          <SettingsErrorText data-testid="account-sessions-error">{error}</SettingsErrorText>
        )}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("account:loadingSessions")}
          </div>
        )}
        {loaded && sessions.length > 0 && (
          <SessionsTable sessions={sessions} display={display} onRevoke={onRevoke} />
        )}
      </CardContent>
    </SettingsCard>
  );
}

export function SecuritySettings() {
  return (
    <div className="space-y-4">
      <ChangePasswordCard />
      <SessionsCard />
    </div>
  );
}
