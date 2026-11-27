import { z } from "zod";
import { MODEL_CATALOG_LIMITS } from "@kandev/types/model-catalog";
import { normalizeModelCatalog } from "@/lib/model-catalog";

const safeCounter = z.number().int().nonnegative().max(Number.MAX_SAFE_INTEGER);
const identifier = z.string().min(1).max(MODEL_CATALOG_LIMITS.maxIdLength);
const label = z.string().max(MODEL_CATALOG_LIMITS.maxStringLength);
const catalog = z.unknown().transform(normalizeModelCatalog).optional();

export const sessionCatalogOwnerSchema = z.object({
  execution_id: identifier,
  attempt_id: z.string().max(MODEL_CATALOG_LIMITS.maxStringLength),
  source_generation: safeCounter,
});
export type SessionCatalogOwner = z.infer<typeof sessionCatalogOwnerSchema>;

export const sessionCatalogSourceOwnerSchema = z
  .object({
    settings_source_execution_id: identifier,
    settings_attempt_id: z
      .string()
      .max(MODEL_CATALOG_LIMITS.maxStringLength)
      .optional()
      .default(""),
    settings_source_generation: safeCounter.positive(),
  })
  .transform(
    (source): SessionCatalogOwner => ({
      execution_id: source.settings_source_execution_id,
      attempt_id: source.settings_attempt_id,
      source_generation: source.settings_source_generation,
    }),
  );

export function sameSessionCatalogOwner(
  left: SessionCatalogOwner | undefined,
  right: SessionCatalogOwner | undefined,
): boolean {
  return (
    !!left &&
    !!right &&
    left.execution_id === right.execution_id &&
    left.attempt_id === right.attempt_id &&
    left.source_generation === right.source_generation
  );
}

export const sessionCatalogChoiceSchema = z.object({
  id: identifier,
  name: label.optional(),
  description: label.optional(),
});
export type SessionCatalogChoice = z.infer<typeof sessionCatalogChoiceSchema>;
const choices = z.array(sessionCatalogChoiceSchema).max(MODEL_CATALOG_LIMITS.maxNodes);

export const sessionCatalogSurfaceSchema = z.object({
  present: z.boolean(),
  supported: z.boolean(),
  choices: choices.nullable(),
  catalog,
});
export type SessionCatalogSurface = z.infer<typeof sessionCatalogSurfaceSchema>;

export const sessionCatalogDiscoverySchema = z
  .object({
    outcome: z.enum(["complete", "unsupported", "failed"]),
    generation: safeCounter.optional(),
    models: sessionCatalogSurfaceSchema,
    modes: sessionCatalogSurfaceSchema,
  })
  .refine((result) => {
    const surfaces = [result.models, result.modes];
    if (surfaces.some((surface) => surface.supported && !surface.present)) return false;
    if (result.outcome === "complete")
      return surfaces.some((surface) => surface.present && surface.supported);
    return surfaces.every(
      (surface) => !surface.supported && !surface.choices?.length && !surface.catalog,
    );
  });
export type SessionCatalogDiscovery = z.infer<typeof sessionCatalogDiscoverySchema>;

// Host candidates are task-owned only when the target's own complete fresh
// task probe produced them.
export const SESSION_CATALOG_TASK_PROBE_SOURCE = "task_probe_v1";

const rawSessionCatalogRecordSchema = z.object({
  version: z.literal(1),
  revision: safeCounter.positive(),
  profile_id: identifier,
  digest: z.string().optional(),
  owner: sessionCatalogOwnerSchema,
  disposition: z.enum(["host_candidates", "rediscovery_required", "cleared"]),
  reason: z.string(),
  source: z.string().optional(),
  models: choices.optional(),
  modes: choices.optional(),
  catalog,
  probe: sessionCatalogDiscoverySchema.optional(),
  result: sessionCatalogDiscoverySchema.optional(),
});
type RawSessionCatalogRecord = z.infer<typeof rawSessionCatalogRecordSchema>;

