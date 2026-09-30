// SOLID-split of chat/page.tsx 2026-05-31: pure quota/mode/messages utilities.

import type { DailyQuota, Message, Mode } from "./types";
import { publicApiBase } from "@/lib/apiBase";

export const apiBase = publicApiBase;
export const MODE_SWITCH_TRACE_PREFIX = "[mode-switch] ";

export function formatQuota(quota?: DailyQuota | null) {
  if (!quota) return "Лимит: —";
  if (typeof quota.remaining === "number" && typeof quota.limit === "number") return `Осталось ${quota.remaining} из ${quota.limit}`;
  if (typeof quota.limit === "number") return `Осталось ${Math.max(quota.limit - quota.used, 0)} из ${quota.limit}`;
  return `Использовано ${quota.used}`;
}

export function quotaExhausted(quota?: DailyQuota | null) {
  return Boolean(quota && typeof quota.remaining === "number" && quota.remaining <= 0);
}

export type QuotaWarningSettings = {
  enabled?: boolean;
  thresholdRemaining?: number;
  thresholdPercent?: number;
  title: string;
  body: string;
}

export type QuotaWarning = {
  title: string;
  body: string;
  remaining: number;
  limit?: number;
}

export function quotaRemaining(quota?: DailyQuota | null): number | null {
  if (!quota) return null;
  if (typeof quota.remaining === "number") return quota.remaining;
  if (typeof quota.limit === "number" && typeof quota.used === "number") return Math.max(quota.limit - quota.used, 0);
  return null;
}

export function quotaNearLimit(quota: DailyQuota | null | undefined, settings: Pick<QuotaWarningSettings, "enabled" | "thresholdRemaining" | "thresholdPercent">): boolean {
  if (settings.enabled === false || quotaExhausted(quota)) return false;
  const remaining = quotaRemaining(quota);
  if (remaining === null || remaining <= 0) return false;
  const absolute = Math.max(1, Math.floor(settings.thresholdRemaining ?? 3));
  const percent = typeof quota?.limit === "number"
    ? Math.max(1, Math.ceil(quota.limit * Math.max(0, settings.thresholdPercent ?? 20) / 100))
    : 0;
  return remaining <= Math.max(absolute, percent);
}

export function buildQuotaWarning(quota: DailyQuota | null | undefined, settings: QuotaWarningSettings): QuotaWarning | null {
  if (!quotaNearLimit(quota, settings)) return null;
  const remaining = quotaRemaining(quota);
  if (remaining === null) return null;
  const vars = {
    remaining,
    limit: quota?.limit ?? "",
    used: quota?.used ?? "",
  };
  return {
    title: fillSimpleVars(settings.title, vars),
    body: fillSimpleVars(settings.body, vars),
    remaining,
    limit: quota?.limit,
  };
}

function fillSimpleVars(text: string, vars: Record<string, string | number>) {
  return text.replace(/{{\s*([a-zA-Z_]+)\s*}}/g, (match, key: string) => {
    const value = vars[key] ?? vars[key.toLowerCase()];
    return value === undefined ? match : String(value);
  });
}

export function modeHasMessages(mode: Mode) {
  return !quotaExhausted(mode.quota);
}

export function selectableModeIds(modes: Mode[]) {
  return modes.filter(modeHasMessages).map(m => m.id);
}

export function syncModesWithGlobalUsage(modes: Mode[], globalQuota: DailyQuota): Mode[] {
  const incomingUsed = typeof globalQuota.used === "number" ? globalQuota.used : 0;
  return modes.map(mode => {
    const limit = mode.quota?.limit ?? globalQuota.limit;
    if (typeof limit !== "number") return { ...mode, quota: { ...globalQuota, used: incomingUsed } };
    return { ...mode, quota: { limit, used: incomingUsed, remaining: Math.max(limit - incomingUsed, 0) } };
  });
}

export function dailyUnlockLabel() {
  const now = new Date();
  const next = new Date(now);
  next.setHours(24, 0, 0, 0);
  const minutes = Math.max(1, Math.ceil((next.getTime() - now.getTime()) / 60000));
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours <= 0) return `${rest} мин`;
  if (rest === 0) return `${hours} ч`;
  return `${hours} ч ${rest} мин`;
}

export function cleanOutgoingMessage(raw: string) {
  return raw
    .replace(/[\u200B-\u200D\uFEFF]/g, "")
    .split("\n")
    .map(l => l.trim())
    .filter((l, i, arr) => l || (i > 0 && arr[i - 1]))
    .join("\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString() : "—";
}

export function modeSwitchTraceContent(fromModeName: string, toModeName: string) {
  const from = fromModeName.trim() || "Базовый ИИ";
  const to = toModeName.trim() || "Базовый ИИ";
  return `${MODE_SWITCH_TRACE_PREFIX}Стратум переключил режим: «${from}» → «${to}».`;
}

export function displaySystemMessage(content: string) {
  return content.trim().startsWith(MODE_SWITCH_TRACE_PREFIX)
    ? content.trim().slice(MODE_SWITCH_TRACE_PREFIX.length)
    : content;
}

export function isModeSwitchTraceMessage(message: Pick<Message, "role" | "content">) {
  return message.role === "system" && message.content.trim().startsWith(MODE_SWITCH_TRACE_PREFIX);
}

export function replaceTrailingModeSwitchTrace(messages: Message[], trace: Message) {
  const next = [...messages];
  if (next.length > 0 && isModeSwitchTraceMessage(next[next.length - 1])) {
    next.pop();
  }
  next.push(trace);
  return next;
}

export function prependUniqueMessages(incoming: Message[], current: Message[]) {
  const seenIds = new Set(current.map(message => message.id));
  const uniqueIncoming: Message[] = [];
  for (const message of incoming) {
    if (seenIds.has(message.id)) continue;
    seenIds.add(message.id);
    uniqueIncoming.push(message);
  }
  return [...uniqueIncoming, ...current];
}

export function collapseAdjacentModeSwitchTraces(messages: Message[]) {
  const result: Message[] = [];
  for (const message of messages) {
    if (
      isModeSwitchTraceMessage(message) &&
      result.length > 0 &&
      isModeSwitchTraceMessage(result[result.length - 1])
    ) {
      result[result.length - 1] = message;
      continue;
    }
    result.push(message);
  }
  return result;
}
