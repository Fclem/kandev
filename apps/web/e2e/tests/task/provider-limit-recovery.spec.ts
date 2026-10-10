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
test("auto-start waits for a trusted provider reset and shows the waiting task state", async ({
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
    phone: false,
    testInfo,
  });
});

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-005.4
test("manual provider limit notice preserves the conversation and survives reload", async ({
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
    phone: false,
    testInfo,
  });
});

// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.1
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.4
// @covers AC-AGENTS-PROVIDER-LIMIT-RECOVERY-004.5
test("reset wait reloads, cancels and resumes once at the exact deadline", async ({
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
    phone: false,
    testInfo,
  });
});
