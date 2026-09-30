// SOLID-split of admin/page.tsx 2026-05-31: pure utilities, no React/JSX.

import type { ModeRow, Promo } from "./types";
import { grantLimitSliderMax, grantLimitTail } from "./constants";

export function grantLimitFromSlider(raw: string) {
  const position = Math.max(0, Math.min(grantLimitSliderMax, Number(raw) || 0));
  if (position <= 98) return position + 2;
  return (
    grantLimitTail[position - 99] || grantLimitTail[grantLimitTail.length - 1]
  );
}

export function grantSliderFromLimit(raw: string) {
  const limit = Math.max(2, Number(raw) || 2);
  if (limit <= 100) return String(Math.round(limit - 2));
  let bestIndex = 0;
  let bestDelta = Number.POSITIVE_INFINITY;
  grantLimitTail.forEach((value, index) => {
    const delta = Math.abs(value - limit);
    if (delta < bestDelta) {
      bestDelta = delta;
      bestIndex = index;
    }
  });
  return String(99 + bestIndex);
}

export function normalizeGrantLimit(raw: string) {
  const value = Math.floor(Number(raw) || 2);
  return String(Math.max(2, value));
}

export function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString() : "—";
}

export function statusHelp(status: string) {
  return status === "blocked"
    ? "blocked — вход заблокирован"
    : "active — пользователь может работать";
}

export function passwordScore(password: string) {
  return [
    password.length >= 10,
    /[a-zа-я]/i.test(password),
    /\d/.test(password),
    /[^a-zа-я0-9]/i.test(password),
  ].filter(Boolean).length;
}

export function selectedNumberValues(options: HTMLCollectionOf<HTMLOptionElement>) {
  return Array.from(options)
    .filter((o) => o.selected)
    .map((o) => Number(o.value))
    .filter(Boolean);
}

export function toggleNumberSelection(list: number[], id: number) {
  return list.includes(id) ? list.filter((item) => item !== id) : [...list, id];
}

export function toLocalDateInput(d: Date) {
  const copy = new Date(d);
  copy.setMinutes(copy.getMinutes() - copy.getTimezoneOffset());
  return copy.toISOString().slice(0, 10);
}

export function todayLocalDate() {
  return toLocalDateInput(new Date());
}

export function yesterdayLocalDate() {
  const d = new Date();
  d.setDate(d.getDate() - 1);
  return toLocalDateInput(d);
}

export function weekAgoLocalDate() {
  const d = new Date();
  d.setDate(d.getDate() - 7);
  return toLocalDateInput(d);
}

export function monthAheadLocalDate() {
  const d = new Date();
  d.setMonth(d.getMonth() + 1);
  return toLocalDateInput(d);
}

export function monthBeforeLocalDate() {
  const d = new Date();
  d.setMonth(d.getMonth() - 1);
  return toLocalDateInput(d);
}

export function exportLimitLabel(value: string) {
  return Number(value) >= 1000001 ? "∞" : value;
}

export function modeNamesByIds(modes: ModeRow[], ids?: number[]) {
  const names = modes
    .filter((m) => (ids || []).includes(m.id))
    .map((m) => m.name);
  return names.length ? names.join(", ") : "режимы не выбраны";
}

export function fullClientUrl(url: string) {
  if (typeof window === "undefined") return url;
  try {
    return new URL(url, window.location.origin).toString();
  } catch {
    return url;
  }
}

export function promoStateLabel(p: Promo) {
  if ((p.maxUses || 0) > 0 && (p.usedCount || 0) >= (p.maxUses || 0)) return "исчерпан по числу активаций";
  if (p.activeTo && new Date(p.activeTo).getTime() < Date.now()) return "истёк срок окна активации";
  return p.active ? "активен" : "неактивен";
}

export function promoIsExhaustedMultiUse(p: Promo) {
  return (p.maxUses || 0) > 1 && (p.usedCount || 0) >= (p.maxUses || 0);
}
