"use client";

import { memo } from "react";
import type { Message } from "@/lib/types/http";
import { kandevToolStemOf, type ToolCallMetadata } from "@/components/task/chat/types";
import { extractKandevArgs, extractMcpResult } from "./kandev/parse";
import { getKandevRenderer } from "./kandev/registry";
import { hasKandevRenderer } from "./kandev/has-renderer";
import { PermissionActionRow } from "./permission-action-row";
import { normalizeToolCallStatus } from "./tool-status";
import {
  parsePermission,
  resolvePermissionAvailability,
  usePermissionResponseHandlers,
} from "./use-permission-handlers";
import { KandevPermissionUIProvider, type KandevPermissionUIState } from "./kandev/shared";
import type { PermissionRequestMetadata } from "./use-permission-handlers";
export { hasKandevRenderer };

// Approval falls through to `undefined` on purpose: the tool_call status
// stays the source of truth so an approved-but-still-pending tool does not
// render as complete and a later tool_call error is not masked.
function derivePermissionUI(
  permissionStatus: PermissionRequestMetadata["status"],
  isPermissionPending: boolean,
): KandevPermissionUIState {
  if (isPermissionPending) return "pending";
  if (permissionStatus === "rejected") return "rejected";
  return undefined;
}

type KandevToolMessageProps = {
  comment: Message;
  permissionMessage?: Message;
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
};

// KandevToolMessage is the rendered entry point for every Kandev tool call.
// It parses metadata once and invokes the registered renderer with normalized
// arguments and results.
export const KandevToolMessage = memo(function KandevToolMessage({
  comment,
  permissionMessage,
  sessionId,
  onOpenFile,
}: KandevToolMessageProps) {
  const meta = comment.metadata as ToolCallMetadata | undefined;
  const renderer = getKandevRenderer(kandevToolStemOf(comment));
  const { permissionMetadata, permissionStatus, isPermissionPending } =
    parsePermission(permissionMessage);
  const {
    isResponding,
    isUnavailable,
    handleApprove,
    handleAllowAlways,
    hasAllowAlways,
    handleReject,
    offeredChoices,
    handleOfferedChoice,
  } = usePermissionResponseHandlers({
    permissionMetadata,
    permissionMessage,
  });
  if (!renderer) return null;

  // The ACP normalizer stores MCP tool args/result inside the Generic payload:
  // `generic.input` holds the call args, `generic.output` the MCP result. We
  // also accept `metadata.args` / `metadata.result` as fallbacks so the
  // component still works for any code path that hasn't routed through the
  // normalizer yet.
  const generic = meta?.normalized?.generic;
  const args = extractKandevArgs(generic?.input) ?? extractKandevArgs(meta?.args);
  const rawResult = generic?.output ?? meta?.result;
  const result = extractMcpResult(rawResult);
  const status = normalizeToolCallStatus(meta?.status);

  const {
    permissionStatus: effectivePermissionStatus,
    isPermissionPending: effectivePermissionPending,
  } = resolvePermissionAvailability(permissionStatus, isPermissionPending, isUnavailable);
  const permissionUI = derivePermissionUI(effectivePermissionStatus, effectivePermissionPending);

  // Each renderer is a stable function-pointer pulled from the static
  // registry, so invoking it like a function (rather than via JSX) is safe
  // and avoids the lint rule against "components created during render".
  const rendered = (
    <KandevPermissionUIProvider value={permissionUI}>
      {renderer({ args, result, status, sessionId, messageId: comment.id, onOpenFile })}
    </KandevPermissionUIProvider>
  );

  if (!effectivePermissionPending) return rendered;

  return (
    <>
      {rendered}
      <div className="mt-2 ml-7" data-testid="kandev-tool-permission">
        <PermissionActionRow
          onApprove={handleApprove}
          onReject={handleReject}
          onAllowAlways={hasAllowAlways ? handleAllowAlways : undefined}
          offeredChoices={offeredChoices}
          onChooseOfferedChoice={handleOfferedChoice}
          isResponding={isResponding}
        />
      </div>
    </>
  );
});
