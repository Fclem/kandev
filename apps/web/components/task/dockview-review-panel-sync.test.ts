import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  resolveCanonicalReviewParams,
  syncCanonicalReviewPanel,
} from "./dockview-review-panel-sync";

function makeApi(panel?: { params?: Record<string, unknown>; groupId?: string }): {
  api: DockviewApi;
  updateParameters: ReturnType<typeof vi.fn>;
} {
  const updateParameters = vi.fn((next: Record<string, unknown>) => {
    Object.assign(panel?.params ?? {}, next);
  });
  const reviewPanel = panel
    ? {
        id: "pr-detail",
        params: panel.params ?? {},
        group: { id: panel.groupId ?? "group-right-top" },
        api: { updateParameters },
      }
    : undefined;
  return {
    api: {
      getPanel: (id: string) => (id === "pr-detail" ? reviewPanel : undefined),
      addPanel: vi.fn(),
      removePanel: vi.fn(),
    } as unknown as DockviewApi,
    updateParameters,
  };
}

const githubPR = {
  owner: "kandev",
  repo: "kandev",
  pr_number: 42,
} as TaskPR;

const gitlabMR = {
  host: "https://gitlab.example.test",
  project_path: "group/project",
  mr_iid: 7,
} as TaskMR;
const githubPRKey = "kandev/kandev/42";
const gitlabMRKey = "https://gitlab.example.test|group/project|7";

describe("resolveCanonicalReviewParams", () => {
  it("prefers the primary GitHub pull request when both providers are linked", () => {
    expect(resolveCanonicalReviewParams([githubPR], [gitlabMR])).toEqual({
      providerId: "github",
      provider: "github",
      reviewKey: githubPRKey,
      prKey: githubPRKey,
      mrKey: undefined,
    });
  });

  it("selects the first linked GitLab merge request when GitHub is absent", () => {
    expect(resolveCanonicalReviewParams([], [gitlabMR])).toEqual({
      providerId: "gitlab",
      provider: "gitlab",
      reviewKey: gitlabMRKey,
      prKey: undefined,
      mrKey: gitlabMRKey,
    });
  });

  it("clears review identity when the active task has no linked review", () => {
    expect(resolveCanonicalReviewParams([], [])).toEqual({
      providerId: undefined,
      provider: undefined,
      reviewKey: undefined,
      prKey: undefined,
      mrKey: undefined,
    });
  });

  it("selects a registered review when no built-in review is linked", () => {
    expect(
      resolveCanonicalReviewParams(
        [],
        [],
        [
          {
            providerId: "bitbucket",
            reviewKey: "workspace/repository/42",
            title: "Bitbucket pull request",
            url: "https://bitbucket.example/workspace/repository/pull-requests/42",
            repositoryId: "repository-1",
            state: "OPEN",
          },
        ],
      ),
    ).toEqual({
      providerId: "bitbucket",
      provider: undefined,
      reviewKey: "workspace/repository/42",
      prKey: undefined,
      mrKey: undefined,
    });
  });
});

describe("syncCanonicalReviewPanel", () => {
  it("leaves a layout without PR Details structurally untouched", () => {
    const { api, updateParameters } = makeApi();

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(false);
    expect(updateParameters).not.toHaveBeenCalled();
    expect(api.addPanel).not.toHaveBeenCalled();
    expect(api.removePanel).not.toHaveBeenCalled();
  });

  it("updates an existing panel's identity without changing its configured group", () => {
    const params: Record<string, unknown> = { provider: "gitlab", mrKey: "old/mr" };
    const { api, updateParameters } = makeApi({ params, groupId: "custom-review-group" });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(true);
    expect(updateParameters).toHaveBeenCalledWith({
      providerId: "github",
      provider: "github",
      reviewKey: githubPRKey,
      prKey: githubPRKey,
      mrKey: undefined,
    });
    expect(api.getPanel("pr-detail")?.group.id).toBe("custom-review-group");
    expect(params).toMatchObject({ provider: "github", prKey: githubPRKey });
    expect(params.mrKey).toBeUndefined();
  });
});
