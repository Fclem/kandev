"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { settingsWithDockerAcknowledgement } from "@/hooks/domains/system/use-storage-maintenance";
import type { StorageCapabilities, StorageMaintenanceSettings } from "@/lib/types/system";
import { DedicatedDockerDialog, ExternalGoCacheDialog } from "./storage-confirmation-dialogs";
import { StorageActionButton } from "./storage-action-button";
import { NumberField, PolicySection, SettingRow } from "./storage-policy-fields";
import { StorageSettingHelp } from "./storage-setting-help";
import { bytesToGigabytes, gigabytesToBytes } from "./storage-units";

type Props = {
  settings: StorageMaintenanceSettings;
  savedSettings: StorageMaintenanceSettings;
  capabilities: StorageCapabilities;
  pending: boolean;
  onChange: (settings: StorageMaintenanceSettings) => void;
  onAdopt: (path: string) => Promise<void>;
};

type PolicySectionProps = Pick<
  Props,
  "settings" | "savedSettings" | "capabilities" | "onChange" | "pending"
>;

function settingIsDirty<T>(
  settings: StorageMaintenanceSettings,
  savedSettings: StorageMaintenanceSettings,
  select: (value: StorageMaintenanceSettings) => T,
): boolean {
  return !Object.is(select(settings), select(savedSettings));
}

function ScheduleSection({ settings, savedSettings, pending, onChange }: PolicySectionProps) {
  const { t } = useTranslation();
  const enabledDirty = settingIsDirty(settings, savedSettings, (value) => value.enabled);
  const intervalDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.check_interval_hours,
  );
  const idleDirty = settingIsDirty(settings, savedSettings, (value) => value.idle_for_minutes);
  return (
    <PolicySection
      sectionId="schedule"
      title={t("settings:schedule")}
      description={t("settings:controlsWhenAutomaticMaintenanceIsAllowed")}
      isDirty={enabledDirty || intervalDirty || idleDirty}
    >
      <SettingRow
        title={t("settings:scheduledMaintenance")}
        description={t("settings:periodicallyReclaimDiskSpaceUsingThe")}
        help={t("settings:whenEnabledKandevChecksThisPolicy")}
        control={
          <Switch
            checked={settings.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, enabled })}
            aria-label={t("settings:scheduledMaintenance")}
            data-testid="storage-scheduling-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:checkEveryHours")}
          help={t("settings:howOftenKandevChecksWhetherScheduled")}
          value={settings.check_interval_hours}
          min={1}
          max={168}
          disabled={pending || !settings.enabled}
          onChange={(check_interval_hours) => onChange({ ...settings, check_interval_hours })}
          testId="storage-check-interval"
          isDirty={intervalDirty}
        />
        <NumberField
          label={t("settings:requireIdleForMinutes")}
          help={t("settings:scheduledCleanupStartsOnlyAfterNo")}
          value={settings.idle_for_minutes}
          min={1}
          max={1440}
          disabled={pending || !settings.enabled}
          onChange={(idle_for_minutes) => onChange({ ...settings, idle_for_minutes })}
          testId="storage-idle-period"
          isDirty={idleDirty}
        />
      </div>
    </PolicySection>
  );
}

function WorkspaceSection({ settings, savedSettings, pending, onChange }: PolicySectionProps) {
  const { t } = useTranslation();
  const workspacesDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.workspaces.enabled,
  );
  const graceDirty = settingIsDirty(settings, savedSettings, (value) => value.orphan_grace_hours);
  const containersDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.kandev_containers.enabled,
  );
  return (
    <PolicySection
      sectionId="workspaces"
      title={t("settings:workspacesAndContainers")}
      description={t("settings:reclaimResourcesThatKandevCanPositively")}
      isDirty={workspacesDirty || graceDirty || containersDirty}
    >
      <SettingRow
        title={t("settings:orphanTaskWorkspaces")}
        description={t("settings:moveConfirmedOrphanWorkspacesToQuarantine")}
        help={t("settings:kandevOnlySelectsATaskWorkspace")}
        control={
          <Switch
            checked={settings.workspaces.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, workspaces: { enabled } })}
            aria-label={t("settings:cleanOrphanTaskWorkspaces")}
            data-settings-dirty={workspacesDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:waitBeforeOrphaningHours")}
          help={t("settings:aWorkspaceMustBeUnusedFor")}
          value={settings.orphan_grace_hours}
          min={24}
          max={2160}
          disabled={pending || !settings.workspaces.enabled}
          onChange={(orphan_grace_hours) => onChange({ ...settings, orphan_grace_hours })}
          testId="storage-orphan-grace"
          isDirty={graceDirty}
        />
      </div>
      <SettingRow
        title={t("settings:kandevContainers")}
        description={t("settings:removeStoppedUnusedContainersCreatedAnd")}
        help={t("settings:onlyStoppedContainersLabeledAsKandev")}
        control={
          <Switch
            checked={settings.kandev_containers.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, kandev_containers: { enabled } })}
            aria-label={t("settings:cleanKandevContainers")}
            data-settings-dirty={containersDirty}
          />
        }
      />
    </PolicySection>
  );
}

