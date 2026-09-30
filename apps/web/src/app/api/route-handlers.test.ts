import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as accessApply from './access/promocode/apply/route'
import * as accessStatus from './access/status/route'
import * as oauthProxy from './auth/oauth/[...path]/route'
import * as promoValidate from './promo/validate/route'
import * as demoModes from './public/demo-modes/route'
import * as publicModes from './public/modes/route'
import * as publicTariffs from './public/tariffs/route'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  })
}

async function readJson(response: Response) {
  return JSON.parse(await response.text()) as Record<string, unknown>
}

describe('web API route proxies P0', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('GET /api/access/status forwards cookies, status, content-type and set-cookie', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true, hasAccess: true }, {
      status: 207,
      headers: { 'content-type': 'application/json; charset=utf-8', 'set-cookie': 'sid=next; Path=/; HttpOnly' },
    }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const response = await accessStatus.GET(new Request('http://localhost/api/access/status', {
      headers: { cookie: 'sid=old' },
    }))

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/access/status', expect.objectContaining({
      method: 'GET',
      headers: { cookie: 'sid=old' },
      cache: 'no-store',
      signal: expect.any(AbortSignal),
    }))
    expect(response.status).toBe(207)
    expect(response.headers.get('content-type')).toBe('application/json; charset=utf-8')
    expect(response.headers.get('set-cookie')).toBe('sid=next; Path=/; HttpOnly')
    expect(await readJson(response)).toEqual({ ok: true, hasAccess: true })
  })

  it('POST /api/access/promocode/apply forwards raw body and auth cookie to canonical backend endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true, code: 'P0' }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const response = await accessApply.POST(new Request('http://localhost/api/access/promocode/apply', {
      method: 'POST',
      headers: { cookie: 'sid=abc' },
      body: '{"code":"P0"}',
    }))

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/access/promocode/apply', expect.objectContaining({
      method: 'POST',
      headers: { 'content-type': 'application/json', cookie: 'sid=abc' },
      body: '{"code":"P0"}',
      cache: 'no-store',
      signal: expect.any(AbortSignal),
    }))
    expect(await readJson(response)).toEqual({ ok: true, code: 'P0' })
  })

  it('POST /api/promo/validate normalizes legacy promoCode field before proxying', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await promoValidate.POST(new Request('http://localhost/api/promo/validate', {
      method: 'POST',
      headers: { cookie: 'sid=abc' },
      body: JSON.stringify({ promoCode: 'LEGACY' }),
    }))

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/access/promocode/apply', expect.objectContaining({
      body: JSON.stringify({ promoCode: 'LEGACY', code: 'LEGACY' }),
    }))
  })

  it('promo/validate does NOT re-add code when code already present (убивает !parsed.code мутацию)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await promoValidate.POST(new Request('http://localhost/api/promo/validate', {
      method: 'POST',
      headers: { cookie: 'sid=abc' },
      body: JSON.stringify({ code: 'DIRECT', promoCode: 'LEGACY' }),
    }))

    // code is already present; the body must stay unchanged
    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/access/promocode/apply', expect.objectContaining({
      body: JSON.stringify({ code: 'DIRECT', promoCode: 'LEGACY' }),
    }))
  })

  it('access/status sends empty cookie string when request has none (убивает || "" мутацию)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true, hasAccess: false }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await accessStatus.GET(new Request('http://localhost/api/access/status'))

    expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
      headers: { cookie: '' },
    }))
  })

  it('public modes routes set cache headers for success and no-store fallback for upstream failures', async () => {
    globalThis.fetch = vi.fn().mockResolvedValueOnce(jsonResponse({ modes: [{ id: 1 }] })) as unknown as typeof fetch

    const okResponse = await publicModes.GET()
    expect(okResponse.headers.get('cache-control')).toContain('s-maxage=300')
    expect(await readJson(okResponse)).toEqual({ modes: [{ id: 1 }] })

    globalThis.fetch = vi.fn().mockResolvedValueOnce(jsonResponse({ ok: false }, { status: 500 })) as unknown as typeof fetch
    const failResponse = await demoModes.GET()
    expect(failResponse.status).toBe(500)
    expect(failResponse.headers.get('cache-control')).toBe('no-store')
  })

  it('GET /api/public/tariffs proxies storefront tariffs with cache headers', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true, tariffs: [{ id: 1, name: 'Профи' }] }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const response = await publicTariffs.GET()

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/public/tariffs', expect.objectContaining({
      next: { revalidate: 300 },
      signal: expect.any(AbortSignal),
    }))
    expect(response.headers.get('cache-control')).toContain('s-maxage=300')
    expect(await readJson(response)).toEqual({ ok: true, tariffs: [{ id: 1, name: 'Профи' }] })
  })

  it('OAuth proxy preserves redirect location and set-cookie from upstream auth provider flow', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('', {
      status: 302,
      headers: { location: 'https://oauth.example/authorize', 'set-cookie': 'oauth_state=1; Path=/; HttpOnly' },
    }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const response = await oauthProxy.GET(
      new Request('http://localhost/api/auth/oauth/yandex/start?next=%2Fprofile', { headers: { cookie: 'sid=abc' } }),
      { params: Promise.resolve({ path: ['yandex', 'start'] }) },
    )

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/auth/oauth/yandex/start?next=%2Fprofile', expect.objectContaining({
      method: 'GET',
      headers: { cookie: 'sid=abc' },
      redirect: 'manual',
      cache: 'no-store',
      signal: expect.any(AbortSignal),
    }))
    expect(response.status).toBe(302)
    expect(response.headers.get('location')).toBe('https://oauth.example/authorize')
    expect(response.headers.get('set-cookie')).toBe('oauth_state=1; Path=/; HttpOnly')
  })

  it('returns stable JSON fallbacks when upstream API is unavailable', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('network down')) as unknown as typeof fetch

    const statusResponse = await accessStatus.GET(new Request('http://localhost/api/access/status'))
    expect(statusResponse.status).toBe(502)
    expect(await readJson(statusResponse)).toEqual({ ok: false, error: 'access_status_unavailable' })

    const modesResponse = await publicModes.GET()
    expect(modesResponse.status).toBe(503)
    expect(modesResponse.headers.get('cache-control')).toBe('no-store')
    expect(await readJson(modesResponse)).toEqual({ ok: false, error: 'modes_unavailable' })

    const tariffsResponse = await publicTariffs.GET()
    expect(tariffsResponse.status).toBe(503)
    expect(tariffsResponse.headers.get('cache-control')).toBe('no-store')
    expect(await readJson(tariffsResponse)).toEqual({ ok: false, error: 'tariffs_unavailable' })
  })

  it('demo-modes.GET success path: точный CACHE_CONTROL и content-type fallback (убивает удаление ok-ветки)', async () => {
    // A response without a content-type header: the 'application/json' default must apply.
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(new TextEncoder().encode(JSON.stringify({ demo: true })), { status: 200 })) as unknown as typeof fetch

    const response = await demoModes.GET()

    expect(response.status).toBe(200)
    expect(response.headers.get('content-type')).toBe('application/json')
    expect(response.headers.get('cache-control')).toBe('public, max-age=60, s-maxage=300, stale-while-revalidate=86400')
    expect(await readJson(response)).toEqual({ demo: true })
  })

  it('demo-modes.GET catch-block: сетевая ошибка → 503 + demo_modes_unavailable + no-store (убивает подмену error-кода)', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('down')) as unknown as typeof fetch

    const response = await demoModes.GET()

    expect(response.status).toBe(503)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(await readJson(response)).toEqual({ ok: false, error: 'demo_modes_unavailable' })
  })

  it('modes.GET: upstream.ok=false → no-store + console.error со status/body (убивает удаление if(!upstream.ok) ветки)', async () => {
    const errorSpy = vi.spyOn(console, 'error')
    globalThis.fetch = vi.fn().mockResolvedValue(new Response('boom', { status: 500 })) as unknown as typeof fetch

    const response = await publicModes.GET()

    expect(response.status).toBe(500)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(errorSpy).toHaveBeenCalledWith('[api/public/modes] upstream error', { status: 500, body: 'boom' })
  })

  it('tariffs.GET: upstream.ok=false → no-store + console.error (та же ветка, отдельный маршрут)', async () => {
    const errorSpy = vi.spyOn(console, 'error')
    globalThis.fetch = vi.fn().mockResolvedValue(new Response('fail', { status: 502 })) as unknown as typeof fetch

    const response = await publicTariffs.GET()

    expect(response.status).toBe(502)
    expect(response.headers.get('cache-control')).toBe('no-store')
    expect(errorSpy).toHaveBeenCalledWith('[api/public/tariffs] upstream error', { status: 502, body: 'fail' })
  })

  it('access/status: content-type fallback на application/json, когда upstream его не прислал', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(new TextEncoder().encode('{}'), { status: 200 })) as unknown as typeof fetch

    const response = await accessStatus.GET(new Request('http://localhost/api/access/status'))

    expect(response.headers.get('content-type')).toBe('application/json')
  })

  it('access/status: set-cookie НЕ проставляется в ответ, если upstream его не прислал (убивает удаление if(setCookie) guard)', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(jsonResponse({ ok: true })) as unknown as typeof fetch

    const response = await accessStatus.GET(new Request('http://localhost/api/access/status'))

    expect(response.headers.get('set-cookie')).toBeNull()
  })

  it('access/promocode/apply: catch-block сетевой ошибки → 502 + server_error (не тестировалось вообще)', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('down')) as unknown as typeof fetch

    const response = await accessApply.POST(new Request('http://localhost/api/access/promocode/apply', {
      method: 'POST',
      body: '{"code":"X"}',
    }))

    expect(response.status).toBe(502)
    expect(await readJson(response)).toEqual({ ok: false, code: 'server_error', error: 'Ошибка подключения к API' })
  })

  it('promo/validate: невалидный JSON в теле — тело пересылается как есть, без падения (убивает удаление try/catch вокруг JSON.parse)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await promoValidate.POST(new Request('http://localhost/api/promo/validate', {
      method: 'POST',
      body: 'не json вообще',
    }))

    expect(fetchMock).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
      body: 'не json вообще',
    }))
  })

  it('promo/validate: catch-block сетевой ошибки → 502 + server_error (не тестировалось вообще)', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('down')) as unknown as typeof fetch

    const response = await promoValidate.POST(new Request('http://localhost/api/promo/validate', {
      method: 'POST',
      body: '{"code":"X"}',
    }))

    expect(response.status).toBe(502)
    expect(await readJson(response)).toEqual({ ok: false, code: 'server_error', error: 'Ошибка подключения к API' })
  })
})
