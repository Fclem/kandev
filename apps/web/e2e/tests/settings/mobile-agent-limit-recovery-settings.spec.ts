import { test } from "../../fixtures/test-base";
import { exerciseLimitRecoverySettings } from "../../helpers/agent-limit-recovery-settings";

test("persists concrete profile limit recovery with touch help and no overflow", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(90_000);
  await exerciseLimitRecoverySettings(testPage, apiClient, true);
});
