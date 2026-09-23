export class BackendFixtureEnvOverrides {
  private readonly entries = new Map<symbol, Record<string, string>>();

  add(overrides: Record<string, string>): () => void {
    this.assertNoConflicts(overrides);
    const token = Symbol("backend-fixture-env");
    this.entries.set(token, overrides);
    return () => this.entries.delete(token);
  }

  apply(
    baseline: Record<string, string>,
    oneShotOverrides?: Record<string, string>,
  ): Record<string, string> {
    if (oneShotOverrides) this.assertNoConflicts(oneShotOverrides);
    const env = { ...baseline };
    for (const overrides of this.entries.values()) Object.assign(env, overrides);
    if (oneShotOverrides) Object.assign(env, oneShotOverrides);
    return env;
  }

  private assertNoConflicts(next: Record<string, string>): void {
    for (const current of this.entries.values()) {
      for (const [key, value] of Object.entries(next)) {
        if (key in current && current[key] !== value) {
          throw new Error(`conflicting backend fixture environment override for ${key}`);
        }
      }
    }
  }
}

export function createScopedEnvUse(
  scopedEnv: BackendFixtureEnvOverrides,
  restart: () => Promise<void>,
): (overrides: Record<string, string>) => Promise<() => Promise<void>> {
  return async (overrides) => {
    const removeScope = scopedEnv.add(overrides);
    try {
      await restart();
    } catch (activationError) {
      removeScope();
      try {
        await restart();
      } catch (restoreError) {
        throw new AggregateError(
          [activationError, restoreError],
          "failed to activate backend fixture environment and restore baseline",
        );
      }
      throw activationError;
    }

    let released = false;
    return async () => {
      if (released) return;
      removeScope();
      await restart();
      released = true;
    };
  };
}

/**
 * Sanitizes the inherited environment before handing it to the e2e backend.
 * Three classes of vars must not leak through the `...process.env` spread:
 *   - GitHub tokens — tests must hit the mock GitHub, never a real token.
 *   - KANDEV_FEATURES_* flags — these are profile-managed (profiles.yaml `e2e:`
 *     column turns them on). When the suite is launched from inside a kandev
 *     task, the parent process exports KANDEV_FEATURES_OFFICE=false; left in
 *     place it survives the spread and, because the backend's ApplyProfile
 *     leaves already-set vars alone, disables Office (and any future feature)
 *     in the test backend → /api/v1/office/* 404s. Dropping the whole
 *     KANDEV_FEATURES_* namespace lets the e2e profile govern feature flags so
 *     the suite always exercises them, regardless of where it's launched.
 *   - KANDEV_WEB_TITLE_PREFIX — the browser identity is profile-managed in
 *     this suite. Explicit per-test values are applied after this baseline is
 *     sanitized through `backend.restart(overrides)`.
 *   - KANDEV_GITHUB_PERSONAL_STAR_OAUTH_* — the dedicated personal-star
 *     deployment settings are owned by the deployment fixture
 *     (`fixtures/personal-star-deployment.ts`), which seeds the matching
 *     deployment material and applies these values through
 *     `backend.restart(seed.env)`. A host shell (or another kandev instance)
 *     exporting them would otherwise make a spec's "unsupported deployment"
 *     baseline depend on the developer's environment.
 *   - KANDEV_SERVICE_MODE / KANDEV_DEPLOYMENT_SECRET_TARGET — both decide
 *     whether this backend may use the root-owned deployment-secret
 *     supervisor. The launcher writes KANDEV_SERVICE_MODE into every service
 *     unit it installs, and the suite is often launched from a kandev task
 *     running under `kandev.service`, so the spawned backend would otherwise
 *     inherit "user" and classify itself as a user-scope systemd install. That
 *     is a deployment fact, not a test fact, and the same is true of a host
 *     value for KANDEV_DEPLOYMENT_SECRET_TARGET, which would beat the e2e
 *     profile's "supported" because the profile only fills unset variables.
 *     Specs that want those rejections set them explicitly through
 *     `backend.restart({ ... })`.
 *   - PATH casing aliases — Windows commonly inherits `Path`; retaining it
 *     beside the fixture's new `PATH` makes child-process lookup order
 *     ambiguous. The caller restores one canonical PATH after sanitizing.
 *   - Service-install markers — a host's managed service identity must not make
 *     the source-built e2e backend appear eligible for managed updates.
 *   - Remote helper paths — container-backed tests must exercise the standard
 *     package cache path, never an inherited helper override. */
export function sanitizeInheritedEnv(env: Record<string, string>): Record<string, string> {
  const cleaned = { ...env };
  delete cleaned.GH_TOKEN;
  delete cleaned.GITHUB_TOKEN;
  for (const key of [
    "KANDEV_RUNNING_AS_SERVICE",
    "KANDEV_SERVICE_MODE",
    "KANDEV_SERVICE_MANAGER",
    "KANDEV_INSTALL_KIND",
    "KANDEV_SERVICE_METADATA",
  ]) {
    delete cleaned[key];
  }
  delete cleaned.KANDEV_DEPLOYMENT_SECRET_TARGET;
  for (const key of Object.keys(cleaned)) {
    if (
      key === "KANDEV_WEB_TITLE_PREFIX" ||
      key.startsWith("KANDEV_FEATURES_") ||
      key.startsWith("KANDEV_GITHUB_PERSONAL_STAR_OAUTH_")
    ) {
      delete cleaned[key];
    }
    if (/^KANDEV_AGENTCTL_(?:LINUX_BINARY|(?:LINUX|DARWIN)_(?:AMD64|ARM64)_BINARY)$/i.test(key)) {
      delete cleaned[key];
    }
    if (key.toUpperCase() === "PATH") delete cleaned[key];
  }
  return cleaned;
}