// Missing or legacy provenance is never reinterpreted as task-owned: such a
// record becomes a rediscovery notice. An unsupported surface never supplies
// candidates, even when a list accompanies it.
function retainTaskOwnedCandidates(record: RawSessionCatalogRecord): RawSessionCatalogRecord {
  if (record.disposition !== "host_candidates") return record;
  const probe = record.probe;
  if (record.source !== SESSION_CATALOG_TASK_PROBE_SOURCE || probe?.outcome !== "complete") {
    return {
      ...record,
      disposition: "rediscovery_required",
      reason: "context_unverified",
      source: undefined,
      models: undefined,
      modes: undefined,
      catalog: undefined,
      probe: undefined,
    };
  }
  const modelsSupported = probe.models.present && probe.models.supported;
  const modesSupported = probe.modes.present && probe.modes.supported;
  return {
    ...record,
    models: modelsSupported ? record.models : undefined,
    catalog: modelsSupported ? record.catalog : undefined,
    modes: modesSupported ? record.modes : undefined,
  };
}

export const sessionCatalogRecordSchema =
  rawSessionCatalogRecordSchema.transform(retainTaskOwnedCandidates);
export type SessionCatalogRecord = z.infer<typeof sessionCatalogRecordSchema>;

export const sessionProviderCatalogSchema = z.object({
  revision: safeCounter.positive(),
  owner: sessionCatalogOwnerSchema,
  profile_id: identifier,
  result: sessionCatalogDiscoverySchema,
});
export type SessionProviderCatalog = z.infer<typeof sessionProviderCatalogSchema>;

export function matchesSessionCatalogSource(
  catalog: SessionCatalogRecord | SessionProviderCatalog | undefined,
  profileId: string | undefined,
  sourceOwner: SessionCatalogOwner | undefined,
): boolean {
  return (
    !!catalog &&
    catalog.profile_id === profileId &&
    sameSessionCatalogOwner(catalog.owner, sourceOwner)
  );
}

export type ProjectedSessionProviderCatalog = {
  record: SessionProviderCatalog;
  // Only a current-owner record may advertise choices.
  current: boolean;
};

// A provider record proves availability only for the owner that reported it.
// When the current owner is proven and differs (a failed start, or a transport
// that never publishes discovery), the record is ignored so it cannot disable
// that owner's live choices. While the owner is unproven the live lists may be
// stale too, so the record still retires them instead of reviving them.
export function projectedSessionProviderCatalog(
  provider: SessionProviderCatalog | undefined,
  profileId: string | undefined,
  sourceOwner: SessionCatalogOwner | undefined,
): ProjectedSessionProviderCatalog | undefined {
  if (!provider) return undefined;
  if (matchesSessionCatalogSource(provider, profileId, sourceOwner)) {
    return { record: provider, current: true };
  }
  return sourceOwner ? undefined : { record: provider, current: false };
}
export const sessionCatalogResumeAckSchema = z.object({
  accepted_launch_owner: sessionCatalogOwnerSchema.optional(),
});

export const sessionCatalogMetadataSchema = z.object({
  profile_catalog_refresh: sessionCatalogRecordSchema.optional().catch(undefined),
  acp_model_state: z
    .object({
      catalog_discovery: sessionProviderCatalogSchema.optional().catch(undefined),
    })
    .optional()
    .catch(undefined),
});

export const sessionCatalogRefreshPayloadSchema = z.object({
  task_id: identifier,
  session_id: identifier,
  record: sessionCatalogRecordSchema,
});
export const sessionCatalogDiscoveryPayloadSchema = z.object({
  task_id: identifier,
  session_id: identifier,
  provider: sessionProviderCatalogSchema,
});
export type SessionCatalogRefreshPayload = z.infer<typeof sessionCatalogRefreshPayloadSchema>;
export type SessionCatalogDiscoveryPayload = z.infer<typeof sessionCatalogDiscoveryPayloadSchema>;

export type SessionCatalogChecking = {
  taskId: string;
  sessionId: string;
  noticeRevision: number;
  owner?: SessionCatalogOwner;
  deadlineAt?: number;
  status: "checking" | "unavailable";
};
export type SessionCatalogObservationTarget = Omit<SessionCatalogChecking, "deadlineAt" | "status">;
export type SessionCatalogRefreshEntry = {
  taskId: string;
  record?: SessionCatalogRecord;
  provider?: SessionProviderCatalog;
  checking?: SessionCatalogChecking;
};
export type SessionCatalogRefreshState = {
  bySessionId: Record<string, SessionCatalogRefreshEntry>;
};
