import { describe, expect, it } from "vitest";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { reviewItemId, selectReviewItem } from "./review-selection";

const githubReview: ReviewItemSummary = {
  providerId: "github",
  reviewKey: "owner/repository/12",
  title: "GitHub pull request",
  url: "https://github.test/owner/repository/pull/12",
  repositoryId: "owner/repository",
  state: "OPEN",
};

const bitbucketReview: ReviewItemSummary = {
  providerId: "bitbucket",
  reviewKey: "workspace/repository/42",
  title: "Bitbucket pull request",
  url: "https://bitbucket.test/workspace/repository/pull-requests/42",
  repositoryId: "workspace/repository",
  state: "OPEN",
};

describe("selectReviewItem", () => {
  it("requires an explicit choice when built-in and plugin reviews coexist", () => {
    expect(selectReviewItem([githubReview, bitbucketReview], null)).toBeNull();
  });

  it("selects the requested plugin review instead of the first built-in result", () => {
    expect(selectReviewItem([githubReview, bitbucketReview], reviewItemId(bitbucketReview))).toBe(
      bitbucketReview,
    );
  });

  it("opens a lone review without an unnecessary chooser", () => {
    expect(selectReviewItem([bitbucketReview], null)).toBe(bitbucketReview);
  });
});
