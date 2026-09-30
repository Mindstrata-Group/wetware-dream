// Utility for loading site_content from the public API.
//
// Architecture:
// - One fetch to /api/public/site-content for the whole page
// - Browser cache for 60 s (Cache-Control from the API) + stale-while-revalidate
// - Works on the server (SSR) too: called during render
//
// Usage:
//   const content = await loadSiteContent()
//   const title = getString(content, 'landing.hero.title', 'Default title')

import { apiBase } from './api'
import { clearApiCache } from './useApi'

export type SiteContent = Record<string, unknown>

let memo: { data: SiteContent; expiresAt: number } | null = null
const CACHE_MS = 60_000

export async function loadSiteContent(): Promise<SiteContent> {
  const now = Date.now()
  if (memo && now < memo.expiresAt) {
    return memo.data
  }
  try {
    // No cache:'no-store': the server Cache-Control (60s + SWR) lets the browser
    // reuse the response between navigations. memo above is the fast path within the current tab.
    const res = await fetch(`${apiBase}/api/public/site-content`)
    if (!res.ok) return memo?.data ?? {}
    const json = await res.json()
    const data = (json?.content ?? {}) as SiteContent
    memo = { data, expiresAt: now + CACHE_MS }
    return data
  } catch {
    return memo?.data ?? {}
  }
}

// Helpers with a fallback: if the key is not in the DB, return the default from the code.
export function getString(content: SiteContent, key: string, fallback: string): string {
  const v = content[key]
  return typeof v === 'string' ? v : fallback
}

export function getArray<T = unknown>(content: SiteContent, key: string, fallback: T[]): T[] {
  const v = content[key]
  return Array.isArray(v) ? (v as T[]) : fallback
}

export function getBoolean(content: SiteContent, key: string, fallback = false): boolean {
  const v = content[key]
  if (typeof v === 'boolean') return v
  if (typeof v === 'string') {
    const normalized = v.trim().toLowerCase()
    if (['1', 'true', 'yes', 'on', 'вкл'].includes(normalized)) return true
    if (['0', 'false', 'no', 'off', 'выкл'].includes(normalized)) return false
  }
  return fallback
}

export function getNumber(content: SiteContent, key: string, fallback: number): number {
  const v = content[key]
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'string') {
    const parsed = Number(v.trim().replace(',', '.'))
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

export type StorefrontFeature = 'blog' | 'shop'

export function isFeatureEnabled(content: SiteContent, feature: StorefrontFeature): boolean {
  return getBoolean(content, `features.${feature}_enabled`, false)
}

export function getStorefrontFeatureLinks(content: SiteContent): Array<{ href: string; label: string; feature: StorefrontFeature }> {
  return [
    ...(isFeatureEnabled(content, 'blog') ? [{
      href: '/blog',
      label: getString(content, 'landing.blog_button_label', 'Блог'),
      feature: 'blog' as const,
    }] : []),
    ...(isFeatureEnabled(content, 'shop') ? [{
      href: '/shop',
      label: getString(content, 'landing.shop_button_label', 'Магазин решений'),
      feature: 'shop' as const,
    }] : []),
  ]
}

// Clears the in-memory cache in the browser. Useful after an admin save.
export function clearSiteContentCache() {
  memo = null
  clearApiCache('/api/public/site-content')
}

// Substitution of variables like {{modes_count}}, {{paid_count}}, {{year}}.
// Used for templating admin-editable text.
export function fillVars(text: string, vars: Record<string, string | number>): string {
  return text.replace(/{{\s*([a-zA-Z_]+)\s*}}/g, (m, name: string) => {
    const v = vars[name] ?? vars[name.toLowerCase()]
    return v === undefined ? m : String(v)
  })
}
