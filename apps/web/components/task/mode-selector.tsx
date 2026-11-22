"use client";

import {
  forwardRef,
  memo,
  type ComponentPropsWithoutRef,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import type { RefObject } from "react";
import { IconAlertTriangle, IconCheck, IconChevronDown, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { MobilePillButton } from "@/components/task/mobile/mobile-pill-button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { setSessionMode } from "@/lib/api/domains/session-api";
import {
  usePickerConfigurationFeedback,
  type PickerConfigurationFeedbackController,
} from "@/components/task/session-configuration-feedback";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { useSessionCatalogRefresh } from "@/hooks/domains/session/use-session-catalog-refresh";
import { SessionCatalogRefreshConfirmation } from "./session-catalog-refresh-confirmation";
import {
  formatModeName,
  useModeSelectorState,
  type ModeOption,
  type ModeSelectorState,
} from "./mode-selector-state";
import { MobileConfirmationHost } from "@/components/confirmation/mobile-confirmation-host";
import type { MobileConfirmationHostState } from "@/components/confirmation/mobile-confirmation-host";

const THINKING_LEVEL_LABEL = "agents:thinkingLevel";
type ModeSelectorProps = {
  sessionId: string | null;
  triggerClassName?: string;
};

function translateModeLabel(t: ReturnType<typeof useTranslation>["t"], thinking?: boolean) {
  return t(thinking ? THINKING_LEVEL_LABEL : "task:agentPermissionMode");
}

function ModeMismatchWarning({
  requestedName,
  displayName,
  unconfirmed,
}: {
  requestedName: string;
  displayName: string;
  unconfirmed: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="mx-4 mb-2 flex items-start gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-foreground"
      data-testid="session-mode-mismatch-warning"
      role="status"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden />
      <span>
        {unconfirmed
          ? t("task:sessionModeUnconfirmed", { requested: requestedName })
          : t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })}
      </span>
    </div>
  );
}

function MobileModeOption({
  mode,
  selected,
  onSelect,
}: {
  mode: ModeOption;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      data-testid={`session-mode-option-${mode.id}`}
      aria-label={mode.name}
      aria-pressed={selected}
      disabled={mode.disabled}
      className={cn(
        "relative flex min-h-11 w-full cursor-pointer items-center rounded-md border px-3 py-2 pr-10 text-left",
        selected ? "border-primary/50 bg-card font-medium" : "border-transparent hover:bg-muted/60",
      )}
      onClick={onSelect}
    >
      <span className="min-w-0 flex-1">
        <span className="block">{mode.name}</span>
        {mode.description && (
          <span className="block text-xs text-muted-foreground">{mode.description}</span>
        )}
      </span>
      {selected && <IconCheck className="absolute right-3 h-4 w-4" aria-hidden />}
    </button>
  );
}

function MobileModeOptions({
  modeState,
  onModeChange,
  onClose,
  onRefreshRequired,
}: {
  modeState: ModeSelectorState;
  onModeChange: (modeId: string) => Promise<void>;
  onClose: () => void;
  onRefreshRequired: () => void;
}) {
  return (
    <div className="space-y-1">
      {modeState.availableModes.map((mode) => (
        <MobileModeOption
          key={mode.id}
          mode={mode}
          selected={mode.id === modeState.currentModeId}
          onSelect={() => {
            if (mode.disabled) return;
            if (mode.requiresRefresh) {
              onRefreshRequired();
              return;
            }
            void onModeChange(mode.id);
            onClose();
          }}
        />
      ))}
    </div>
  );
}

type ModePresentationProps = {
  modeState: ModeSelectorState;
  displayName: string;
  requestedName?: string;
  triggerClassName?: string;
  onModeChange: (modeId: string) => Promise<void>;
  onRefreshRequired: () => void;
  anchorRef: RefObject<HTMLButtonElement | null>;
};

