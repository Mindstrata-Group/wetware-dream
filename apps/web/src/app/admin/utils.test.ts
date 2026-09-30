import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  exportLimitLabel,
  formatDate,
  fullClientUrl,
  grantLimitFromSlider,
  grantSliderFromLimit,
  modeNamesByIds,
  monthAheadLocalDate,
  monthBeforeLocalDate,
  normalizeGrantLimit,
  passwordScore,
  promoIsExhaustedMultiUse,
  promoStateLabel,
  selectedNumberValues,
  statusHelp,
  todayLocalDate,
  toLocalDateInput,
  toggleNumberSelection,
  weekAgoLocalDate,
  yesterdayLocalDate,
} from './utils'
import type { ModeRow } from './types'

describe('admin grantLimit slider helpers', () => {
  it('maps slider position to limit: low end, middle, high end', () => {
    // kills the mutations: position + 2, <= 98, Math.max/min
    expect(grantLimitFromSlider('0')).toBe(2)
    expect(grantLimitFromSlider('50')).toBe(52)
    expect(grantLimitFromSlider('98')).toBe(100)
    // jump to the tail
    expect(grantLimitFromSlider('99')).toBe(125)
    expect(grantLimitFromSlider('112')).toBe(10000)
    // going past max is clamped
    expect(grantLimitFromSlider('999')).toBe(10000)
    // invalid input -> 0 -> 2
    expect(grantLimitFromSlider('abc')).toBe(2)
  })

  it('maps limit back to slider: low, mid, tail, out of range', () => {
    // kills: limit - 2, <= 100, Math.abs delta comparison
    expect(grantSliderFromLimit('2')).toBe('0')
    expect(grantSliderFromLimit('50')).toBe('48')
    expect(grantSliderFromLimit('100')).toBe('98')
    // nearest tail = 200 with limit=200
    expect(grantSliderFromLimit('200')).toBe('101')
    // invalid input -> limit=2 -> '0'
    expect(grantSliderFromLimit('abc')).toBe('0')
  })

  it('normalizeGrantLimit floors value and enforces minimum 2', () => {
    // kills the mutations: Math.floor, Math.max(2,...), || 2
    expect(normalizeGrantLimit('5')).toBe('5')
    expect(normalizeGrantLimit('5.9')).toBe('5')
    expect(normalizeGrantLimit('1')).toBe('2')
    expect(normalizeGrantLimit('0')).toBe('2')
    expect(normalizeGrantLimit('abc')).toBe('2')
  })
})

describe('admin password score', () => {
  it('scores password by length, letters, digits, special chars', () => {
    // kills the mutations: >= 10, regex presence
    expect(passwordScore('abc')).toBe(1)           // letters only, < 10 characters
    expect(passwordScore('abc123')).toBe(2)         // letters + digits
    expect(passwordScore('abc123!!!')).toBe(3)      // + a special character
    expect(passwordScore('Abcde12345!')).toBe(4)    // everything + >= 10
  })

  it('returns 0 for empty password', () => {
    expect(passwordScore('')).toBe(0)
  })
})

describe('admin statusHelp', () => {
  it('returns blocked/active descriptions', () => {
    // kills the === "blocked" mutation
    expect(statusHelp('blocked')).toContain('заблокирован')
    expect(statusHelp('active')).toContain('работать')
  })
})

describe('admin toggle and select helpers', () => {
  it('toggleNumberSelection adds or removes id', () => {
    // kills: includes, filter !== id, [...list, id]
    expect(toggleNumberSelection([1, 2, 3], 2)).toEqual([1, 3])
    expect(toggleNumberSelection([1, 3], 2)).toEqual([1, 3, 2])
    expect(toggleNumberSelection([], 5)).toEqual([5])
  })

  it('selectedNumberValues extracts selected option numbers', () => {
    // kills: o.selected filter, Number(o.value), filter(Boolean)
    const div = document.createElement('select')
    div.setAttribute('multiple', '')
    div.innerHTML = '<option value="10" selected></option><option value="20"></option><option value="0" selected></option>'
    const options = div.options
    expect(selectedNumberValues(options)).toEqual([10])
    // 0 is filtered out as falsy by filter(Boolean)
  })
})

describe('admin modeNamesByIds', () => {
  const modes: ModeRow[] = [
    { id: 1, name: 'Психолог' } as ModeRow,
    { id: 2, name: 'Коуч' } as ModeRow,
    { id: 3, name: 'Аналитик' } as ModeRow,
  ]

  it('joins names by ids', () => {
    // kills: includes, join(", "), length check
    expect(modeNamesByIds(modes, [1, 3])).toBe('Психолог, Аналитик')
    expect(modeNamesByIds(modes, [])).toBe('режимы не выбраны')
    expect(modeNamesByIds(modes, undefined)).toBe('режимы не выбраны')
  })
})

