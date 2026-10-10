import { test } from "../../fixtures/test-base";
import { exerciseLimitRecoverySettings } from "../../helpers/agent-limit-recovery-settings";

test("persists concrete profile limit recovery and dormant choices on desktop", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(90_000);
  await exerciseLimitRecoverySettings(testPage, apiClient, false);
});
