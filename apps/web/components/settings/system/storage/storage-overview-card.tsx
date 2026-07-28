import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconChartPie, IconTrash } from "@tabler/icons-react";
import type { StorageOverviewResponse, StorageQuarantineSummary } from "@/lib/types/system";
import { formatRelativeTime } from "@/lib/utils";
import { StorageActionButton } from "./storage-action-button";
import { formatGigabytes } from "./storage-units";

interface Props {
  overview: StorageOverviewResponse | null;
  disabledReason?: string;
  onRunGoCache: () => void;
}

interface StorageResource {
  id: string;
  label: string;
  value: string;
  detail: string;
  warning?: string;
}

function goCacheDisabledReason(overview: StorageOverviewResponse, pendingReason?: string) {
  if (pendingReason) return pendingReason;
  if (overview.summary.go_cache.owned !== true) {
    return t("settings:onlyAKandevOwnedGoBuild");
  }
  if ((overview.summary.go_cache.size_bytes ?? 0) <= overview.settings.go_cache.max_bytes) {
    return t("settings:theGoBuildCacheIsBelow");
  }
  return undefined;
}

function quarantineResource(summary: StorageQuarantineSummary): StorageResource {
  if (summary.available === false) {
    return {
      id: "quarantine",
      label: t("settings:quarantinedResources"),
      value: t("settings:unavailable"),
      detail: t("settings:quarantineUsageCouldNotBeMeasured"),
      warning: summary.warning,
    };
  }
  const count = summary.count;
  return {
    id: "quarantine",
    label: t("settings:quarantinedResources"),
    value: formatGigabytes(summary.size_bytes),
    detail: t("settings:itemsMovedAsideForRecoveryBefore", { count }),
  };
}

function dockerMeasurement(
  available: boolean,
  value: string,
  detail: string,
): Pick<StorageResource, "value" | "detail"> {
  if (!available) {
    return {
      value: t("settings:unavailable"),
      detail: t("settings:dockerUsageCouldNotBeMeasured"),
    };
  }
  return { value, detail };
}

function storageResources(overview: StorageOverviewResponse): StorageResource[] {
  const { summary } = overview;
  const dockerWarning = summary.docker.warnings?.join(" · ");
  const reclaimable = formatGigabytes(summary.workspaces.candidate_bytes ?? 0);
  const active = formatGigabytes(summary.workspaces.active_bytes ?? 0);
  const managedContainers = summary.docker.managed_container_count ?? 0;
  return [
    {
      id: "workspaces",
      label: t("settings:taskWorkspaces"),
      value: formatGigabytes(summary.workspaces.total_bytes ?? 0),
      detail: t("settings:reclaimableAfterTheGracePeriodActive", { reclaimable, active }),
      warning: summary.workspaces.warning,
    },
    quarantineResource(summary.quarantine),
    {
      id: "managed-containers",
      label: t("settings:kandevContainers"),
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.managed_container_bytes ?? 0),
        t("settings:managedContainers", { managedContainers }),
      ),
      warning: dockerWarning,
    },
    {
      id: "go-cache",
      label: t("settings:goBuildCache"),
      value: formatGigabytes(summary.go_cache.size_bytes ?? 0),
      detail: summary.go_cache.path ?? overview.capabilities.managed_go_cache_path,
      warning: summary.go_cache.warning,
    },
    ...(summary.go_cache.unmanaged_path
      ? [
          {
            id: "unmanaged-go-cache",
            label: t("settings:userGoBuildCache"),
            value: formatGigabytes(summary.go_cache.unmanaged_size_bytes ?? 0),
            detail: summary.go_cache.unmanaged_path,
          },
        ]
      : []),
    {
      id: "docker-image-layers",
      label: t("settings:dockerImageLayers"),
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.image_layer_bytes ?? 0),
        overview.capabilities.docker_host || t("settings:defaultDockerHost"),
      ),
      warning: dockerWarning,
    },
    {
      id: "docker-build-cache",
      label: t("settings:dockerBuildCache"),
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.build_cache_bytes),
        overview.capabilities.docker_host || t("settings:defaultDockerHost"),
      ),
      warning: dockerWarning,
    },
    {
      id: "docker-unused-images",
      label: t("settings:unusedDockerImages"),
      ...dockerMeasurement(
        summary.docker.available,
        formatGigabytes(summary.docker.unused_image_bytes),
        t("settings:unusedByEveryContainerAndOlder"),
      ),
      warning: dockerWarning,
    },
  ];
}

interface ResourceRowProps {
  resource: StorageResource;
  goCacheCleanupDisabledReason?: string;
  onRunGoCache: () => void;
}

function ResourceRow({ resource, goCacheCleanupDisabledReason, onRunGoCache }: ResourceRowProps) {
  const { t } = useTranslation();
  return (
    <AccordionItem value={resource.id} data-testid={`storage-resource-${resource.id}`}>
      <AccordionTrigger
        className="min-h-11 items-center px-3 no-underline"
        data-testid={`storage-resource-${resource.id}-trigger`}
      >
        <span className="min-w-0">
          <span className="block text-sm">{resource.label}</span>
          <span className="block text-xs font-normal text-muted-foreground">{resource.value}</span>
        </span>
      </AccordionTrigger>
      <AccordionContent className="px-3">
        <p className="break-all text-muted-foreground">{resource.detail}</p>
        {resource.warning && <p className="mt-2 break-words text-amber-600">{resource.warning}</p>}
        {resource.id === "go-cache" && (
          <StorageActionButton
            variant="outline"
            className="mt-3 w-full sm:w-auto"
            disabledReason={goCacheCleanupDisabledReason}
            onClick={onRunGoCache}
            data-testid="storage-go-cache-clean"
          >
            <IconTrash className="size-4" /> {t("settings:cleanGoCache")}
          </StorageActionButton>
        )}
      </AccordionContent>
    </AccordionItem>
  );
}

export function StorageOverviewCard({ overview, disabledReason, onRunGoCache }: Props) {
  const { t } = useTranslation();
  if (!overview) {
    return (
      <Card data-testid="storage-overview-card">
        <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Spinner className="size-4" data-testid="storage-overview-spinner" />
          {t("settings:loadingStorageData")}
        </CardContent>
      </Card>
    );
  }
  const { summary } = overview;
  const analyzedAt = new Date(overview.analyzed_at).toLocaleString();
  const relativeTime = formatRelativeTime(overview.analyzed_at);
  const cleanupDisabledReason = goCacheDisabledReason(overview, disabledReason);
  const resources = storageResources(overview);
  return (
    <Card className="min-w-0" data-testid="storage-overview-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconChartPie className="size-4" /> {t("settings:storageAnalysis")}
          {!summary.docker.available && (
            <Badge variant="outline">{t("settings:dockerUnavailable")}</Badge>
          )}
        </CardTitle>
        <CardDescription>{t("settings:aReadOnlyBreakdownOfCurrent")}</CardDescription>
        <time
          className="text-xs text-muted-foreground"
          dateTime={overview.analyzed_at}
          title={analyzedAt}
          aria-label={t("settings:lastAnalyzed", { analyzedAt })}
        >
          {t("settings:lastAnalyzed2", { relativeTime })}
        </time>
      </CardHeader>
      <CardContent className="min-w-0">
        <Accordion type="multiple" className="min-w-0">
          {resources.map((resource) => (
            <ResourceRow
              key={resource.id}
              resource={resource}
              goCacheCleanupDisabledReason={cleanupDisabledReason}
              onRunGoCache={onRunGoCache}
            />
          ))}
        </Accordion>
      </CardContent>
    </Card>
  );
}
