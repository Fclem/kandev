import { t } from "@lingui/core/macro";
import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function StoragePage() {
  return (
    <SystemPageShell
      title={t`Storage`}
      description={t`Review disk use and reclaim space from Kandev-owned workspaces, caches, and Docker resources whenever your installation needs it.`}
    >
      <StorageMaintenanceSettings />
    </SystemPageShell>
  );
}
