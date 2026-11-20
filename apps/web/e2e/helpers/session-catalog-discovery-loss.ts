import { expect } from "@playwright/test";
import type { Page, WebSocketRoute } from "@playwright/test";
import {
  sessionCatalogResumeAckSchema,
  type SessionCatalogRefreshEntry,
} from "../../lib/types/session-catalog";
import type { ApiClient } from "./api-client";
import {
  assertCatalogNativeRetained,
  catalogMutationFrames,
  catalogRecord,
  cleanupSessionCatalogScenario,
  createSessionCatalogScenario,
  refreshSavedSessionCatalog,
  writeSessionCatalogFile,
} from "./session-catalog-refresh";
import {
  assertRestartRetained,
  captureRestartBaseline,
} from "../tests/session/session-tab-restart-helpers";
import { activate } from "./builtin-mcp-management";
import type {
  SessionCatalogBrowserContext,
  SessionCatalogScenario,
} from "./session-catalog-refresh";
import type { SessionCatalogOwner } from "../../lib/types/session-catalog";

type CatalogFrame = {
  type?: string;
  action?: string;
  payload?: { session_id?: string; record?: { disposition?: string; result?: unknown } };
};

function stripTerminalCatalogMetadata(value: unknown, sessionId: string): boolean {
  if (!value || typeof value !== "object") return false;
  if (Array.isArray(value)) {
    let changed = false;
    for (const item of value) changed = stripTerminalCatalogMetadata(item, sessionId) || changed;
    return changed;
  }
  const object = value as Record<string, unknown>;
  let changed = false;
  if (object.id === sessionId || object.session_id === sessionId) {
    for (const field of ["metadata", "session_metadata"] as const)
      changed = stripTerminalMetadataFields(object[field]) || changed;
  }
  for (const child of Object.values(object))
    changed = stripTerminalCatalogMetadata(child, sessionId) || changed;
  return changed;
}

function stripTerminalMetadataFields(raw: unknown): boolean {
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return false;
  const metadata = raw as Record<string, unknown>;
  const record = metadata.profile_catalog_refresh as
    | { disposition?: string; result?: unknown }
    | undefined;
  let changed = false;
  if (record?.disposition === "cleared" || record?.result) {
    delete metadata.profile_catalog_refresh;
    changed = true;
  }
  const source = metadata.acp_model_state as Record<string, unknown> | undefined;
  if (source?.catalog_discovery) {
    delete source.catalog_discovery;
    changed = true;
  }
  return changed;
}

function isTerminalCatalogFrame(frame: CatalogFrame): boolean {
  const record = frame.payload?.record;
  return (
    frame.action === "session.catalog_discovery_updated" ||
    (frame.action === "session.catalog_refresh_updated" &&
      (record?.disposition === "cleared" || !!record?.result))
  );
}

/** Delays committed catalog projections, not launch ACKs or provider execution. */
export async function routeSessionCatalogDiscoveryLoss(page: Page) {
  let targetSessionId: string | undefined;
  let holding = false;
  let currentSocket: WebSocketRoute | undefined;
  const held: string[] = [];
  await page.routeWebSocket(/\/ws$/, (socket) => {
    currentSocket = socket;
    const server = socket.connectToServer();
    socket.onMessage((message) => server.send(message));
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      for (const part of message.split("\n").filter(Boolean)) {
        let frame: CatalogFrame;
        try {
          frame = JSON.parse(part) as CatalogFrame;
        } catch {
          socket.send(part);
          continue;
        }
        const catalogResult = isTerminalCatalogFrame(frame);
        if (
          holding &&
          frame.type === "notification" &&
          frame.payload?.session_id === targetSessionId &&
          catalogResult
        )
          held.push(part);
        else
          socket.send(
            holding && targetSessionId && stripTerminalCatalogMetadata(frame, targetSessionId)
              ? JSON.stringify(frame)
              : part,
          );
      }
    });
  });
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    if (!holding || request.method() !== "GET") {
      await route.continue();
      return;
    }
    const headers = { ...request.headers() };
    delete headers["if-none-match"];
    const response = await route.fetch({ headers });
    if (
      response.status() !== 200 ||
      !response.headers()["content-type"]?.includes("application/json")
    ) {
      await route.fulfill({ response });
      return;
    }
    const data: unknown = await response.json();
    const catalogStripped =
      !!targetSessionId && stripTerminalCatalogMetadata(data, targetSessionId);
    if (catalogStripped) {
      await route.fulfill({ response, json: data });
    } else {
      await route.fulfill({ response });
    }
  });
  return {
    hold(sessionId: string) {
      targetSessionId = sessionId;
      holding = true;
    },
    heldCount() {
      return held.length;
    },
    release() {
      holding = false;
      if (held.length && !currentSocket)
        throw new Error("No client socket for delayed catalog delivery");
      for (const message of held.splice(0)) currentSocket!.send(message);
    },
  };
}

type CatalogDeadlineContext = SessionCatalogBrowserContext & {
  late: "matching" | "newer-notice";
};

type CatalogEntryReader = () => Promise<SessionCatalogRefreshEntry | undefined>;

