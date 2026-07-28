import { useTranslation } from "react-i18next";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { LicensesList } from "@/components/settings/system/licenses-list";
import licenses from "@/generated/licenses.json";
import type { LicenseEntry } from "@/lib/types/system";

export default function SystemLicensesPage() {
  const { t } = useTranslation();
  const entries = licenses as LicenseEntry[];

  return (
    <StateProvider initialState={{}}>
      <SystemPageShell
        title={t("common:licenses")}
        description={t("settings:openSourceLicensesForEveryNpm")}
      >
        <LicensesList entries={entries} />
      </SystemPageShell>
    </StateProvider>
  );
}
