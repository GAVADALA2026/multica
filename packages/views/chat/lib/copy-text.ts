import type { ChatMessage } from "@multica/core/types";
import type { ChatTimelineItem } from "@multica/core/chat";
import { stripChatQuickActionsProtocol } from "./quick-actions";

/**
 * Split an assistant timeline into three regions for the conductor-style fold:
 *   preface — text items before the first thinking/tool/error item
 *   middle  — everything from the first to the last non-text item (inclusive),
 *             including any text items sandwiched between them
 *   final   — text items after the last non-text item
 *
 * UI renders preface above the outer fold, middle inside the fold (with each
 * row keeping its existing inner Collapsible), and final below the fold.
 * Copy concatenates preface + final — the fold's contents are intentionally
 * omitted, mirroring what's visible when the fold is closed.
 */
export function splitTimeline(items: ChatTimelineItem[]): {
  preface: ChatTimelineItem[];
  middle: ChatTimelineItem[];
  final: ChatTimelineItem[];
} {
  const firstNonTextIdx = items.findIndex((i) => i.type !== "text");
  if (firstNonTextIdx === -1) {
    return { preface: [], middle: [], final: items };
  }
  let lastNonTextIdx = items.length - 1;
  while (lastNonTextIdx >= 0 && items[lastNonTextIdx]!.type === "text") {
    lastNonTextIdx--;
  }
  return {
    preface: items.slice(0, firstNonTextIdx),
    middle: items.slice(firstNonTextIdx, lastNonTextIdx + 1),
    final: items.slice(lastNonTextIdx + 1),
  };
}

/**
 * Markdown source the Copy action puts on the clipboard. A persisted
 * chat_message is the canonical completed answer; task messages are only the
 * execution transcript and may split one answer around thinking/tool events.
 * Surface-specific transforms still apply so hidden protocols stay out of the
 * rendered and copied answer.
 */
export function extractCopyText(
  message: ChatMessage,
  transformContent?: (content: string) => string,
): string {
  const content = stripChatQuickActionsProtocol(message.content ?? "");
  return transformContent ? transformContent(content) : content;
}
