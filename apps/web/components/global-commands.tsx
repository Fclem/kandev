"use client";
import { useMemo } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { useTheme } from "@/components/theme/app-theme";
import {
  IconHome,
  IconList,
  IconSettings,
  IconChartBar,
  IconSun,
  IconMoon,
  IconRobot,
  IconCpu,
  IconFolder,
  IconMessageCircle,
  IconSparkles,
  IconBrandGithub,
} from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import { useTranslation } from "react-i18next";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useKeyboardShortcut } from "@/hooks/use-keyboard-shortcut";
import { useAppShortcuts } from "@/hooks/use-app-shortcuts";
import { usePluginShortcuts } from "@/hooks/use-plugin-shortcuts";
import { useAppStore } from "@/components/state-provider";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { getShortcut } from "@/lib/keyboard/shortcut-overrides";
import type { CommandItem } from "@/lib/commands/types";

type PushFn = ReturnType<typeof useRouter>["push"];

function buildNavigationCommands(push: PushFn): CommandItem[] {
  return [
    {
      id: "nav-home",
      label: t("common:goToHome"),
      group: "Navigation",
      icon: <IconHome className="size-3.5" />,
      keywords: ["home", "kanban", "board"],
      action: () => push("/"),
    },
    {
      id: "nav-tasks",
      label: t("common:goToAllTasks"),
      group: "Navigation",
      icon: <IconList className="size-3.5" />,
      keywords: ["tasks", "list", "all"],
      action: () => push("/tasks"),
    },
    {
      id: "nav-settings",
      label: t("common:goToSettings"),
      group: "Navigation",
      icon: <IconSettings className="size-3.5" />,
      keywords: ["settings", "preferences", "config", "general settings"],
      action: () => push("/settings/general"),
    },
    {
      id: "nav-stats",
      label: t("common:goToStats"),
      group: "Navigation",
      icon: <IconChartBar className="size-3.5" />,
      keywords: ["stats", "statistics", "analytics", "metrics"],
      action: () => push("/stats"),
    },
    {
      id: "nav-github",
      label: t("common:goToGithubDashboard"),
      group: "Navigation",
      icon: <IconBrandGithub className="size-3.5" />,
      keywords: ["github", "dashboard", "pr", "pull request", "code review", "issues", "review"],
      action: () => push("/github"),
    },
    {
      id: "settings-agents",
      label: t("common:agentsSettings"),
      group: "Settings",
      icon: <IconRobot className="size-3.5" />,
      keywords: ["agents", "agent settings", "agent profiles", "installed agents", "claude"],
      action: () => push("/settings/agents"),
    },
    {
      id: "settings-executors",
      label: t("common:executorsSettings"),
      group: "Settings",
      icon: <IconCpu className="size-3.5" />,
      keywords: [
        "executors",
        "executor profiles",
        "execution environment",
        "environment variables",
        "runtime",
        "compute",
      ],
      action: () => push("/settings/executors"),
    },
    {
      id: "settings-workspace",
      label: t("common:workspaceSettings"),
      group: "Settings",
      icon: <IconFolder className="size-3.5" />,
      keywords: ["workspace", "workspaces"],
      action: () => push("/settings/workspace"),
    },
    {
      id: "settings-prompts",
      label: t("common:promptsSettings"),
      group: "Settings",
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: [
        "prompts",
        "prompt settings",
        "custom prompts",
        "prompt snippets",
        "prompt templates",
      ],
      action: () => push("/settings/prompts"),
    },
  ];
}

function buildThemeCommand(
  resolvedTheme: string | undefined,
  setTheme: (theme: string) => void,
): CommandItem {
  const isDark = resolvedTheme === "dark";
  const destinationTheme = isDark ? "light" : "dark";
  return {
    id: "pref-theme",
    label: isDark ? t("common:switchToLightMode") : t("common:switchToDarkMode"),
    group: "Preferences",
    icon: isDark ? <IconSun className="size-3.5" /> : <IconMoon className="size-3.5" />,
    keywords: ["theme", "color theme", "appearance"],
    action: () => setTheme(destinationTheme),
  };
}

export function GlobalCommands() {
  const { t } = useTranslation();
  const router = useRouter();
  const { resolvedTheme, setTheme } = useTheme();
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const handleOpenQuickChat = useQuickChatLauncher(activeWorkspaceId);
  const handleOpenConfigChat = useQuickChatLauncher(activeWorkspaceId, "config");

  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const quickChatShortcut = getShortcut("QUICK_CHAT", keyboardShortcuts);

  const quickChatCommand: CommandItem = useMemo(
    () => ({
      id: "quick-chat",
      label: t("common:quickChat"),
      group: "Actions",
      icon: <IconMessageCircle className="size-3.5" />,
      keywords: ["quick chat", "new quick chat", "quick question", "ask agent"],
      shortcut: quickChatShortcut,
      action: handleOpenQuickChat,
    }),
    [handleOpenQuickChat, quickChatShortcut, t],
  );

  const configChatCommand: CommandItem = useMemo(
    () => ({
      id: "config-chat",
      label: t("common:configurationChat"),
      group: "Actions",
      icon: <IconSparkles className="size-3.5" />,
      keywords: [
        "config chat",
        "config mode",
        "configure kandev",
        "workflow configuration",
        "mcp configuration",
      ],
      action: handleOpenConfigChat,
    }),
    [handleOpenConfigChat, t],
  );

  const commands = useMemo<CommandItem[]>(
    () => [
      ...buildNavigationCommands(router.push),
      buildThemeCommand(resolvedTheme, setTheme),
      quickChatCommand,
      configChatCommand,
    ],
    [router.push, resolvedTheme, setTheme, quickChatCommand, configChatCommand],
  );

  useRegisterCommands(commands);
  useKeyboardShortcut(quickChatShortcut, handleOpenQuickChat);
  // Order matters: useAppShortcuts (core) must register its capture-phase
  // keydown listener before usePluginShortcuts so core shortcuts win when a
  // combo matches both — see the precedence note on each hook.
  useAppShortcuts();
  usePluginShortcuts();

  return null;
}
