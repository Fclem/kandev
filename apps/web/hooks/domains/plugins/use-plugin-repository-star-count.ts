"use client";

import { useEffect, useMemo } from "react";
import { useOptionalAppStore, useOptionalAppStoreApi } from "@/components/state-provider";
import { parseGitHubRepositoryUrl } from "@/lib/plugins/github-repository-url";

/** Shares the catalog count and confirmed session adjustments across repository rows. */
export function usePluginRepositoryStarCount(repositoryUrl: string, catalogCount: number | null) {
  const store = useOptionalAppStoreApi();
  const repository = useMemo(() => parseGitHubRepositoryUrl(repositoryUrl), [repositoryUrl]);
  const count = useOptionalAppStore(
    (state) => (repository ? state.pluginStars.repositoryCounts[repository] : undefined),
    undefined,
  );

  useEffect(() => {
    if (repository) store?.getState().seedPluginStarCount(repository, catalogCount);
  }, [catalogCount, repository, store]);

  return count ?? catalogCount;
}
