import { t } from "@/lib/i18n";
import {
  projectedSessionProviderCatalog,
  matchesSessionCatalogSource,
} from "@/lib/types/session-catalog";
import type {
  SessionCatalogOwner,
  SessionCatalogRecord,
  SessionCatalogRefreshEntry,
  ProjectedSessionProviderCatalog,
} from "@/lib/types/session-catalog";
import type { TaskSession } from "@/lib/types/http";
import type { ModeOption, ModeSelectorState } from "./mode-selector-state";

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
  const provider = projectedSessionProviderCatalog(entry?.provider, profileId, sourceOwner);
  const unchanged = unprojectedModeState(input, host, provider);
  if (unchanged) return unchanged.value;
  const { availableModes: providerModes, providerIds } = providerModeChoices(input, provider);
  const choices = host ? mergeHostModes(host, providerModes, providerIds) : providerModes;
  const availableModes = retainCurrentMode(choices, currentModeId, currentModeName);
  if (availableModes.length === 0) return undefined;
  return {
    currentModeId: currentModeId ?? "",
    availableModes,
    requestedModeId: state?.requestedModeId,
  };
}

// Returns the result when there is nothing to project. Without catalog evidence
// the live state stands. Once a provider record applies, only advertised modes,
// host candidates or the current mode may appear: the generic profile's static
// modes and an unproven owner's live list are not this owner's catalog, so with
// none of the three there is nothing to show.
function unprojectedModeState(
  input: Input,
  host: SessionCatalogRecord | undefined,
  provider: ProjectedSessionProviderCatalog | undefined,
): { value: ModeSelectorState | undefined } | undefined {
  if (!provider && !host) return { value: input.state };
  if (host || advertisesModes(provider) || input.currentModeId) return undefined;
  return { value: undefined };
}

// The current mode stays visible, disabled, when the catalog no longer offers it.
function retainCurrentMode(
  availableModes: ModeOption[],
  currentModeId: string | null,
  currentModeName: string,
): ModeOption[] {
  if (!currentModeId || availableModes.some((mode) => mode.id === currentModeId)) {
    return availableModes;
  }
  return [
    ...availableModes,
    {
      id: currentModeId,
      name: currentModeName,
      disabled: true,
      description: t("task:sessionCatalogChoiceUnavailable"),
    },
  ];
}

function advertisesModes(provider: ProjectedSessionProviderCatalog | undefined): boolean {
  if (!provider?.current) return false;
  const { outcome, modes } = provider.record.result;
  return outcome === "complete" && modes.present && modes.supported;
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

function providerModeChoices(input: Input, provider: ProjectedSessionProviderCatalog | undefined) {
  const { state, liveModes } = input;
  if (!provider)
    return {
      availableModes: state?.availableModes ?? [],
      providerIds: new Set((liveModes ?? []).map((mode) => mode.id)),
    };
  const surface = provider.record.result.modes;
  const availableModes: ModeOption[] = advertisesModes(provider)
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
