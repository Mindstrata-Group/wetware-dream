import { afterEach, describe, expect, it, vi } from 'vitest'
import { absolutePromoAccessUrl, formatBytes, formatDate, ghostBtnStyle, promoAccessUrl } from './_utils'

describe('promoAccessUrl', () => {
  it('builds relative URL with encoded promo code', () => {
    // kills: encodeURIComponent -> removed; the /access?promo= mutation
    expect(promoAccessUrl('HELLO')).toBe('/access?promo=HELLO')
    expect(promoAccessUrl('special code!')).toBe('/access?promo=special%20code!')
    expect(promoAccessUrl('a&b=c')).toBe('/access?promo=a%26b%3Dc')
  })
})

describe('absolutePromoAccessUrl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('resolves to absolute URL in browser context', () => {
    // kills: typeof window === "undefined" -> always returns rel
    const result = absolutePromoAccessUrl('TEST')
    expect(result).toMatch(/^http/)
    expect(result).toContain('/access?promo=TEST')
  })

  it('returns relative URL in SSR context (window undefined)', () => {
    // kills the typeof window === "undefined" -> false mutation
    vi.stubGlobal('window', undefined)
    expect(absolutePromoAccessUrl('CODE')).toBe('/access?promo=CODE')
  })
})

describe('formatDate (promo-admin)', () => {
  it('formats valid date as locale string', () => {
    const result = formatDate('2026-06-01T00:00:00Z')
    expect(result).not.toBe('—')
    expect(typeof result).toBe('string')
    expect(result.length).toBeGreaterThan(0)
  })

  it('returns dash for null/undefined', () => {
    // kills the value ? ... : "—" mutation
    expect(formatDate(null)).toBe('—')
    expect(formatDate(undefined)).toBe('—')
    expect(formatDate('')).toBe('—')
  })
})

describe('formatBytes', () => {
  it('converts bytes to КБ with ceil rounding', () => {
    // kills the Math.ceil -> Math.floor mutation
    expect(formatBytes(1024)).toBe('1 КБ')
    expect(formatBytes(1025)).toBe('2 КБ') // ceil: 1025/1024=1.0009 → ceil=2 (not 1)
    expect(formatBytes(2048)).toBe('2 КБ')
  })

  it('uses 0 as fallback when value is absent', () => {
    // kills: (value || 0) -> value (without the fallback, undefined/1024 -> NaN)
    expect(formatBytes(undefined)).toBe('0 КБ')
    expect(formatBytes(0)).toBe('0 КБ')
  })

  it('formats large values correctly', () => {
    // kills the / 1024 -> / 1023 mutation
    expect(formatBytes(10240)).toBe('10 КБ')
  })
})

describe('ghostBtnStyle', () => {
  it('returns smaller dimensions for sm size', () => {
    // kills the size === "sm" ? 30 : 36 mutation
    const sm = ghostBtnStyle('sm')
    const md = ghostBtnStyle('md')
    expect(sm.height).toBe(30)
    expect(md.height).toBe(36)
    expect(sm.fontSize).toBe(12)
    expect(md.fontSize).toBe(13)
  })

  it('returns correct padding for each size', () => {
    // kills the "0 12px" vs "0 14px" mutation
    expect(ghostBtnStyle('sm').padding).toBe('0 12px')
    expect(ghostBtnStyle('md').padding).toBe('0 14px')
  })

  it('includes fixed style properties regardless of size', () => {
    // kills the borderRadius: 8 -> 0 mutation
    const style = ghostBtnStyle('sm')
    expect(style.borderRadius).toBe(8)
    expect(style.fontWeight).toBe(500)
    expect(style.cursor).toBe('pointer')
  })
})
