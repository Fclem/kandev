"use client";

import { useState } from "react";
import { Trans, useLingui } from "@lingui/react/macro";
import { t } from "@lingui/core/macro";
import {
  IconAlertTriangle,
  IconLayoutDashboard,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { LayoutEditor } from "./layout-editor";
import { LayoutProfileList } from "./layout-profile-list";
import { useLayoutSettings } from "./use-layout-settings";

type Controller = ReturnType<typeof useLayoutSettings>;

function defaultActionHelp(selectedSavedDefault: boolean, selectedIsDefault: boolean) {
  if (selectedSavedDefault) {
    return t`Make the original Default layout the starting layout for new tasks.`;
  }
  if (selectedIsDefault) return t`This layout is used as the starting layout for new tasks.`;
  return t`Use this layout as the starting layout for new tasks.`;
}

function LayoutSettingsHeader() {
  return (
    <>
      <div className="min-w-0">
        <h2 className="flex items-center gap-2 text-2xl font-bold">
          <IconLayoutDashboard className="h-5 w-5" />
          <Trans>Layouts</Trans>
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          <Trans>Configure the initial desktop task workbench.</Trans>
        </p>
      </div>
      <Separator />
    </>
  );
}

function ResetBuiltInButton({ onClick }: { onClick: () => void }) {
  const { t } = useLingui();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-8"
          aria-label={t`Reset built-in layout`}
          onClick={onClick}
        >
          <IconRestore className="h-4 w-4" /> <Trans>Reset</Trans>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        <Trans>Restore the original built-in layout and discard its override.</Trans>
      </TooltipContent>
    </Tooltip>
  );
}

function DeleteProfileButton({ onClick }: { onClick: () => void }) {
  const { t } = useLingui();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          className="min-h-11 min-w-11 cursor-pointer sm:min-h-8 sm:min-w-8"
          aria-label={t`Delete layout profile`}
          onClick={onClick}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        <Trans>Delete this custom layout after confirmation.</Trans>
      </TooltipContent>
    </Tooltip>
  );
}

function SelectedLayoutHeader({
  controller,
  onDelete,
}: {
  controller: Controller;
  onDelete: () => void;
}) {
  const { t } = useLingui();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 flex-1">
        {controller.selectedCustom ? (
          <Input
            aria-label={t`Layout profile name`}
            value={controller.selectedCustom.name}
            onChange={(event) => controller.updateSelected({ name: event.target.value })}
            className="min-h-11 max-w-md sm:min-h-9"
          />
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-semibold">{controller.selectedName}</h3>
            <Badge variant="outline">
              <Trans>Built-in</Trans>
            </Badge>
            {controller.selectedBuiltInOverride && (
              <Badge variant="secondary">
                <Trans>Customized</Trans>
              </Badge>
            )}
          </div>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={controller.defaultActionDisabled ? 0 : -1} className="inline-flex">
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-11 cursor-pointer sm:min-h-8"
                disabled={controller.defaultActionDisabled}
                onClick={controller.setDefault}
              >
                {controller.defaultActionLabel}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {defaultActionHelp(controller.selectedSavedDefault, controller.selectedIsDefault)}
          </TooltipContent>
        </Tooltip>
        {controller.selectedBuiltInOverride && (
          <ResetBuiltInButton onClick={controller.resetBuiltIn} />
        )}
        {controller.selectedCustom && <DeleteProfileButton onClick={onDelete} />}
      </div>
    </div>
  );
}

function SelectedLayoutEditor({ controller }: { controller: Controller }) {
  const editorKey = `${controller.selection.kind}:${controller.selection.id}:${controller.editorReset}`;
  if (!controller.editorLayout) {
    return (
      <Alert>
        <IconAlertTriangle className="h-4 w-4" />
        <AlertTitle>
          <Trans>Visual editor unavailable</Trans>
        </AlertTitle>
        <AlertDescription>
          {controller.compatibility?.issues.map((issue) => issue.message).join(". ")}
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <LayoutEditor
      key={editorKey}
      layout={controller.editorLayout}
      editable
      onChange={controller.updateLayout}
    />
  );
}

function DeleteProfileDialog({
  controller,
  open,
  onOpenChange,
}: {
  controller: Controller;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLingui();
  const confirm = () => {
    controller.deleteSelected();
    onOpenChange(false);
  };
  const profileName = controller.selectedCustom?.name ?? t`layout profile`;
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            <Trans>Delete {profileName}?</Trans>
          </AlertDialogTitle>
          <AlertDialogDescription>
            {controller.selectedCustom?.is_default
              ? t`The built-in Default layout will become the default.`
              : t`This profile will be removed when you save.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">
            <Trans>Cancel</Trans>
          </AlertDialogCancel>
          <AlertDialogAction className="cursor-pointer" onClick={confirm}>
            <Trans>Delete</Trans>
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function LayoutSettings() {
  const controller = useLayoutSettings();
  const { t } = useLingui();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const invalidName = controller.profiles.some((profile) => !profile.name.trim());
  useSettingsSaveContributor({
    id: "layout-profiles",
    revision: controller.profilesKey,
    isDirty: controller.isDirty,
    canSave: !invalidName,
    invalidReason: invalidName ? t`Layout profile names must not be empty` : undefined,
    save: controller.save,
    discard: controller.cancel,
  });
  return (
    <div className="min-w-0 space-y-6" data-testid="layout-settings">
      <LayoutSettingsHeader />
      {controller.error && (
        <Alert variant="destructive">
          <IconAlertTriangle className="h-4 w-4" />
          <AlertTitle>
            <Trans>Layout profiles were not saved</Trans>
          </AlertTitle>
          <AlertDescription>{controller.error}</AlertDescription>
        </Alert>
      )}
      <div className="grid min-w-0 gap-5 lg:grid-cols-[16rem_minmax(0,1fr)]">
        <LayoutProfileList
          profiles={controller.profiles}
          selection={controller.selection}
          onSelect={controller.setSelection}
          onCreate={controller.create}
          onDuplicate={controller.duplicate}
        />
        <section className="min-w-0 space-y-3" aria-label={t`${controller.selectedName} editor`}>
          <SelectedLayoutHeader controller={controller} onDelete={() => setDeleteOpen(true)} />
          <SelectedLayoutEditor controller={controller} />
        </section>
      </div>
      <DeleteProfileDialog controller={controller} open={deleteOpen} onOpenChange={setDeleteOpen} />
    </div>
  );
}
