import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import { clearApiCache } from './useApi'
import { clearSiteContentCache, fillVars, getString, getArray, getBoolean, getNumber, getStorefrontFeatureLinks, isFeatureEnabled, loadSiteContent } from './siteContent'

vi.mock('./useApi', () => ({
  clearApiCache: vi.fn(),
}))

describe('loadSiteContent', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    clearSiteContentCache()
  })

  afterEach(() => {
    clearSiteContentCache()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('caches content within TTL, refetches after expiry', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ content: { 'hero.title': 'Стратум' } }),
    })
    vi.stubGlobal('fetch', fetchMock)

    const first = await loadSiteContent()
    await loadSiteContent()
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(first['hero.title']).toBe('Стратум')

    vi.advanceTimersByTime(61_000)
    await loadSiteContent()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('returns stale cache when fetch returns non-ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ content: { k: 'v' } }),
    }))
    await loadSiteContent()
    vi.advanceTimersByTime(61_000)

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
    const result = await loadSiteContent()
    expect(result['k']).toBe('v')
  })

  it('returns empty object on non-ok with no stale cache', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false }))
    expect(await loadSiteContent()).toEqual({})
  })

  it('returns stale cache when fetch throws', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ content: { k: 'fallback' } }),
    }))
    await loadSiteContent()
    vi.advanceTimersByTime(61_000)

    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    const result = await loadSiteContent()
    expect(result['k']).toBe('fallback')
  })

  it('returns empty object when fetch throws and no stale cache', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network')))
    expect(await loadSiteContent()).toEqual({})
  })

  it('uses json.content and falls back to {} when content absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({}),
    }))
    expect(await loadSiteContent()).toEqual({})
  })
})

describe('fillVars', () => {
  it('заменяет одну переменную', () => {
    expect(fillVars('У нас {{modes_count}} режимов', { modes_count: 11 })).toBe('У нас 11 режимов')
  })

  it('заменяет несколько разных переменных', () => {
    const out = fillVars('{{paid_count}} из {{total_count}}', { paid_count: 10, total_count: 11 })
    expect(out).toBe('10 из 11')
  })

  it('повторяет одну переменную сколько нужно', () => {
    expect(fillVars('{{n}} + {{n}} = {{n}}{{n}}', { n: 1 })).toBe('1 + 1 = 11')
  })

  it('игнорирует пробелы внутри скобок', () => {
    expect(fillVars('{{ modes_count }}', { modes_count: 5 })).toBe('5')
    expect(fillVars('{{   modes_count   }}', { modes_count: 5 })).toBe('5')
  })

  it('кейс-инсенситивен', () => {
    expect(fillVars('{{Modes_Count}}', { modes_count: 7 })).toBe('7')
    expect(fillVars('{{MODES_COUNT}}', { modes_count: 7 })).toBe('7')
  })

  it('оставляет неизвестные переменные как есть', () => {
    expect(fillVars('{{unknown}} foo', { other: 1 })).toBe('{{unknown}} foo')
  })

  it('не трогает одинарные скобки', () => {
    expect(fillVars('{modes_count}', { modes_count: 5 })).toBe('{modes_count}')
  })

  it('работает с пустой строкой', () => {
    expect(fillVars('', { x: 1 })).toBe('')
  })

  it('работает без переменных в строке', () => {
    expect(fillVars('Просто текст', { x: 1 })).toBe('Просто текст')
  })

  it('строковое значение тоже подставляется', () => {
    expect(fillVars('{{name}}', { name: 'Стратум' })).toBe('Стратум')
  })

  it('число 0 не пропускается как fallback', () => {
    expect(fillVars('{{n}}', { n: 0 })).toBe('0')
  })

  it('?? не заменяет 0 из vars[name] на vars[name.toLowerCase()] (убивает ?? → || мутацию)', () => {
    // vars['N'] = 0 (falsy) -> ?? keeps 0; || would fall back to n=5
    expect(fillVars('{{N}}', { N: 0, n: 5 })).toBe('0')
  })
})

describe('getString', () => {
  it('возвращает строку из контента', () => {
    expect(getString({ key1: 'hello' }, 'key1', 'fallback')).toBe('hello')
  })
  it('возвращает fallback если ключа нет', () => {
    expect(getString({}, 'missing', 'fb')).toBe('fb')
  })
  it('возвращает fallback если значение не строка', () => {
    expect(getString({ key1: 42 }, 'key1', 'fb')).toBe('fb')
    expect(getString({ key1: null }, 'key1', 'fb')).toBe('fb')
  })
})

