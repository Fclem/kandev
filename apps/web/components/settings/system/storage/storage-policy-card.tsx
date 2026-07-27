"use client";

import { useEffect, useRef, useState } from "react";
import { Trans, useLingui } from "@lingui/react/macro";
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
  const { t } = useLingui();
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
      title={t`Schedule`}
      description={t`Controls when automatic maintenance is allowed to start. Manual actions remain available when scheduling is off.`}
      isDirty={enabledDirty || intervalDirty || idleDirty}
    >
      <SettingRow
        title={t`Scheduled maintenance`}
        description={t`Periodically reclaim disk space using the enabled resource rules.`}
        help={t`When enabled, Kandev checks this policy at the configured interval and starts only after Kandev has been idle for the required period. Turning it off does not disable Analyze or Run now.`}
        control={
          <Switch
            checked={settings.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, enabled })}
            aria-label={t`Scheduled maintenance`}
            data-testid="storage-scheduling-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t`Check every (hours)`}
          help={t`How often Kandev checks whether scheduled maintenance is due. A check that finds Kandev busy is skipped and tried again at the next interval.`}
          value={settings.check_interval_hours}
          min={1}
          max={168}
          disabled={pending || !settings.enabled}
          onChange={(check_interval_hours) => onChange({ ...settings, check_interval_hours })}
          testId="storage-check-interval"
          isDirty={intervalDirty}
        />
        <NumberField
          label={t`Require idle for (minutes)`}
          help={t`Scheduled cleanup starts only after no task, shell command, test, setup, cleanup, or image build has used managed resources for this long. Run now does not wait for this timer, but it still refuses to run while resources are active.`}
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
  const { t } = useLingui();
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
      title={t`Workspaces and containers`}
      description={t`Reclaim resources that Kandev can positively identify as no longer in use.`}
      isDirty={workspacesDirty || graceDirty || containersDirty}
    >
      <SettingRow
        title={t`Orphan task workspaces`}
        description={t`Move confirmed orphan workspaces to quarantine.`}
        help={t`Kandev only selects a task workspace after inventory confirms that no active task, environment, session, or protected worktree uses it. The workspace is moved to quarantine first, where it can be restored before permanent deletion.`}
        control={
          <Switch
            checked={settings.workspaces.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, workspaces: { enabled } })}
            aria-label={t`Clean orphan task workspaces`}
            data-settings-dirty={workspacesDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t`Wait before orphaning (hours)`}
          help={t`A workspace must be unused for at least this long before it can be classified as an orphan. Increasing this value keeps old workspaces longer before they enter quarantine.`}
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
        title={t`Kandev containers`}
        description={t`Remove stopped, unused containers created and labeled by Kandev.`}
        help={t`Only stopped containers labeled as Kandev-managed are considered, and inventory must confirm they are no longer needed. Running containers and unrelated Docker containers are never removed by this option.`}
        control={
          <Switch
            checked={settings.kandev_containers.enabled}
            disabled={pending}
            onCheckedChange={(enabled) => onChange({ ...settings, kandev_containers: { enabled } })}
            aria-label={t`Clean Kandev containers`}
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
  const { t } = useLingui();
  let disabledReason: string | undefined;
  if (pending) disabledReason = t`Wait for the current storage action to finish.`;
  else if (!enabled) disabledReason = t`Enable the managed Go cache first.`;
  else if (!path.trim()) disabledReason = t`Enter an absolute cache path first.`;
  return (
    <div className="min-w-0 space-y-2 pt-3">
      <div className="flex items-center gap-1">
        <Label htmlFor="storage-adoption-path">
          <Trans>External Go cache</Trans>
        </Label>
        <StorageSettingHelp label={t`External Go cache`}>
          {t`Adoption gives Kandev explicit permission to rotate this existing cache. Kandev never cleans a default user cache or another path unless you adopt that exact absolute path and confirm the destructive access.`}
        </StorageSettingHelp>
      </div>
      <p className="text-xs text-muted-foreground">
        <Trans>
          Optionally allow Kandev to maintain an existing host cache outside its managed path.
        </Trans>
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
          <Trans>Adopt cache</Trans>
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
  const { t } = useLingui();
  const enabledDirty = settingIsDirty(settings, savedSettings, (value) => value.go_cache.enabled);
  const maxBytesDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.go_cache.max_bytes,
  );
  return (
    <PolicySection
      sectionId="go-cache"
      title={t`Go build cache`}
      description={t`Use and trim a Kandev-owned cache for new host-local Go executions.`}
      isDirty={enabledDirty || maxBytesDirty}
    >
      <SettingRow
        title={t`Managed Go cache`}
        description={t`New host-local executions use ${capabilities.managed_go_cache_path}.`}
        help={t`When enabled, Kandev gives new local task processes a dedicated Go build cache and may rotate it during maintenance when it exceeds the maximum. Turning this off stops using the managed cache for new executions but does not delete it.`}
        control={
          <Switch
            checked={settings.go_cache.enabled}
            disabled={pending}
            onCheckedChange={(enabled) =>
              onChange({ ...settings, go_cache: { ...settings.go_cache, enabled } })
            }
            aria-label={t`Enable managed Go cache`}
            data-testid="storage-go-cache-enabled"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t`Maximum cache size (GB)`}
          help={t`This is a cleanup trigger, not a hard quota. The cache may grow past this size while tasks are active. Maintenance rotates the owned cache into quarantine and recreates an empty cache after the limit is exceeded.`}
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
  const { t } = useLingui();
  const enabledDirty = docker.build_cache_enabled !== savedDocker.build_cache_enabled;
  const keepBytesDirty = docker.build_cache_keep_bytes !== savedDocker.build_cache_keep_bytes;
  const unusedHoursDirty = docker.build_cache_unused_hours !== savedDocker.build_cache_unused_hours;
  return (
    <>
      <SettingRow
        title={t`Docker build cache`}
        description={t`Remove old build cache while retaining the configured amount.`}
        help={t`Uses Docker's age and storage filters to remove old build cache. It does not run docker system prune, does not remove volumes globally, and remains disabled until the dedicated-daemon acknowledgment is confirmed.`}
        control={
          <Switch
            checked={docker.build_cache_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(build_cache_enabled) =>
              updateDocker({ ...docker, build_cache_enabled })
            }
            aria-label={t`Clean Docker build cache`}
            data-testid="storage-docker-build-cache"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 py-3 sm:grid-cols-2">
        <NumberField
          label={t`Build cache to retain (GB)`}
          help={t`Docker keeps approximately this much build cache when pruning eligible records. A larger value preserves more reusable build layers but reclaims less disk space.`}
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
          label={t`Build cache must be unused for (hours)`}
          help={t`Only build cache records older than this unused-age threshold are eligible. Increasing it protects recent build layers for longer.`}
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
  const { t } = useLingui();
  const enabledDirty = docker.unused_images_enabled !== savedDocker.unused_images_enabled;
  const hoursDirty = docker.unused_images_hours !== savedDocker.unused_images_hours;
  return (
    <>
      <SettingRow
        title={t`Unused Docker images`}
        description={t`Remove old images that no container uses.`}
        help={t`Removes an image only when no running or stopped container references it and it is older than the configured age. This is daemon-wide and therefore requires the dedicated-daemon acknowledgment.`}
        control={
          <Switch
            checked={docker.unused_images_enabled}
            disabled={Boolean(disabledReason)}
            onCheckedChange={(unused_images_enabled) =>
              updateDocker({ ...docker, unused_images_enabled })
            }
            aria-label={t`Clean unused Docker images`}
            data-testid="storage-docker-unused-images"
            data-settings-dirty={enabledDirty}
          />
        }
      />
      <div className="grid min-w-0 grid-cols-1 gap-3 pt-3 sm:grid-cols-2">
        <NumberField
          label={t`Image must be unused for (hours)`}
          help={t`An image must be unused by every container and older than this age before Kandev can remove it. Increasing the value keeps old images available for longer.`}
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
  const { t } = useLingui();
  const dockerDirty = JSON.stringify(settings.docker) !== JSON.stringify(savedSettings.docker);
  const dedicatedDirty =
    settings.docker.dedicated_daemon_acknowledged !==
    savedSettings.docker.dedicated_daemon_acknowledged;
  const unavailable = capabilities.docker_available
    ? undefined
    : t`Docker is unavailable on the configured host.`;
  const disabledReason =
    (pending ? t`Wait for the current storage action to finish.` : undefined) ??
    unavailable ??
    (!settings.docker.dedicated_daemon_acknowledged
      ? t`Acknowledge a dedicated Docker daemon first.`
      : undefined);
  const updateDocker = (docker: StorageMaintenanceSettings["docker"]) =>
    onChange({ ...settings, docker });
  return (
    <PolicySection
      sectionId="docker"
      title={t`Docker cleanup`}
      description={t`Optional daemon-wide cleanup. Enable it only when this Docker daemon is dedicated to Kandev.`}
      isDirty={dockerDirty}
    >
      <SettingRow
        title={t`Dedicated Docker daemon`}
        description={t`Confirm that unrelated workloads do not share this Docker daemon.`}
        help={t`Build cache and image ownership cannot be attributed reliably to Kandev. This acknowledgment unlocks daemon-wide cleanup and should only be enabled when the configured Docker daemon is used exclusively by Kandev. Kandev never performs a volume-wide prune.`}
        control={
          <Switch
            checked={settings.docker.dedicated_daemon_acknowledged}
            disabled={pending || !capabilities.docker_available}
            onCheckedChange={(checked) => {
              if (checked) onOpenDedicated();
              else onChange(settingsWithDockerAcknowledgement(settings, false));
            }}
            aria-label={t`Dedicated Docker daemon`}
            data-testid="storage-docker-dedicated"
            data-settings-dirty={dedicatedDirty}
          />
        }
      />
      {unavailable && (
        <p className="py-2 text-xs text-amber-600">
          <Trans>Docker is unavailable; Docker cleanup options cannot run on this host.</Trans>
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
  const { t } = useLingui();
  const retentionDirty = settingIsDirty(
    settings,
    savedSettings,
    (value) => value.quarantine_retention_hours,
  );
  return (
    <PolicySection
      sectionId="quarantine"
      title={t`Quarantine safety`}
      description={t`Keep recoverable resources for a grace period before permanent deletion.`}
      isDirty={retentionDirty}
    >
      <div className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label={t`Keep quarantined items for (hours)`}
          help={t`Cleanup first moves orphan workspaces and rotated Go caches into Kandev's trash area instead of deleting them immediately. During this retention period you can restore an item. After the deadline, a later maintenance run may permanently delete it.`}
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
        <h2 className="text-base font-medium">
          <Trans>Maintenance policy</Trans>
        </h2>
        <p className="text-xs text-muted-foreground">
          <Trans>
            Choose what Kandev may reclaim automatically and the safety limits it must follow.
          </Trans>
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