function AdoptionField({
  path,
  setPath,
  onOpen,
  pending,
  enabled,
}: {
  path: string;
  setPath: (path: string) => void;
  onOpen: () => void;
  pending: boolean;
  enabled: boolean;
}) {
  const { t } = useTranslation();
  let disabledReason: string | undefined;
  if (pending) disabledReason = t("settings:waitForTheCurrentStorageAction");
  else if (!enabled) disabledReason = t("settings:enableTheManagedGoCacheFirst");
  else if (!path.trim()) disabledReason = t("settings:enterAnAbsoluteCachePathFirst");
  return (
    <div className="min-w-0 space-y-2 pt-3">
      <div className="flex items-center gap-1">
        <Label htmlFor="storage-adoption-path">{t("settings:externalGoCache")}</Label>
        <StorageSettingHelp label={t("settings:externalGoCache")}>
          {t("settings:adoptionGivesKandevExplicitPermissionTo")}
        </StorageSettingHelp>
      </div>
      <p className="text-xs text-muted-foreground">
        {t("settings:optionallyAllowKandevToMaintainAn")}
      </p>
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
        <Input
          id="storage-adoption-path"
          value={path}
          disabled={pending || !enabled}
          onChange={(event) => setPath(event.target.value)}
          placeholder="/root/.cache/go-build"
          className="h-11 min-w-0 font-mono"
          data-testid="storage-go-cache-adopt-path"
        />
        <StorageActionButton
          variant="outline"
          disabledReason={disabledReason}
          onClick={onOpen}
          data-testid="storage-go-cache-adopt"
        >
          {t("settings:adoptCache")}
        </StorageActionButton>
      </div>
    </div>
  );
}

function GoCacheSection({
  settings,
  savedSettings,
  capabilities,
  pending,
  onChange,
  adoptionPath,
  setAdoptionPath,
  onOpenAdoption,
}: PolicySectionProps & {
  adoptionPath: string;
  setAdoptionPath: (path: string) => void;
  onOpenAdoption: () => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = settingIsDirty(settings, savedSettings, (value) => value.go_cache.enabled);
  const maxBytesDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.go_cache.max_bytes,
  );
  return (
    <PolicySection
      sectionId="go-cache"
      title={t("settings:goBuildCache")}
      description={t("settings:useAndTrimAKandevOwned")}
      isDirty={enabledDirty || maxBytesDirty}
    >
      <SettingRow
        title={t("settings:managedGoCache")}
        description={t("settings:newHostLocalExecutionsUse", {
          managed_go_cache_path: capabilities.managed_go_cache_path,
        })}
        help={t("settings:whenEnabledKandevGivesNewLocal")}
        control={
          <Switch
            checked={settings.go_cache.enabled}
            disabled={pending}
            onCheckedChange={(enabled) =>
              onChange({ ...settings, go_cache: { ...settings.go_cache, enabled } })
            }
            aria-label={t("settings:enableManagedGoCache")}
            data-testid="storage-go-cache-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:maximumCacheSizeGb")}
          help={t("settings:thisIsACleanupTriggerNot")}
          value={bytesToGigabytes(settings.go_cache.max_bytes)}
          min={1}
          disabled={pending || !settings.go_cache.enabled}
          onChange={(gigabytes) =>
            onChange({
              ...settings,
              go_cache: { ...settings.go_cache, max_bytes: gigabytesToBytes(gigabytes) },
            })
          }
          testId="storage-go-cache-max"
          isDirty={maxBytesDirty}
        />
      </div>
      {capabilities.go_cache_adoption_available && (
        <AdoptionField
          path={adoptionPath}
          setPath={setAdoptionPath}
          pending={pending}
          enabled={settings.go_cache.enabled}
          onOpen={onOpenAdoption}
        />
      )}
    </PolicySection>
  );
}

type DockerSettings = StorageMaintenanceSettings["docker"];

