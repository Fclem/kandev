import { useTranslation } from "react-i18next";
import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";

export default function StoragePage() {
  const { t } = useTranslation();
  return (
    <SystemPageShell
      title={t("common:storage")}
      description={t("settings:reviewDiskUseAndReclaimSpace")}
    >
      <StorageMaintenanceSettings />
    </SystemPageShell>
  );
}
