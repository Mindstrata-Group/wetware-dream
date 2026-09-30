import type { CSSProperties } from 'react'
import { FONT_BODY, MS } from './_theme'

export function promoAccessUrl(promo: string) {
  return `/access?promo=${encodeURIComponent(promo)}`;
}
export function absolutePromoAccessUrl(promo: string) {
  const rel = promoAccessUrl(promo);
  if (typeof window === "undefined") return rel;
  return new URL(rel, window.location.origin).toString();
}
export function formatDate(value?: string | null) {
  return value ? new Date(value).toLocaleString("ru-RU") : "—";
}
export function formatBytes(value?: number) {
  return `${Math.ceil((value || 0) / 1024)} КБ`;
}
export function ghostBtnStyle(size: "sm" | "md"): CSSProperties {
  return {
    height: size === "sm" ? 30 : 36,
    padding: size === "sm" ? "0 12px" : "0 14px",
    borderRadius: 8,
    border: `1px solid ${MS.ink20}`,
    background: MS.surfaceSoft,
    color: MS.ink70,
    fontSize: size === "sm" ? 12 : 13,
    fontWeight: 500,
    cursor: "pointer",
    fontFamily: "inherit",
  };
}
