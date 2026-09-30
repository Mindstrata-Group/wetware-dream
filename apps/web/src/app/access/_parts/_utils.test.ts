import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildPromoSourceLink, errorMap, formatActiveTo, normalizePromoModes } from './_utils'

describe('access promo utils P0', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('adds encoded text parameter to valid promo source URL while preserving existing params', () => {
    expect(buildPromoSourceLink('https://t.me/support_bot?start=abc', 'Добрый день!')).toBe(
      'https://t.me/support_bot?start=abc&text=%D0%94%D0%BE%D0%B1%D1%80%D1%8B%D0%B9+%D0%B4%D0%B5%D0%BD%D1%8C%21',
    )
  })

  it('returns invalid/free-form contact value unchanged', () => {
    expect(buildPromoSourceLink('@support_bot', 'ignored')).toBe('@support_bot')
  })

  it('normalizes only named modes and keeps dates/ids for success screen', () => {
    expect(normalizePromoModes([
      { modeId: 10, modeName: 'Психолог', activeTo: '2026-06-01T00:00:00Z' },
      { modeId: 11, modeName: '' },
      { modeName: 'Коуч' },
    ])).toEqual([
      { modeId: 10, name: 'Психолог', activeTo: '2026-06-01T00:00:00Z' },
      { modeId: undefined, name: 'Коуч', activeTo: undefined },
    ])
  })

  it('handles absent modes as empty list', () => {
    expect(normalizePromoModes()).toEqual([])
  })

  it('formats missing activeTo as open-ended', () => {
    expect(formatActiveTo()).toBe('без даты окончания')
  })

  it('formats activeTo through ru-RU locale for user-facing access cards', () => {
    const spy = vi.spyOn(Date.prototype, 'toLocaleString').mockReturnValue('01.06.2026, 03:00:00')
    expect(formatActiveTo('2026-06-01T00:00:00Z')).toBe('01.06.2026, 03:00:00')
    expect(spy).toHaveBeenCalledWith('ru-RU')
  })

  it('contains P0 backend error codes shown on /access', () => {
    expect(errorMap.empty).toBe('Введите промокод')
    expect(errorMap.auth_required).toMatch(/войдите через Яндекс/i)
    expect(errorMap.already_used).toMatch(/уже использовали/i)
    expect(errorMap.limit_reached).toMatch(/Лимит/i)
    expect(errorMap.not_found).toMatch(/нет/i)
    expect(errorMap.not_started).toMatch(/не активен/i)
    expect(errorMap.expired).toMatch(/истёк/i)
    expect(errorMap.archived).toMatch(/отключён/i)
    expect(errorMap.server_error).toMatch(/ошибка сервера/i)
  })

  it('formats empty string activeTo as open-ended', () => {
    expect(formatActiveTo('')).toBe('без даты окончания')
  })

  it('normalizes modes list with null/undefined input', () => {
    expect(normalizePromoModes(undefined)).toEqual([])
    expect(normalizePromoModes(null as any)).toEqual([])
  })

  it('returns raw string when toLocaleString throws (убивает catch-branch мутацию)', () => {
    vi.spyOn(Date.prototype, 'toLocaleString').mockImplementation(() => {
      throw new Error('locale not supported')
    })
    expect(formatActiveTo('2026-06-01T00:00:00Z')).toBe('2026-06-01T00:00:00Z')
  })
})
