import { useEffect, useRef } from "react";

import { useAppStore } from "@/components/state-provider";
import { selectCommandCount } from "@/lib/state/slices/session/selectors";
import type { TaskComment, TaskSession } from "@/app/office/tasks/[id]/types";

const AUTOSCROLL_THRESHOLD_PX = 80;

// isAtBottom reports whether the chat scroll container is close enough to its
// bottom edge that new content should keep following.
function isAtBottom(scrollParent: HTMLElement | null): boolean {
  if (!scrollParent) return true; // window scroll case — be conservative.
  const remaining = scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight;
  return remaining <= AUTOSCROLL_THRESHOLD_PX;
}

// scrollToBottom pins the chat scroll container to its latest content.
function scrollToBottom(scrollParent: HTMLElement | null): void {
  if (!scrollParent) return;
  scrollParent.scrollTop = scrollParent.scrollHeight;
}

/**
 * Auto-scroll the chat container to the bottom when new content arrives,
 * but only if the user was already near the bottom (within ~80px) at the
 * time of the change.
 *
 * Triggers on:
 *   - a new active session entry first appearing (active count grows)
 *   - new messages arriving in any session for this task
 *
 * Uses a scroll listener to track the user's "at-bottom" intent. Reads
 * the latest value before scrolling so we never yank focus from a user
 * who has scrolled up.
 */
export function useChatAutoScroll(
  scrollParent: HTMLElement | null,
  sessions: TaskSession[],
  taskId: string,
): void {
  const activeSessionCount = sessions.filter(
    (s) => s.state === "RUNNING" || s.state === "WAITING_FOR_INPUT",
  ).length;

  // Sum messages + command counts across all task sessions — single scalar
  // that grows whenever new content streams in.
  const totalContentSignal = useAppStore((s) => {
    let sum = 0;
    for (const session of sessions) {
      sum += s.messages.bySession[session.id]?.length ?? 0;
      sum += selectCommandCount(s, session.id);
    }
    return sum;
  });

  const wasAtBottomRef = useRef(true);

  useEffect(() => {
    if (!scrollParent) return;
    const handler = () => {
      wasAtBottomRef.current = isAtBottom(scrollParent);
    };
    handler();
    scrollParent.addEventListener("scroll", handler, { passive: true });
    return () => scrollParent.removeEventListener("scroll", handler);
  }, [scrollParent]);

  useEffect(() => {
    if (wasAtBottomRef.current) {
      scrollToBottom(scrollParent);
      // After programmatic scroll, we are still "at bottom" by definition.
      wasAtBottomRef.current = true;
    }
  }, [scrollParent, activeSessionCount, totalContentSignal, taskId]);
}

/**
 * Scrolls the comment matching `location.hash` (e.g. `#comment-cm-A`)
 * into view once it has rendered. Runs whenever the comments list
 * changes so deeplinks land on the target even when comments load
 * after first paint. Cleared after first match so it doesn't fight
 * the user when they scroll away.
 */
export function useCommentHashScroll(comments: TaskComment[]): void {
  const targetIdRef = useRef<string | null>(null);
  useEffect(() => {
    if (typeof window === "undefined") return;
    const hash = window.location.hash;
    if (!hash.startsWith("#comment-")) return;
    targetIdRef.current = hash.slice(1);
  }, []);

  useEffect(() => {
    const targetId = targetIdRef.current;
    if (!targetId) return;
    const el = document.getElementById(targetId);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    targetIdRef.current = null;
  }, [comments]);
}
