import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  fetchGitHubStarConnection,
  fetchGitHubStarStates,
  starGitHubRepository,
  startGitHubStarConnection,
  unstarGitHubRepository,
} from "./github-star-api";

const workspaceID = "ws/1";
type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function lastCall() {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("dedicated personal star connection", () => {
  it("scopes the connection read to the workspace", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        user_id: "user-1",
        credential_generation: 2,
        active: true,
        capable: true,
        reconnect_required: false,
        available: true,
      }),
    );

    await fetchGitHubStarConnection(workspaceID, { cache: "no-store" });

    const { url, init } = lastCall();
    expect(url).toBe(
      "http://api.test/api/v1/github/user/starred/connection?workspace_id=ws%2F1",
    );
    expect(init).toMatchObject({ cache: "no-store" });
  });

  it("starts the flow with an explicit workspace and same-origin return path", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        authorization_url: "https://github.com/login/oauth/authorize?state=s",
        state_expires_at: "2026-09-22T10:00:00Z",
      }),
    );

    await startGitHubStarConnection({ workspace_id: workspaceID, return_path: "/settings/plugins" });

    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/github/user/starred/connection/start");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      workspace_id: workspaceID,
      return_path: "/settings/plugins",
    });
  });

  it("sends raw repository URLs to the bounded status endpoint", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ states: { "kdlbs/kandev": true } }));

    await fetchGitHubStarStates({
      workspace_id: "ws-1",
      repository_urls: ["https://github.com/Kdlbs/Kandev", "https://github.com/kdlbs/other.git"],
    });

    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/github/user/starred/status");
    expect(init?.method).toBe("POST");
    // The backend canonicalizes; the client never rewrites the URL.
    expect(JSON.parse(String(init?.body))).toEqual({
      workspace_id: "ws-1",
      repository_urls: ["https://github.com/Kdlbs/Kandev", "https://github.com/kdlbs/other.git"],
    });
  });

  it("stars with PUT and unstars with DELETE, both carrying the workspace", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ starred: true }))
      .mockResolvedValueOnce(jsonResponse({ starred: false }));
    const payload = {
      workspace_id: workspaceID,
      repository_url: "https://github.com/kdlbs/kandev",
    };

    await starGitHubRepository(payload);
    const starCall = lastCall();
    await unstarGitHubRepository(payload);
    const unstarCall = lastCall();

    expect(starCall.url).toBe("http://api.test/api/v1/github/user/starred");
    expect(starCall.init?.method).toBe("PUT");
    expect(unstarCall.url).toBe("http://api.test/api/v1/github/user/starred");
    expect(unstarCall.init?.method).toBe("DELETE");
    expect(JSON.parse(String(unstarCall.init?.body))).toEqual(payload);
  });
});
