import { test } from "../../fixtures/test-base";
import {
  exerciseProviderLimitAutomaticLaunchWait,
  exerciseProviderLimitManualNotice,
  exerciseProviderLimitWait,
} from "../../helpers/provider-limit-recovery";

test.afterAll(async ({ backend }) => {
  await backend.restart();
});

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.2
test("auto-start waits for a provider reset on a phone", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(120_000);
  await exerciseProviderLimitAutomaticLaunchWait({
    page: testPage,
    apiClient,
    seedData,
    backend,
    phone: true,
    testInfo,
  });
});

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4
test("manual provider limit notice remains usable on a phone after reload", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(120_000);
  await exerciseProviderLimitManualNotice({
    page: testPage,
    apiClient,
    seedData,
    backend,
    phone: true,
    testInfo,
  });
});

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
test("reset wait cancel and exact-deadline resume remain usable on a phone", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}, testInfo) => {
  test.setTimeout(120_000);
  await exerciseProviderLimitWait({
    page: testPage,
    apiClient,
    seedData,
    backend,
    phone: true,
    testInfo,
  });
});