function MobileModeSelector({
  modeState,
  displayName,
  requestedName,
  triggerClassName,
  onModeChange,
  onRefreshRequired,
  anchorRef,
  open,
  onOpenChange,
  confirmationState,
}: ModePresentationProps & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  confirmationState: MobileConfirmationHostState;
}) {
  const { t } = useTranslation();

  return (
    <>
      <MobilePillButton
        ref={anchorRef}
        icon={
          requestedName ? (
            <IconAlertTriangle className="h-3 w-3 shrink-0 text-amber-500" aria-hidden />
          ) : undefined
        }
        label={displayName}
        ariaLabel={displayName}
        isOpen={open}
        onClick={() => onOpenChange(true)}
        data-testid="session-mode-selector"
        className={cn("h-11 min-h-11 min-w-11", triggerClassName)}
      />
      <MobilePickerSheet
        open={open}
        onOpenChange={onOpenChange}
        title={translateModeLabel(t, modeState.thinking)}
        description={t(modeState.thinking ? THINKING_LEVEL_LABEL : "task:availableModes")}
        contentTestId="session-mode-picker-scroll"
        confirmationState={confirmationState}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          anchorRef.current?.focus({ preventScroll: true });
        }}
        fixedContent={
          requestedName ? (
            <ModeMismatchWarning
              requestedName={requestedName}
              displayName={displayName}
              unconfirmed={!modeState.currentModeId}
            />
          ) : undefined
        }
        headerAction={
          <Button
            variant="ghost"
            size="icon"
            className="h-12 w-12 shrink-0"
            aria-label={t("common:close")}
            data-testid="session-mode-mobile-picker-close"
            onClick={() => onOpenChange(false)}
          >
            <IconX className="h-4 w-4" aria-hidden />
          </Button>
        }
      >
        <MobileModeOptions
          modeState={modeState}
          onModeChange={onModeChange}
          onClose={() => onOpenChange(false)}
          onRefreshRequired={onRefreshRequired}
        />
      </MobilePickerSheet>
    </>
  );
}

// Rendered under DropdownMenuTrigger/TooltipTrigger `asChild`, so it must
// forward the ref and spread the props Radix attaches; swallowing them leaves
// the button inert.
const ModeSelectorTrigger = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<typeof Button> & {
    displayName: string;
    notApplied: boolean;
    triggerClassName?: string;
  }
>(function ModeSelectorTrigger({ displayName, notApplied, triggerClassName, ...props }, ref) {
  return (
    <Button
      {...props}
      ref={ref}
      variant="ghost"
      size="sm"
      data-testid="session-mode-selector"
      className={cn(
        "h-7 min-w-0 gap-1 overflow-hidden px-2 cursor-pointer whitespace-nowrap hover:bg-muted/40 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11",
        triggerClassName,
      )}
    >
      {notApplied && (
        <IconAlertTriangle
          className="h-3 w-3 text-amber-500 shrink-0"
          data-testid="session-mode-not-applied"
          aria-hidden
        />
      )}
      <span className="truncate text-xs">{displayName}</span>
      <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
    </Button>
  );
});

function useModeSelectorDropdownState() {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);

  const handleDropdownOpenChange = useCallback((open: boolean) => {
    setDropdownOpen(open);
    if (!open) {
      recentlyClosedRef.current = true;
      setTooltipOpen(false);
      setTimeout(() => {
        recentlyClosedRef.current = false;
      }, 200);
    }
  }, []);

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (dropdownOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [dropdownOpen],
  );

  return {
    dropdownOpen,
    tooltipOpen,
    handleDropdownOpenChange,
    handleTooltipOpenChange,
  };
}

function useModeChangeHandler({
  sessionId,
  modeState,
  onRefreshRequired,
  feedback,
  label,
}: {
  sessionId: string | null;
  modeState: ModeSelectorState | undefined;
  onRefreshRequired: () => void;
  feedback: PickerConfigurationFeedbackController;
  label: string;
}) {
  return useCallback(
    async (modeId: string) => {
      const mode = modeState?.availableModes.find((item) => item.id === modeId);
      if (!sessionId || !mode || mode.disabled) return;
      if (mode.requiresRefresh) {
        onRefreshRequired();
        return;
      }
      const feedbackId = feedback.begin();
      const change = { configId: "mode", label, requested: modeId };
      try {
        const response = await setSessionMode(sessionId, modeId);
        feedback.showChanges(feedbackId, [{ ...change, result: response.configuration_result }]);
      } catch (error) {
        feedback.showChanges(feedbackId, [{ ...change, error }]);
      }
    },
    [feedback, label, modeState, onRefreshRequired, sessionId],
  );
}

function resolveModeTooltipLabel({
  modeState,
  requestedName,
  displayName,
  defaultLabel,
  t,
}: {
  modeState: ModeSelectorState;
  requestedName?: string;
  displayName: string;
  defaultLabel: string;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  if (!requestedName) return defaultLabel;
  return modeState.currentModeId
    ? t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })
    : t("task:sessionModeUnconfirmed", { requested: requestedName });
}

function resolveModeDisplayNames(
  modeState: ModeSelectorState,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const currentMode = modeState.availableModes.find((mode) => mode.id === modeState.currentModeId);
  const displayName = currentMode?.name || modeState.currentModeId || t("common:unknown");
  // Show a request name only when the agent did not reach the requested mode.
  const requestedName = modeState.requestedModeId
    ? (modeState.availableModes.find((mode) => mode.id === modeState.requestedModeId)?.name ??
      formatModeName(modeState.requestedModeId))
    : undefined;
  return { displayName, requestedName };
}