function DockerBuildCacheSettings({
  docker,
  savedDocker,
  disabledReason,
  updateDocker,
}: {
  docker: DockerSettings;
  savedDocker: DockerSettings;
  disabledReason?: string;
  updateDocker: (docker: DockerSettings) => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = docker.build_cache_enabled !== savedDocker.build_cache_enabled;
  const keepBytesDirty = docker.build_cache_keep_bytes !== savedDocker.build_cache_keep_bytes;
  const unusedHoursDirty = docker.build_cache_unused_hours !== savedDocker.build_cache_unused_hours;
  return (
    <>
      <SettingRow
        title={t("settings:dockerBuildCache")}
        description={t("settings:removeOldBuildCacheWhileRetaining")}
        help={t("settings:usesDockerSAgeAndStorage")}
        control={
          <Switch
            checked={docker.build_cache_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(build_cache_enabled) =>
              updateDocker({ ...docker, build_cache_enabled })
            }
            aria-label={t("settings:cleanDockerBuildCache")}
            data-testid="storage-docker-build-cache"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:buildCacheToRetainGb")}
          help={t("settings:dockerKeepsApproximatelyThisMuchBuild")}
          value={bytesToGigabytes(docker.build_cache_keep_bytes)}
          min={1}
          disabled={Boolean(disabledReason) || !docker.build_cache_enabled}
          onChange={(gigabytes) =>
            updateDocker({
              ...docker,
              build_cache_keep_bytes: gigabytesToBytes(gigabytes),
            })
          }
          testId="storage-docker-build-cache-keep-bytes"
          isDirty={keepBytesDirty}
        />
        <NumberField
          label={t("settings:buildCacheMustBeUnusedFor")}
          help={t("settings:onlyBuildCacheRecordsOlderThan")}
          value={docker.build_cache_unused_hours}
          min={24}
          max={2562047}
          disabled={Boolean(disabledReason) || !docker.build_cache_enabled}
          onChange={(build_cache_unused_hours) =>
            updateDocker({ ...docker, build_cache_unused_hours })
          }
          testId="storage-docker-build-cache-unused-hours"
          isDirty={unusedHoursDirty}
        />
      </div>
    </>
  );
}

function DockerImageSettings({
  docker,
  savedDocker,
  disabledReason,
  updateDocker,
}: {
  docker: DockerSettings;
  savedDocker: DockerSettings;
  disabledReason?: string;
  updateDocker: (docker: DockerSettings) => void;
}) {
  const { t } = useTranslation();
  const enabledDirty = docker.unused_images_enabled !== savedDocker.unused_images_enabled;
  const hoursDirty = docker.unused_images_hours !== savedDocker.unused_images_hours;
  return (
    <>
      <SettingRow
        title={t("settings:unusedDockerImages")}
        description={t("settings:removeOldImagesThatNoContainer")}
        help={t("settings:removesAnImageOnlyWhenNo")}
        control={
          <Switch
            checked={docker.unused_images_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(unused_images_enabled) =>
              updateDocker({ ...docker, unused_images_enabled })
            }
            aria-label={t("settings:cleanUnusedDockerImages")}
            data-testid="storage-docker-unused-images"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:imageMustBeUnusedForHours")}
          help={t("settings:anImageMustBeUnusedBy")}
          value={docker.unused_images_hours}
          min={24}
          max={2562047}
          disabled={Boolean(disabledReason) || !docker.unused_images_enabled}
          onChange={(unused_images_hours) => updateDocker({ ...docker, unused_images_hours })}
          testId="storage-docker-unused-images-hours"
          isDirty={hoursDirty}
        />
      </div>
    </>
  );
}

function DockerSection({
  settings,
  savedSettings,
  capabilities,
  pending,
  onChange,
  onOpenDedicated,
}: PolicySectionProps & { onOpenDedicated: () => void }) {
  const { t } = useTranslation();
  const dockerDirty = JSON.stringify(settings.docker) !== JSON.stringify(savedSettings.docker);
  const dedicatedDirty =
    settings.docker.dedicated_daemon_acknowledged !==
    savedSettings.docker.dedicated_daemon_acknowledged;
  const unavailable = capabilities.docker_available
    ? undefined
    : t("settings:dockerIsUnavailableOnTheConfigured");
  const disabledReason =
    (pending ? t("settings:waitForTheCurrentStorageAction") : undefined) ??
    unavailable ??
    (!settings.docker.dedicated_daemon_acknowledged
      ? t("settings:acknowledgeADedicatedDockerDaemonFirst")
      : undefined);
  const updateDocker = (docker: StorageMaintenanceSettings["docker"]) =>
    onChange({ ...settings, docker });
  return (
    <PolicySection
      sectionId="docker"
      title={t("settings:dockerCleanup")}
      description={t("settings:optionalDaemonWideCleanupEnableIt")}
      isDirty={dockerDirty}
    >
      <SettingRow
        title={t("settings:dedicatedDockerDaemon")}
        description={t("settings:confirmThatUnrelatedWorkloadsDoNot")}
        help={t("settings:buildCacheAndImageOwnershipCannot")}
        control={
          <Switch
            checked={settings.docker.dedicated_daemon_acknowledged}
            disabled={pending || !capabilities.docker_available}
            onCheckedChange={(checked) => {
              if (checked) onOpenDedicated();
              else onChange(settingsWithDockerAcknowledgement(settings, false));
            }}
            aria-label={t("settings:dedicatedDockerDaemon")}
            data-testid="storage-docker-dedicated"
            data-settings-dirty={dedicatedDirty}
          />
        }
      />
      {unavailable && (
        <p className="py-2 text-xs text-amber-600">
          {t("settings:dockerIsUnavailableDockerCleanupOptions")}
        </p>
      )}
      <DockerBuildCacheSettings
        docker={settings.docker}
        savedDocker={savedSettings.docker}
        disabledReason={disabledReason}
        updateDocker={updateDocker}
      />
      <DockerImageSettings
        docker={settings.docker}
        savedDocker={savedSettings.docker}
        disabledReason={disabledReason}
        updateDocker={updateDocker}
      />
      {disabledReason && <p className="pt-2 text-xs text-muted-foreground">{disabledReason}</p>}
    </PolicySection>
  );
}

function QuarantineSection({ settings, savedSettings, pending, onChange }: PolicySectionProps) {
  const { t } = useTranslation();
  const retentionDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.quarantine_retention_hours,
  );
  return (
    <PolicySection
      sectionId="quarantine"
      title={t("settings:quarantineSafety")}
      description={t("settings:keepRecoverableResourcesForAGrace")}
      isDirty={retentionDirty}
    >
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label={t("settings:keepQuarantinedItemsForHours")}
          help={t("settings:cleanupFirstMovesOrphanWorkspacesAnd")}
          value={settings.quarantine_retention_hours}
          min={24}
          max={2160}
          disabled={pending}
          onChange={(quarantine_retention_hours) =>
            onChange({ ...settings, quarantine_retention_hours })
          }
          testId="storage-quarantine-retention"
          isDirty={retentionDirty}
        />
      </div>
    </PolicySection>
  );
}

export function StoragePolicyCard({
  settings,
  savedSettings,
  capabilities,
  pending,
  onChange,
  onAdopt,
}: Props) {
  const { t } = useTranslation();
  const [dockerDialogOpen, setDockerDialogOpen] = useState(false);
  const [adoptionDialogOpen, setAdoptionDialogOpen] = useState(false);
  const savedAdoptionPath = savedSettings.go_cache.adopted_path;
  const [adoptionPath, setAdoptionPath] = useState(savedAdoptionPath);
  const previousSavedAdoptionPath = useRef(savedAdoptionPath);

  useEffect(() => {
    const previousPath = previousSavedAdoptionPath.current;
    setAdoptionPath((currentPath) =>
      currentPath === previousPath ? savedAdoptionPath : currentPath,
    );
    previousSavedAdoptionPath.current = savedAdoptionPath;
  }, [savedAdoptionPath]);

  return (
    <section className="min-w-0 space-y-4" data-testid="storage-policy-card">
      <div>
        <h2 className="text-base font-medium">{t("settings:maintenancePolicy")}</h2>
        <p className="text-xs text-muted-foreground">
          {t("settings:chooseWhatKandevMayReclaimAutomatically")}
        </p>
      </div>
      <div className="space-y-3">
        <ScheduleSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
        />
        <WorkspaceSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
        />
        <GoCacheSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
          adoptionPath={adoptionPath}
          setAdoptionPath={setAdoptionPath}
          onOpenAdoption={() => setAdoptionDialogOpen(true)}
        />
        <DockerSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
          onOpenDedicated={() => setDockerDialogOpen(true)}
        />
        <QuarantineSection
          settings={settings}
          savedSettings={savedSettings}
          capabilities={capabilities}
          pending={pending}
          onChange={onChange}
        />
      </div>
      <DedicatedDockerDialog
        open={dockerDialogOpen}
        onOpenChange={setDockerDialogOpen}
        onConfirm={() => {
          const next = settingsWithDockerAcknowledgement(settings, true);
          onChange(next);
          setDockerDialogOpen(false);
        }}
      />
      <ExternalGoCacheDialog
        path={adoptionPath}
        open={adoptionDialogOpen}
        onOpenChange={setAdoptionDialogOpen}
        onConfirm={() => {
          void onAdopt(adoptionPath.trim());
          setAdoptionDialogOpen(false);
        }}
      />
    </section>
  );
}
