import { useTranslation } from "react-i18next";
import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { AboutCard } from "@/components/settings/system/about-card";
import { fetchSystemInfo } from "@/lib/api/domains/system-api";

export default async function SystemAboutPage() {
  const { t } = useTranslation();
  let initialState: Record<string, unknown> = {};
  try {
    const info = await fetchSystemInfo({ cache: "no-store" }).catch(() => null);
    if (info) {
      initialState = { system: { info } };
    }
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <SystemPageShell
        title={t("common:about")}
        description={t("settings:versionBuildMetadataAndLinks")}
      >
        <AboutCard />
      </SystemPageShell>
    </StateProvider>
  );
}