export async function verifySessionCatalogDiscoveryDeadline(
  context: CatalogDeadlineContext,
): Promise<void> {
  const { page, api, seed, backend, touch, late, testInfo } = context;
  const loss = await routeSessionCatalogDiscoveryLoss(page);
  const fixture = await createSessionCatalogScenario(page, api, seed, backend);
  const entry = () =>
    page.evaluate<SessionCatalogRefreshEntry | undefined>(
      `window.__KANDEV_E2E_STORE__.getState().sessionCatalogRefresh.bySessionId[${JSON.stringify(fixture.sessionId)}]`,
    );
  let clockInstalled = false;
  try {
    const retained = await captureRestartBaseline(api, fixture.taskId, fixture.sessionId);
    const editor = await fixture.session.fillMessage("Keep input while discovery is unavailable");
    await page.clock.install({ time: new Date() });
    clockInstalled = true;
    writeSessionCatalogFile(fixture.catalogPath, "new-choices");
    await refreshSavedSessionCatalog(api, fixture.profile.id);
    const notice = page.getByTestId("session-catalog-refresh-notice");
    await expect(notice).toBeVisible();
    loss.hold(fixture.sessionId);
    const action = notice.getByRole("button", { name: "Refresh session", exact: true });
    await activate(action, touch);
    const stop = fixture.wire.waitForResponse("session.stop", {
      requestWhere: (payload) => payload.session_id === fixture.sessionId,
    });
    const launch = fixture.wire.waitForResponse("session.launch", {
      timeout: 30000,
      timeoutAfter: stop,
      requestWhere: (payload) =>
        payload.session_id === fixture.sessionId && payload.intent === "resume",
    });
    const confirm = page
      .getByTestId("session-catalog-refresh-confirmation")
      .getByRole("button", { name: "Refresh session", exact: true });
    await activate(confirm, touch);
    await Promise.all([stop, launch]);
    const acceptedOwner = sessionCatalogResumeAckSchema.parse(
      (await launch).payload,
    ).accepted_launch_owner;
    await expect.poll(() => loss.heldCount()).toBeGreaterThan(0);
    await expect.poll(async () => (await entry())?.checking?.status).toBe("checking");
    const afterAck = fixture.traffic.frames.length;
    await assertCatalogObservationTimeout(page, entry);
    await expect(notice).toBeVisible();
    await expect(action).toBeEnabled();
    await expect(editor).toContainText("Keep input while discovery is unavailable");
    await expect(fixture.session.activeChat().getByTestId("submit-message-button")).toBeEnabled();
    expect(catalogMutationFrames(fixture.traffic.frames.slice(afterAck))).toEqual([]);
    expect(fixture.httpMutations).toEqual([]);
    const name = `${touch ? "phone" : "desktop"}-catalog-no-report-${late}`;
    await testInfo.attach(name, {
      body: await page.screenshot({ path: testInfo.outputPath(`${name}.png`) }),
      contentType: "image/png",
    });

    const newerRevision = await settleLateCatalogDelivery(
      context,
      fixture,
      loss,
      entry,
      acceptedOwner,
    );
    await assertRestartRetained(api, fixture.taskId, fixture.sessionId, retained);
    assertCatalogNativeRetained(fixture);
    expect(catalogMutationFrames(fixture.traffic.frames.slice(afterAck))).toEqual([]);
    await page.reload();
    await fixture.session.waitForLoad();
    expect((await entry())?.checking).toBeUndefined();
    if (newerRevision) {
      expect((await entry())?.record?.revision).toBe(newerRevision);
      await expect(page.getByTestId("session-catalog-refresh-notice")).toBeVisible();
    }
  } finally {
    if (clockInstalled) await page.clock.resume();
    loss.release();
    await cleanupSessionCatalogScenario(api, fixture);
  }
}

async function assertCatalogObservationTimeout(page: Page, entry: CatalogEntryReader) {
  await page.waitForLoadState("networkidle");
  await page.clock.pauseAt(new Date(await page.evaluate(() => Date.now() + 1000)));
  const checking = (await entry())!.checking!;
  const remaining = checking.deadlineAt! - (await page.evaluate(() => Date.now()));
  expect(remaining).toBeGreaterThan(1);
  await page.clock.fastForward(remaining - 1);
  expect((await entry())?.checking?.status).toBe("checking");
  await page.clock.fastForward(1);
  expect((await entry())?.checking?.status).toBe("unavailable");
  await page.clock.resume();
}

async function publishSupersedingCatalogNotice(
  api: ApiClient,
  fixture: SessionCatalogScenario,
  entry: CatalogEntryReader,
): Promise<number> {
  writeSessionCatalogFile(fixture.catalogPath, "newer-choices");
  await refreshSavedSessionCatalog(api, fixture.profile.id);
  const revision = catalogRecord((await api.getTaskSession(fixture.sessionId)).session)!.revision;
  await expect.poll(async () => (await entry())?.record?.revision).toBe(revision);
  expect((await entry())?.record?.models?.map((model) => model.id)).toEqual([
    "catalog-newer",
    "mock-fast",
  ]);
  expect((await entry())?.checking).toBeUndefined();
  return revision;
}

async function settleLateCatalogDelivery(
  context: CatalogDeadlineContext,
  fixture: SessionCatalogScenario,
  loss: Awaited<ReturnType<typeof routeSessionCatalogDiscoveryLoss>>,
  entry: CatalogEntryReader,
  acceptedOwner: SessionCatalogOwner | undefined,
): Promise<number | undefined> {
  const newerRevision =
    context.late === "newer-notice"
      ? await publishSupersedingCatalogNotice(context.api, fixture, entry)
      : undefined;
  loss.release();
  await expect.poll(async () => (await entry())?.provider?.owner).toEqual(acceptedOwner);
  if (newerRevision) {
    expect((await entry())?.record?.revision).toBe(newerRevision);
    expect((await entry())?.record?.disposition).toBe("host_candidates");
  } else {
    await expect.poll(async () => (await entry())?.record?.disposition).toBe("cleared");
    expect((await entry())?.checking).toBeUndefined();
  }
  return newerRevision;
}