describe('getArray', () => {
  it('возвращает массив', () => {
    expect(getArray({ items: [1, 2, 3] }, 'items', [])).toEqual([1, 2, 3])
  })
  it('возвращает fallback если не массив', () => {
    expect(getArray({ items: 'oops' }, 'items', [9])).toEqual([9])
  })
  it('возвращает fallback если ключа нет', () => {
    expect(getArray({}, 'items', [99])).toEqual([99])
  })
})

describe('getBoolean', () => {
  it('читает флаги из boolean и строковых значений', () => {
    expect(getBoolean({ feature: true }, 'feature')).toBe(true)
    expect(getBoolean({ feature: 'on' }, 'feature')).toBe(true)
    expect(getBoolean({ feature: 'выкл' }, 'feature', true)).toBe(false)
    expect(getBoolean({}, 'feature')).toBe(false)
  })

  it('возвращает false для boolean false (убивает return v → return true)', () => {
    expect(getBoolean({ f: false }, 'f', true)).toBe(false)
  })

  it('возвращает true-fallback когда ключа нет (убивает return fallback → return false)', () => {
    expect(getBoolean({}, 'missing', true)).toBe(true)
  })

  it('распознаёт все truthy-строки', () => {
    for (const s of ['1', 'true', 'yes', 'вкл']) {
      expect(getBoolean({ f: s }, 'f'), `"${s}" должна быть true`).toBe(true)
    }
  })

  it('распознаёт все falsy-строки при truthy-fallback', () => {
    for (const s of ['0', 'false', 'no', 'off', 'выкл']) {
      expect(getBoolean({ f: s }, 'f', true), `"${s}" должна быть false`).toBe(false)
    }
  })

  it('возвращает fallback для нераспознанной строки', () => {
    expect(getBoolean({ f: 'maybe' }, 'f', true)).toBe(true)
    expect(getBoolean({ f: 'maybe' }, 'f', false)).toBe(false)
  })
})

describe('getNumber', () => {
  it('читает числа из number и строковых CMS-значений', () => {
    expect(getNumber({ n: 3 }, 'n', 1)).toBe(3)
    expect(getNumber({ n: '2,5' }, 'n', 1)).toBe(2.5)
    expect(getNumber({ n: 'нет' }, 'n', 7)).toBe(7)
  })

  it('возвращает fallback для Infinity (убивает Number.isFinite → всегда true)', () => {
    expect(getNumber({ n: Infinity }, 'n', 7)).toBe(7)
    expect(getNumber({ n: -Infinity }, 'n', 7)).toBe(7)
  })

  it('возвращает fallback для строки-парсящейся-в-NaN', () => {
    expect(getNumber({ n: 'abc' }, 'n', 99)).toBe(99)
  })

  it('возвращает fallback если ключа нет', () => {
    expect(getNumber({}, 'n', 42)).toBe(42)
  })
})

describe('storefront feature helpers', () => {
  it('по умолчанию скрывает blog/shop и строит ссылки только из включённых флагов', () => {
    expect(isFeatureEnabled({}, 'blog')).toBe(false)
    expect(getStorefrontFeatureLinks({})).toEqual([])
    expect(getStorefrontFeatureLinks({
      'features.blog_enabled': true,
      'features.shop_enabled': 'on',
      'landing.blog_button_label': 'Журнал',
      'landing.shop_button_label': 'Тарифы',
    })).toEqual([
      { href: '/blog', label: 'Журнал', feature: 'blog' },
      { href: '/shop', label: 'Тарифы', feature: 'shop' },
    ])
  })

  it('включает только blog когда shop выключен (убивает мутацию второй ветки)', () => {
    expect(getStorefrontFeatureLinks({ 'features.blog_enabled': true })).toEqual([
      { href: '/blog', label: 'Блог', feature: 'blog' },
    ])
  })

  it('включает только shop когда blog выключен', () => {
    expect(getStorefrontFeatureLinks({ 'features.shop_enabled': true })).toEqual([
      { href: '/shop', label: 'Магазин решений', feature: 'shop' },
    ])
  })
})

describe('clearSiteContentCache', () => {
  it('сбрасывает клиентский cache публичного site-content после admin save', () => {
    clearSiteContentCache()
    expect(clearApiCache).toHaveBeenCalledWith('/api/public/site-content')
  })
})
