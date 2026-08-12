"use client";

import { useState } from "react";

/**
 * Disclosure state for the queue panel. A pinned session starts open (the
 * caller renders nothing while the queue is empty, so the flag only becomes
 * visible once entries exist, including entries that arrive asynchronously
 * after mount); an unpinned session starts collapsed. The panel collapses on
 * a full drain and follows the target session's pin on session switch.
 * Collapsing via the header X closes it for the current view; a later mount
 * reopens it while the pin is retained. State is adjusted during render
 * (React docs: "Adjusting some state when a prop changes") to avoid the
 * cascading-render anti-pattern of doing it inside useEffect.
 */
export function useQueuePanelOpenState(
  sessionId: string | null,
  entryCount: number,
  pinned: boolean,
) {
  const [isOpen, setIsOpen] = useState(pinned);
  const [lastSession, setLastSession] = useState(sessionId);
  const [lastEntryCount, setLastEntryCount] = useState(entryCount);
  if (sessionId !== lastSession) {
    setLastSession(sessionId);
    setIsOpen(pinned);
  }
  if (entryCount !== lastEntryCount) {
    setLastEntryCount(entryCount);
    if (entryCount === 0) setIsOpen(false);
  }
  return [isOpen, setIsOpen] as const;
}
