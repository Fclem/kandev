"use client";

import { useCallback, useState } from "react";
import type { ReviewItemSummary } from "@/lib/plugins/types";

type ReviewSelection = {
  taskId: string | null;
  reviewId: string | null;
};

export function reviewItemId(review: Pick<ReviewItemSummary, "providerId" | "reviewKey">): string {
  return `${review.providerId}:${review.reviewKey}`;
}

/**
 * A single review is the only unambiguous default. Multiple reviews require an
 * explicit user choice so a newly registered provider cannot be hidden behind
 * an earlier GitHub or GitLab result.
 */
export function selectReviewItem(
  reviews: readonly ReviewItemSummary[],
  selectedReviewId: string | null,
): ReviewItemSummary | null {
  if (reviews.length === 1) return reviews[0] ?? null;
  if (!selectedReviewId) return null;
  return reviews.find((review) => reviewItemId(review) === selectedReviewId) ?? null;
}

export function useReviewItemSelection(
  taskId: string | null,
  reviews: readonly ReviewItemSummary[],
) {
  const [selection, setSelection] = useState<ReviewSelection>({ taskId, reviewId: null });
  if (selection.taskId !== taskId) setSelection({ taskId, reviewId: null });

  const selectedReview = selectReviewItem(reviews, selection.reviewId);
  const selectReview = useCallback(
    (review: ReviewItemSummary) => {
      setSelection({ taskId, reviewId: reviewItemId(review) });
    },
    [taskId],
  );
  return { selectedReview, selectReview };
}
