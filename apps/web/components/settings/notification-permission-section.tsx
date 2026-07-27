import { useCallback, useEffect, useState } from "react";
import { Trans, useLingui } from "@lingui/react/macro";
import { t as globalT } from "@lingui/core/macro";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  nativeNotifications,
  type NativeNotificationPermission,
} from "@/lib/desktop/native-notification-client";
import type { PermissionRefresh } from "./notifications-settings-actions";

type NotificationPermissionState =
  | NativeNotificationPermission
  | NotificationPermission
  | "unsupported"
  | "error";

type DesktopNotificationsSectionProps = {
  notificationPermission: NotificationPermissionState;
  onRequestPermission: () => Promise<void>;
  onRefreshPermission: () => Promise<void>;
  onTestNotification: () => void;
};

function permissionActionLabel(permission: NotificationPermissionState) {
  if (permission === "granted") return globalT`Enabled`;
  if (permission === "error") return globalT`Retry`;
  return globalT`Enable`;
}

export function DesktopNotificationsSection({
  notificationPermission,
  onRequestPermission,
  onRefreshPermission,
  onTestNotification,
}: DesktopNotificationsSectionProps) {
  const { t } = useLingui();
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-base font-medium">
            <Trans>Desktop Notifications</Trans>
          </div>
          <p className="text-sm text-muted-foreground">
            <Trans>
              Notify this device when a selected agent turn, question, or Office event occurs.
            </Trans>
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            title={t`Enable desktop notifications`}
            variant="default"
            size="sm"
            onClick={() => void onRequestPermission()}
            disabled={
              notificationPermission === "granted" || notificationPermission === "unsupported"
            }
            className={
              notificationPermission === "granted"
                ? "bg-emerald-500 text-white hover:bg-emerald-500"
                : "cursor-pointer"
            }
          >
            {permissionActionLabel(notificationPermission)}
          </Button>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t`Refresh notification permission`}
                  className="cursor-pointer"
                  onClick={() => void onRefreshPermission()}
                >
                  <IconRefresh className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <Trans>Refresh permission status</Trans>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <HoverCard>
            <HoverCardTrigger asChild>
              <Button
                title={t`Send test notification`}
                variant="outline"
                className="cursor-pointer"
                size="icon"
                onClick={() => {
                  void onTestNotification();
                }}
              >
                <IconBell className="h-4 w-4" />
              </Button>
            </HoverCardTrigger>
            <HoverCardContent side="top" className="text-sm">
              <Trans>
                If you do not see notifications, check your OS settings and allow this browser.
              </Trans>
            </HoverCardContent>
          </HoverCard>
        </div>
      </div>

      {notificationPermission === "denied" && (
        <p className="text-sm text-amber-600">
          {nativeNotifications.isAvailable()
            ? t`Notifications are blocked in your OS app notification settings. Enable them there, then click Refresh.`
            : t`Notifications are blocked in your browser. Enable them in site settings, then click Refresh.`}
        </p>
      )}
      {notificationPermission === "unsupported" && (
        <p className="text-sm text-amber-600">
          <Trans>This browser does not support desktop notifications.</Trans>
        </p>
      )}
      {notificationPermission === "error" && (
        <p className="text-sm text-amber-600">
          <Trans>
            Kandev could not check notification permission. Try again, then check your browser or OS
            notification settings.
          </Trans>
        </p>
      )}
    </div>
  );
}

export function useNotificationPermission() {
  const [notificationPermission, setNotificationPermission] =
    useState<NotificationPermissionState>("default");
  const refreshPermission = useCallback<PermissionRefresh>(async (error) => {
    if (error) {
      setNotificationPermission("error");
      return;
    }
    try {
      if (nativeNotifications.isAvailable()) {
        setNotificationPermission(await nativeNotifications.permission.get());
        return;
      }
      setNotificationPermission(
        typeof Notification === "undefined" ? "unsupported" : Notification.permission,
      );
    } catch {
      setNotificationPermission("error");
    }
  }, []);
  useEffect(() => {
    void refreshPermission();
  }, [refreshPermission]);
  return { notificationPermission, refreshPermission };
}