function DesktopModeSelector({
  modeState,
  displayName,
  requestedName,
  triggerClassName,
  tooltipLabel,
  dropdownOpen,
  tooltipOpen,
  onDropdownOpenChange,
  onTooltipOpenChange,
  onModeChange,
  anchorRef,
}: {
  modeState: ModeSelectorState;
  displayName: string;
  requestedName?: string;
  triggerClassName?: string;
  tooltipLabel: string;
  dropdownOpen: boolean;
  tooltipOpen: boolean;
  onDropdownOpenChange: (open: boolean) => void;
  onTooltipOpenChange: (open: boolean) => void;
  onModeChange: (modeId: string) => void;
  anchorRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu open={dropdownOpen} onOpenChange={onDropdownOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={onTooltipOpenChange}>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <ModeSelectorTrigger
              ref={anchorRef}
              displayName={displayName}
              notApplied={Boolean(requestedName)}
              triggerClassName={triggerClassName}
            />
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="top">{tooltipLabel}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="start" side="top" className="min-w-[280px]">
        <DropdownMenuLabel>
          {t(modeState.thinking ? THINKING_LEVEL_LABEL : "task:availableModes")}
        </DropdownMenuLabel>
        {modeState.availableModes.map((mode) => (
          <DropdownMenuItem
            key={mode.id}
            disabled={mode.disabled}
            onClick={() => onModeChange(mode.id)}
            className={cn(
              "relative min-h-11 cursor-pointer border border-transparent pr-7 sm:min-h-8",
              mode.id === modeState.currentModeId &&
                "border-primary/50 bg-card font-medium hover:bg-card",
            )}
          >
            <div className="min-w-0 flex-1">
              <div>{mode.name}</div>
              {mode.description && (
                <div className="text-xs text-muted-foreground">{mode.description}</div>
              )}
            </div>
            {mode.id === modeState.currentModeId && (
              <IconCheck className="absolute right-2 h-4 w-4" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export const ModeSelector = memo(function ModeSelector({
  sessionId,
  triggerClassName,
}: ModeSelectorProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const feedback = usePickerConfigurationFeedback(sessionId, "mode");
  const refresh = useSessionCatalogRefresh(sessionId);
  const modeState = useModeSelectorState(sessionId, refresh.entry, refresh.sourceOwner);
  const [mobileOpen, setMobileOpen] = useState(false);
  const { dropdownOpen, tooltipOpen, handleDropdownOpenChange, handleTooltipOpenChange } =
    useModeSelectorDropdownState();

  useEffect(() => {
    if (!sessionId || !modeState) {
      handleDropdownOpenChange(false);
    }
  }, [handleDropdownOpenChange, modeState, sessionId]);

  const handleModeChange = useModeChangeHandler({
    sessionId,
    modeState,
    onRefreshRequired: refresh.requestRefresh,
    feedback,
    label: translateModeLabel(t, modeState?.thinking),
  });

  if (!sessionId || !modeState) {
    return null;
  }

  const { displayName, requestedName } = resolveModeDisplayNames(modeState, t);
  const tooltipLabel = resolveModeTooltipLabel({
    modeState,
    requestedName,
    displayName,
    defaultLabel: translateModeLabel(t, modeState.thinking),
    t,
  });
  const mobile = isMobile || !isFinePointer;
  return (
    <MobileConfirmationHost open={mobile && mobileOpen} surface="drawer">
      {(confirmationState) => (
        <>
          {mobile ? (
            <MobileModeSelector
              modeState={modeState}
              displayName={displayName}
              requestedName={requestedName}
              triggerClassName={triggerClassName}
              onModeChange={handleModeChange}
              onRefreshRequired={refresh.requestRefresh}
              anchorRef={refresh.anchorRef}
              open={mobileOpen}
              onOpenChange={setMobileOpen}
              confirmationState={confirmationState}
            />
          ) : (
            <DesktopModeSelector
              modeState={modeState}
              displayName={displayName}
              requestedName={requestedName}
              triggerClassName={triggerClassName}
              tooltipLabel={tooltipLabel}
              dropdownOpen={dropdownOpen}
              tooltipOpen={tooltipOpen}
              onDropdownOpenChange={handleDropdownOpenChange}
              onTooltipOpenChange={handleTooltipOpenChange}
              onModeChange={handleModeChange}
              anchorRef={refresh.anchorRef}
            />
          )}
          <SessionCatalogRefreshConfirmation
            target={refresh.target}
            anchorRef={refresh.anchorRef}
            onDismiss={refresh.dismiss}
            onConfirm={refresh.confirm}
          />
        </>
      )}
    </MobileConfirmationHost>
  );
});
