// SOLID-split of page.tsx (landing) 2026-05-31: pure utilities.

import type { DemoMessage, DemoMode } from "./types";

export function parseDemoChat(mode: DemoMode): DemoMessage[] {
  const raw = (mode.demoChat || mode.demo_chat || "").trim();
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    const source = Array.isArray(parsed)
      ? parsed
      : Array.isArray(parsed?.messages)
        ? parsed.messages
        : [];
    const msgs = source
      .map((item: Record<string, string>) => ({
        role: (item.role === "assistant" ? "assistant" : "user") as "user" | "assistant",
        content: String(item.content || item.text || "").replace(/\\n/g, "\n").trim(),
      }))
      .filter((m: DemoMessage) => m.content);
    if (msgs.length) return msgs;
  } catch { /* fall through */ }
  return [];
}
