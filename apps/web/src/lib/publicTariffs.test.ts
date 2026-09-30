import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { clearPublicTariffsCacheForTests, formatRub, loadPublicTariffs } from './publicTariffs'

function okFetch(tariffs: unknown[]) {
  return vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ tariffs }),
  })
}

function failFetch() {
  return vi.fn().mockRejectedValue(new Error('network'))
}

function notOkFetch() {
  return vi.fn().mockResolvedValue({ ok: false })
}

describe('publicTariffs', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    clearPublicTariffsCacheForTests()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('caches public tariffs with mode welcome messages in the current tab', async () => {
    const fetchMock = okFetch([{
      id: 1, name: 'Профи', description: 'Для практики', tariffType: 'regular',
      monthlyPrice: 1000, dailyMessageLimit: 50, limitType: 'shared',
      groupName: 'Основные', modeIds: [7],
      modes: [{ id: 7, name: 'Метамодель', welcomeMessage: 'Начнём с вашей ситуации' }],
    }])
    vi.stubGlobal('fetch', fetchMock)

    const first = await loadPublicTariffs()
    const second = await loadPublicTariffs()

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(second).toBe(first)
    expect(second[0]?.modes[0]?.welcomeMessage).toBe('Начнём с вашей ситуации')
  })

  it('refetches after TTL expires', async () => {
    const fetchMock = okFetch([{ id: 1, name: 'A', modeIds: [], modes: [] }])
    vi.stubGlobal('fetch', fetchMock)

    await loadPublicTariffs()
    vi.advanceTimersByTime(61_000)
    await loadPublicTariffs()

    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('returns empty array when response has no tariffs field', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }))
    expect(await loadPublicTariffs()).toEqual([])
  })

  it('returns stale cache when fetch returns non-ok status', async () => {
    vi.stubGlobal('fetch', okFetch([{ id: 1, name: 'Stale', modeIds: [], modes: [] }]))
    await loadPublicTariffs()
    vi.advanceTimersByTime(61_000)

    vi.stubGlobal('fetch', notOkFetch())
    const result = await loadPublicTariffs()
    expect(result[0]?.name).toBe('Stale')
  })

  it('returns empty array when non-ok and no stale cache', async () => {
    vi.stubGlobal('fetch', notOkFetch())
    expect(await loadPublicTariffs()).toEqual([])
  })

  it('returns stale cache when fetch throws', async () => {
    vi.stubGlobal('fetch', okFetch([{ id: 2, name: 'Cached', modeIds: [], modes: [] }]))
    await loadPublicTariffs()
    vi.advanceTimersByTime(61_000)

    vi.stubGlobal('fetch', failFetch())
    const result = await loadPublicTariffs()
    expect(result[0]?.name).toBe('Cached')
  })

  it('returns empty array when fetch throws and no stale cache', async () => {
    vi.stubGlobal('fetch', failFetch())
    expect(await loadPublicTariffs()).toEqual([])
  })
})

describe('formatRub', () => {
  it('formats integer rubles with Russian thousands separator', () => {
    const result = formatRub(1000)
    expect(result).toContain('₽')
    expect(result).toMatch(/1[.\s ]?000/)
  })

  it('rounds to nearest integer before formatting', () => {
    expect(formatRub(1234.5)).toContain('1')
    expect(formatRub(1234.5)).toContain('235')
    expect(formatRub(1234.4)).toContain('234')
  })

  it('formats zero', () => {
    expect(formatRub(0)).toMatch(/^0/)
    expect(formatRub(0)).toContain('₽')
  })
})
