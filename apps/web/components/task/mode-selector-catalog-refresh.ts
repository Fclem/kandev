import { t } from "@/lib/i18n";
import { matchesSessionCatalogSource } from "@/lib/types/session-catalog";
import type {
  SessionCatalogOwner,
  SessionCatalogRecord,
  SessionCatalogRefreshEntry,
} from "@/lib/types/session-catalog";
import type { TaskSession } from "@/lib/types/http";
import type { ModeOption, ModeSelectorState } from "./mode-selector";

type Input = {
  state: ModeSelectorState | undefined;
  currentModeId: string | null;
  currentModeName: string;
  liveModes: ModeOption[] | undefined;
  session: TaskSession | null;
  entry: SessionCatalogRefreshEntry | undefined;
  sourceOwner: SessionCatalogOwner | undefined;
};

export function projectSessionModeChoices(input: Input): ModeSelectorState | undefined {
  const { state, currentModeId, currentModeName, session, entry, sourceOwner } = input;
  const profileId = session?.execution_profile_id || session?.agent_profile_id;
  const host = hostModeCatalog(entry?.record, profileId, sourceOwner);
  if (!entry?.provider && !host) return state;
  const { availableModes: providerModes, providerIds } = providerModeChoices(input, profileId);
  let availableModes = providerModes;
  if (host) availableModes = mergeHostModes(host, availableModes, providerIds);
  if (currentModeId && !availableModes.some((mode) => mode.id === currentModeId)) {
    availableModes = [
      ...availableModes,
      {
        id: currentModeId,
        name: currentModeName,
        disabled: true,
        description: t("task:sessionCatalogChoiceUnavailable"),
      },
    ];
  }
  return {
    currentModeId: currentModeId ?? "",
    availableModes,
    requestedModeId: state?.requestedModeId,
  };
}

function hostModeCatalog(
  record: SessionCatalogRecord | undefined,
  profileId: string | undefined,
  sourceOwner: SessionCatalogOwner | undefined,
): SessionCatalogRecord | undefined {
  if (
    record?.disposition !== "host_candidates" ||
    !matchesSessionCatalogSource(record, profileId, sourceOwner) ||
    !record.modes?.length
  )
    return undefined;
  return record;
}

function providerModeChoices(input: Input, profileId: string | undefined) {
  const { state, liveModes, sourceOwner } = input;
  const provider = input.entry?.provider;
  if (!provider)
    return {
      availableModes: state?.availableModes ?? [],
      providerIds: new Set((liveModes ?? []).map((mode) => mode.id)),
    };
  const surface = provider.result.modes;
  const advertised =
    matchesSessionCatalogSource(provider, profileId, sourceOwner) &&
    provider.result.outcome === "complete" &&
    surface.present &&
    surface.supported;
  const availableModes: ModeOption[] = advertised
    ? (surface.choices ?? []).map((mode) => ({
        id: mode.id,
        name: mode.name || mode.id,
        description: mode.description,
      }))
    : [];
  return { availableModes, providerIds: new Set(availableModes.map((mode) => mode.id)) };
}

function mergeHostModes(
  record: SessionCatalogRecord,
  availableModes: ModeOption[],
  providerIds: Set<string>,
): ModeOption[] {
  const seen = new Set<string>();
  const candidates: ModeOption[] = (record.modes ?? []).flatMap((mode) => {
    if (seen.has(mode.id)) return [];
    seen.add(mode.id);
    const requiresRefresh = !providerIds.has(mode.id);
    return [
      {
        id: mode.id,
        name: mode.name || mode.id,
        requiresRefresh,
        description: requiresRefresh
          ? [mode.description, t("task:sessionCatalogChoiceRequiresRefresh")]
              .filter(Boolean)
              .join(". ")
          : mode.description,
      },
    ];
  });
  return [
    ...candidates,
    ...availableModes.filter((mode) => providerIds.has(mode.id) && !seen.has(mode.id)),
  ];
}
