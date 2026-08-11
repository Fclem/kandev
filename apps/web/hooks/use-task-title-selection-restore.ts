"use client";

import { useCallback, useLayoutEffect, useRef } from "react";
import type { ChangeEvent } from "react";
import { clampTaskTitleInput } from "@/lib/task-title";

type TitleInputElement = HTMLInputElement | HTMLTextAreaElement;

type PendingSelection = { start: number; end: number };

/**
 * Keeps the caret in place while a task-title field clamps its value at the
 * 60-character cap.
 *
 * `clampChange` truncates the typed value (dropping the tail beyond the cap)
 * and records where the caret was. Because a truncated value differs from the
 * DOM's current value, React rewrites the DOM and the browser resets the caret
 * to the end of the field; the layout effect re-pins the caret to the recorded
 * position (bounded by the clamped length) after that commit.
 *
 * The record is cleared on every non-truncating change and whenever the
 * clamped result equals the last committed value, so a stale caret from a
 * keystroke that never committed (typing at the very end while at the cap, or
 * typing a character that leaves the clamped value unchanged) cannot be
 * replayed by a later commit.
 */
export function useTaskTitleSelectionRestore<T extends TitleInputElement = HTMLInputElement>(
  value: string,
) {
  const inputRef = useRef<T | null>(null);
  const pendingSelectionRef = useRef<PendingSelection | null>(null);
  const lastCommittedRef = useRef(value);

  const clampChange = useCallback((e: ChangeEvent<TitleInputElement>) => {
    const el = e.target;
    const next = clampTaskTitleInput(el.value);
    // Only record when the commit will actually change the value. If `next`
    // equals the last committed value, `setValue` bails out of the render, no
    // layout effect runs, and a recorded selection would linger until an
    // unrelated later commit.
    pendingSelectionRef.current =
      next !== el.value && next !== lastCommittedRef.current
        ? { start: el.selectionStart ?? el.value.length, end: el.selectionEnd ?? el.value.length }
        : null;
    return next;
  }, []);

  useLayoutEffect(() => {
    lastCommittedRef.current = value;
    const selection = pendingSelectionRef.current;
    pendingSelectionRef.current = null;
    if (!selection) return;
    const el = inputRef.current;
    if (!el || document.activeElement !== el) return;
    const max = value.length;
    el.setSelectionRange(Math.min(selection.start, max), Math.min(selection.end, max));
  }, [value]);

  return { inputRef, clampChange };
}
