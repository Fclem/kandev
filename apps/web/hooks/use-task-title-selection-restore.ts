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
 * The record is cleared on every non-truncating change so a stale caret from
 * a keystroke that never committed (typing at the very end while at the cap)
 * cannot be replayed by a later commit.
 *
 * When a truncating keystroke leaves the clamped value equal to the last
 * committed value (e.g. typing the same character into an all-same-char title
 * at the cap), `setValue` bails out of the render, so no layout effect runs —
 * but React still restores the controlled DOM value after the event, which
 * resets the caret to the end. That case is handled with an immediate
 * microtask restore instead of the commit-driven path.
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
    if (next !== el.value) {
      if (next !== lastCommittedRef.current) {
        // The commit will change the value: record the caret for the layout
        // effect, which runs after React rewrites the DOM.
        pendingSelectionRef.current = {
          start: el.selectionStart ?? el.value.length,
          end: el.selectionEnd ?? el.value.length,
        };
      } else {
        // The clamped value equals the committed value: the render bails out,
        // but React still restores the controlled DOM value after the event
        // and the browser resets the caret to the end. Re-pin it after that
        // write via a microtask; there is no commit to hook into.
        const start = el.selectionStart ?? el.value.length;
        const end = el.selectionEnd ?? el.value.length;
        const max = next.length;
        queueMicrotask(() => {
          if (el.isConnected && document.activeElement === el) {
            el.setSelectionRange(Math.min(start, max), Math.min(end, max));
          }
        });
      }
    } else {
      pendingSelectionRef.current = null;
    }
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
