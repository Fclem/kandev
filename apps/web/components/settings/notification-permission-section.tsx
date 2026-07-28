import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconBell, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  nativeNotifications,
  type NativeNotificationPermission,
} from "@/lib/desktop/native-notification-client";
import type { PermissionRefresh } from "./notifications-settings-actions";
import { t as globalT } from "@/lib/i18n";

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
  if (permission === "granted") return globalT("common:enabled");
  if (permission === "error") return globalT("common:retry");
  return globalT("settings:enable");
}

export function DesktopNotificationsSection({
  notificationPermission,
  onRequestPermission,
  onRefreshPermission,
  onTestNotification,
}: DesktopNotificationsSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-4">
        <div>
          <div className="text-base font-medium">{t("settings:desktopNotifications")}</div>
          <p className="text-sm text-muted-foreground">
            {t("settings:notifyThisDeviceWhenASelected")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            title={t("settings:enableDesktopNotifications")}
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
                  aria-label={t("settings:refreshNotificationPermission")}
                  className="cursor-pointer"
                  onClick={() => void onRefreshPermission()}
                >
                  <IconRefresh className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("settings:refreshPermissionStatus")}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <HoverCard>
            <HoverCardTrigger asChild>
              <Button
                title={t("settings:sendTestNotification")}
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
              {t("settings:ifYouDoNotSeeNotifications")}
            </HoverCardContent>
          </HoverCard>
        </div>
      </div>

      {notificationPermission === "denied" && (
        <p className="text-sm text-amber-600">
          {nativeNotifications.isAvailable()
            ? t("settings:notificationsAreBlockedInYourOs")
            : t("settings:notificationsAreBlockedInYourBrowser")}
        </p>
      )}
      {notificationPermission === "unsupported" && (
        <p className="text-sm text-amber-600">{t("settings:thisBrowserDoesNotSupportDesktop")}</p>
      )}
      {notificationPermission === "error" && (
        <p className="text-sm text-amber-600">
          {t("settings:kandevCouldNotCheckNotificationPermission")}
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
