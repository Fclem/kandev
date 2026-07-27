import { t } from "@lingui/core/macro";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { DatabaseStatsCard } from "@/components/settings/system/database-stats-card";
import { fetchDatabaseStats } from "@/lib/api/domains/system-api";

export default async function SystemDatabasePage() {
  let initialState: Record<string, unknown> = {};
  try {
    const database = await fetchDatabaseStats({ cache: "no-store" }).catch(() => null);
    initialState = { system: database ? { database } : undefined };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title={t`Database`}
        description={t`Database driver, size, and available maintenance controls.`}
      >
        <DatabaseStatsCard />
      </SystemPageShell>
    </StateProvider>
  );
}
