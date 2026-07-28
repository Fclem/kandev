import { useTranslation } from "react-i18next";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { BackupsTable } from "@/components/settings/system/backups-table";
import { fetchBackups } from "@/lib/api/domains/system-api";

export default async function SystemBackupsPage() {
  const { t } = useTranslation();
  let initialState: Record<string, unknown> = {};
  try {
    const backups = await fetchBackups({ cache: "no-store" }).catch(() => null);
    if (backups) {
      initialState = {
        system: {
          backups: { items: backups, loaded: true },
        },
      };
    }
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title={t("common:backups")}
        description={t("settings:vacuumIntoSnapshotsStoredUnderBackups")}
      >
        <BackupsTable />
      </SystemPageShell>
    </StateProvider>
  );
}