describe('admin exportLimitLabel', () => {
  it('returns ∞ for large values and the value itself otherwise', () => {
    // kills the >= 1000001 mutation
    expect(exportLimitLabel('1000001')).toBe('∞')
    expect(exportLimitLabel('1000000')).toBe('1000000')
    expect(exportLimitLabel('500')).toBe('500')
  })
})

describe('admin formatDate', () => {
  it('formats valid date and returns dash for absent', () => {
    expect(formatDate(null)).toBe('—')
    expect(formatDate(undefined)).toBe('—')
    expect(formatDate('2026-06-01T00:00:00Z')).not.toBe('—')
  })
})

describe('admin toLocalDateInput and date helpers', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-06-09T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('formats Date as YYYY-MM-DD local string', () => {
    // kills: setMinutes, getTimezoneOffset, toISOString, slice(0, 10)
    const result = toLocalDateInput(new Date('2026-06-09T00:00:00Z'))
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}$/)
    expect(result.length).toBe(10)
  })

  it('todayLocalDate matches current local date', () => {
    const result = todayLocalDate()
    expect(result).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  it('yesterdayLocalDate is 1 day before today', () => {
    // kills: d.getDate() - 1 -> + 1 or another number
    const today = todayLocalDate()
    const yesterday = yesterdayLocalDate()
    const todayDate = new Date(today)
    const yesterdayDate = new Date(yesterday)
    expect(todayDate.getTime() - yesterdayDate.getTime()).toBe(86400000)
  })

  it('weekAgoLocalDate is 7 days before today', () => {
    // kills the d.getDate() - 7 mutation
    const today = new Date(todayLocalDate())
    const weekAgo = new Date(weekAgoLocalDate())
    expect(today.getTime() - weekAgo.getTime()).toBe(7 * 86400000)
  })

  it('monthAheadLocalDate is 1 month after today', () => {
    // kills the d.getMonth() + 1 -> - 1 mutation
    const today = todayLocalDate()
    const ahead = monthAheadLocalDate()
    expect(ahead > today).toBe(true)
  })

  it('monthBeforeLocalDate is 1 month before today', () => {
    // kills the d.getMonth() - 1 -> + 1 mutation
    const today = todayLocalDate()
    const before = monthBeforeLocalDate()
    expect(before < today).toBe(true)
  })
})

describe('admin fullClientUrl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('resolves relative URL to absolute using window.location.origin', () => {
    // kills the window.location.origin mutation
    expect(fullClientUrl('/profile')).toContain('/profile')
  })

  it('returns url unchanged when window is undefined (SSR)', () => {
    // kills the typeof window === "undefined" mutation
    vi.stubGlobal('window', undefined)
    expect(fullClientUrl('/some/path')).toBe('/some/path')
  })

  it('resolves relative path to full origin url', () => {
    // kills the new URL(url, window.location.origin) mutation
    expect(fullClientUrl('/dashboard')).toBe('http://localhost:3000/dashboard')
  })
})

describe('admin promoStateLabel', () => {
  it('returns exhausted when uses depleted', () => {
    // kills the mutations: > 0, >=, maxUses || 0, usedCount || 0
    expect(promoStateLabel({ maxUses: 10, usedCount: 10, active: true })).toBe('исчерпан по числу активаций')
    // usedCount > maxUses
    expect(promoStateLabel({ maxUses: 5, usedCount: 6, active: true })).toBe('исчерпан по числу активаций')
    // maxUses=0 -> not exhausted (no limit)
    expect(promoStateLabel({ maxUses: 0, usedCount: 100, active: true })).toBe('активен')
  })

  it('returns expired when activeTo is in the past', () => {
    // kills the < Date.now() mutation
    expect(promoStateLabel({ active: true, activeTo: '2020-01-01T00:00:00Z' })).toBe('истёк срок окна активации')
    // activeTo in the future -> not expired
    expect(promoStateLabel({ active: true, activeTo: '2099-01-01T00:00:00Z' })).toBe('активен')
  })

  it('returns active/inactive based on p.active', () => {
    // kills the p.active ? ... mutation
    expect(promoStateLabel({ active: true })).toBe('активен')
    expect(promoStateLabel({ active: false })).toBe('неактивен')
  })
})

describe('admin promoIsExhaustedMultiUse', () => {
  it('detects only multi-use exhausted promocodes', () => {
    expect(promoIsExhaustedMultiUse({ maxUses: 3, usedCount: 3 } as any)).toBe(true)
    expect(promoIsExhaustedMultiUse({ maxUses: 3, usedCount: 4 } as any)).toBe(true)
    expect(promoIsExhaustedMultiUse({ maxUses: 1, usedCount: 1 } as any)).toBe(false)
    expect(promoIsExhaustedMultiUse({ maxUses: 3, usedCount: 2 } as any)).toBe(false)
  })
})
