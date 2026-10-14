import { test } from "../../fixtures/test-base";
import { managedFallbackAwareness } from "./agent-runtime-fallback-helpers";
import { backendRuntimeUpdateSummary } from "./agent-runtime-summary-backend-helpers";
import { runtimeAwareness } from "./agent-runtime-notifications-helpers";
import { ompAutomaticConsent, ompAutomaticOutcomeRecovery } from "./omp-automatic-update-helpers";

test("backend runtime summary reconnect replay stays deduplicated on phone", async ({
  testPage,
  backend,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await backendRuntimeUpdateSummary(testPage, backend, apiClient, true);
});

test("phone runtime notices lead to saved policy, native guidance and version drawer", async ({
  testPage,
  prCapture,
}) => {
  await runtimeAwareness(testPage, true, prCapture);
});

test("native host keeps managed fallback version controls reachable", async ({
  testPage,
  prCapture,
}) => {
  await managedFallbackAwareness(testPage, true, prCapture);
});

test("phone OMP consent uses touch-sized Save, discard and unavailable opt-out", async ({
  testPage,
  prCapture,
}) => {
  await ompAutomaticConsent(testPage, true, prCapture);
});

test("phone OMP output and saved pause recovery retain one page scroll owner", async ({
  testPage,
  prCapture,
}) => {
  await ompAutomaticOutcomeRecovery(testPage, true, prCapture);
});
