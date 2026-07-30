import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import { t } from "@/lib/i18n";

export type AzureDevOpsPullRequestPresentation = {
  provider: "azure_devops";
  label: string;
  tone: "success" | "danger" | "warning" | "muted" | "info";
};

export function getAzureDevOpsPullRequestPresentation(
  pullRequest: AzureDevOpsTaskPullRequest,
): AzureDevOpsPullRequestPresentation {
  const status = pullRequest.status.toLowerCase();
  if (status === "completed") {
    return { provider: "azure_devops", label: t("azureDevops:completed"), tone: "success" };
  }
  if (status === "abandoned") {
    return { provider: "azure_devops", label: t("azureDevops:abandoned"), tone: "muted" };
  }
  if (pullRequest.policyState === "failure") {
    return { provider: "azure_devops", label: t("azureDevops:policyFailed"), tone: "danger" };
  }
  if (pullRequest.reviewState === "rejected") {
    return { provider: "azure_devops", label: t("azureDevops:changesRequested"), tone: "danger" };
  }
  if (pullRequest.isDraft) {
    return { provider: "azure_devops", label: t("azureDevops:draft"), tone: "muted" };
  }
  if (pullRequest.policyState === "pending") {
    return { provider: "azure_devops", label: t("azureDevops:policyRunning"), tone: "warning" };
  }
  if (pullRequest.reviewState === "waiting") {
    return { provider: "azure_devops", label: t("azureDevops:waitingForReview"), tone: "warning" };
  }
  if (pullRequest.reviewState === "approved" && pullRequest.policyState === "success") {
    return { provider: "azure_devops", label: t("azureDevops:ready"), tone: "success" };
  }
  return { provider: "azure_devops", label: t("azureDevops:active"), tone: "info" };
}
