import type { UsageTurn } from "@/lib/types/conversation-usage";

type UsageTurnIndexEntry = {
  byTurnId: Record<string, UsageTurn>;
  /** `usageInvalidation` counter the newest page was last refreshed at; unset = never refreshed. */
  refreshedInvalidation?: number;
  /** Each cached turn's accepted usage invalidation generation. */
  refreshedInvalidationByTurnId?: Record<string, number>;
  nextCursor?: string;
  exhausted: boolean;
};

export type UsageTurnIndexState = {
  bySessionId: Record<string, UsageTurnIndexEntry>;
};
