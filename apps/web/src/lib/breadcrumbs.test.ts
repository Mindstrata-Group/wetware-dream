import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearBreadcrumbs, getBreadcrumbs, installBreadcrumbs } from './breadcrumbs'

beforeEach(() => {
  clearBreadcrumbs()
  // Reset installBreadcrumbs between tests via a new window
})

afterEach(() => {
  clearBreadcrumbs()
  vi.restoreAllMocks()
})

describe('getBreadcrumbs + clearBreadcrumbs', () => {
  it('returns empty array initially (убивает .slice() → убрать мутацию через изоляцию)', () => {
    expect(getBreadcrumbs()).toEqual([])
  })

  it('returns a copy — mutating result does not affect internal state', () => {
    // kills the return breadcrumbs.slice() -> return breadcrumbs mutation
    installBreadcrumbs()
    const btn = document.createElement('button')
    btn.textContent = 'Click'
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    const copy = getBreadcrumbs()
    copy.length = 0
    expect(getBreadcrumbs().length).toBeGreaterThan(0)
    document.body.removeChild(btn)
  })

  it('clearBreadcrumbs empties the list (убивает length = 0 мутацию)', () => {
    installBreadcrumbs()
    const btn = document.createElement('button')
    btn.textContent = 'X'
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    expect(getBreadcrumbs().length).toBeGreaterThan(0)
    clearBreadcrumbs()
    expect(getBreadcrumbs()).toEqual([])
    document.body.removeChild(btn)
  })
})

describe('installBreadcrumbs click tracking', () => {
  beforeEach(() => {
    clearBreadcrumbs()
    installBreadcrumbs()
  })

  it('records a click breadcrumb with selector and text', () => {
    const btn = document.createElement('button')
    btn.id = 'my-btn'
    btn.textContent = 'Отправить'
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    const crumbs = getBreadcrumbs()
    const click = crumbs.find(c => c.type === 'click')
    expect(click).toBeDefined()
    expect(click?.data.selector).toContain('button')
    expect(click?.data.text).toBe('Отправить')
    document.body.removeChild(btn)
  })

  it('stops climbing to interactive parent — button is found (убивает цикл i < 5 мутацию)', () => {
    const div = document.createElement('div')
    const btn = document.createElement('button')
    btn.textContent = 'OK'
    div.appendChild(btn)
    document.body.appendChild(div)

    const span = document.createElement('span')
    span.textContent = 'внутри'
    btn.appendChild(span)
    // A click on a span inside a button must find the button
    span.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    const crumbs = getBreadcrumbs()
    const click = crumbs.find(c => c.type === 'click')
    expect(click?.data.selector).toContain('button')
    document.body.removeChild(div)
  })

  it('shortSelector stops at element with id (убивает id-break мутацию)', () => {
    const outer = document.createElement('div')
    const inner = document.createElement('button')
    inner.id = 'send-btn'
    inner.textContent = 'Да'
    outer.appendChild(inner)
    document.body.appendChild(outer)
    inner.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    const crumbs = getBreadcrumbs()
    const click = crumbs.find(c => c.type === 'click')
    // the selector must contain the id and stop there
    expect(click?.data.selector).toContain('#send-btn')
    document.body.removeChild(outer)
  })

  it('clickText normalizes whitespace (убивает trim и replace мутации)', () => {
    const btn = document.createElement('button')
    btn.textContent = '  Много   пробелов  '
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    const crumbs = getBreadcrumbs()
    const click = crumbs.find(c => c.type === 'click')
    expect(click?.data.text).toBe('Много пробелов')
    document.body.removeChild(btn)
  })

  it('clickText trims to 40 chars (убивает slice(0, 40) мутацию)', () => {
    const btn = document.createElement('button')
    btn.textContent = 'а'.repeat(60)
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))

    const crumbs = getBreadcrumbs()
    const click = crumbs.find(c => c.type === 'click')
    expect((click?.data.text as string).length).toBeLessThanOrEqual(40)
    document.body.removeChild(btn)
  })
})

describe('installBreadcrumbs fetch tracking', () => {
  beforeEach(() => {
    clearBreadcrumbs()
    installBreadcrumbs()
  })

  it('records fetch breadcrumb with method, url and status', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 200 })))
    installBreadcrumbs() // reinstall with a mocked fetch

    await fetch('/api/test', { method: 'POST' })
    const crumbs = getBreadcrumbs()
    const fetchCrumb = crumbs.find(c => c.type === 'fetch')
    expect(fetchCrumb?.data.url).toContain('/api/test')
    expect(fetchCrumb?.data.status).toBe(200)
    vi.unstubAllGlobals()
  })

  it('does NOT track /api/_error calls (убивает skip-condition мутацию)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    installBreadcrumbs()

    await fetch('/api/_error', { method: 'POST' })
    const crumbs = getBreadcrumbs()
    const errorCrumb = crumbs.find(c => c.type === 'fetch' && String(c.data.url).includes('_error'))
    expect(errorCrumb).toBeUndefined()
    vi.unstubAllGlobals()
  })

  it('truncates long URLs to 80 chars with ellipsis (убивает slice(0, 80) мутацию)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 200 })))
    installBreadcrumbs()

    const longUrl = '/api/' + 'x'.repeat(100)
    await fetch(longUrl)
    const crumbs = getBreadcrumbs()
    const fetchCrumb = crumbs.find(c => c.type === 'fetch')
    expect(String(fetchCrumb?.data.url).length).toBeLessThanOrEqual(82) // 80 + "…"
    vi.unstubAllGlobals()
  })
})

describe('installBreadcrumbs console tracking', () => {
  it('records console.error as breadcrumb (убивает type console мутацию)', () => {
    clearBreadcrumbs()
    installBreadcrumbs()
    console.error('test error message')
    const crumbs = getBreadcrumbs()
    const consoleCrumb = crumbs.find(c => c.type === 'console')
    expect(consoleCrumb?.data.level).toBe('error')
    expect(String(consoleCrumb?.data.msg)).toContain('test error message')
  })

  it('truncates console message to 200 chars (убивает slice(0, 200) мутацию)', () => {
    clearBreadcrumbs()
    installBreadcrumbs()
    console.error('x'.repeat(300))
    const crumbs = getBreadcrumbs()
    const consoleCrumb = crumbs.find(c => c.type === 'console')
    expect(String(consoleCrumb?.data.msg).length).toBeLessThanOrEqual(200)
  })
})

describe('circular buffer overflow', () => {
  it('evicts oldest entry after 30 breadcrumbs (убивает > MAX_BREADCRUMBS мутацию)', () => {
    clearBreadcrumbs()
    installBreadcrumbs()
    // Add 31 clicks; the first must be evicted
    for (let i = 0; i < 31; i++) {
      const btn = document.createElement('button')
      btn.textContent = `Кнопка ${i}`
      document.body.appendChild(btn)
      btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      document.body.removeChild(btn)
    }
    const crumbs = getBreadcrumbs()
    expect(crumbs.length).toBe(30)
    // The first button is evicted; the last one must be button number 30
    const last = crumbs[crumbs.length - 1]
    expect(last.data.text).toBe('Кнопка 30')
  })
})

describe('installBreadcrumbs SSR guard', () => {
  it('does nothing when window is undefined', () => {
    // kills the typeof window === "undefined" -> false mutation
    vi.stubGlobal('window', undefined)
    expect(() => installBreadcrumbs()).not.toThrow()
    vi.unstubAllGlobals()
  })
})
