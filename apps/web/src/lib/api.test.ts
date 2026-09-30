import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, apiFetch, roleHome } from './api'

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  })
}

describe('apiFetch P0 contract', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })

  it('always sends credentials and JSON content-type when body is present', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true, value: 42 }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const result = await apiFetch<{ value: number }>('/api/p0', {
      method: 'POST',
      body: JSON.stringify({ hello: 'world' }),
    })

    expect(result.value).toBe(42)
    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/p0', expect.objectContaining({
      credentials: 'include',
      headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
    }))
  })

  it('lets explicit headers override the auto JSON content-type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    globalThis.fetch = fetchMock as unknown as typeof fetch

    await apiFetch('/api/upload', {
      method: 'POST',
      body: 'raw',
      headers: { 'Content-Type': 'text/plain', 'X-Test': '1' },
    })

    expect(fetchMock).toHaveBeenCalledWith('http://localhost:18080/api/upload', expect.objectContaining({
      headers: { 'Content-Type': 'text/plain', 'X-Test': '1' },
    }))
  })

  it('throws ApiError with backend payload/debug for HTTP errors', async () => {
    const payload = { ok: false, error: 'auth required', debug: { requestId: 'req-1' } }
    globalThis.fetch = vi.fn().mockResolvedValue(jsonResponse(payload, { status: 401 })) as unknown as typeof fetch

    await expect(apiFetch('/api/profile')).rejects.toMatchObject({
      name: 'ApiError',
      message: 'auth required',
      status: 401,
      payload,
      debug: payload.debug,
    })
  })

  it('treats ok:false JSON as business-error even with HTTP 200', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(jsonResponse({ ok: false, error: 'limit reached' })) as unknown as typeof fetch

    await expect(apiFetch('/api/chat/send')).rejects.toMatchObject({
      status: 200,
      message: 'limit reached',
    })
  })

  it('rejects non-JSON bodies with a safe preview and requested path', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response('<html>proxy error</html>', {
      status: 502,
      headers: { 'content-type': 'text/html' },
    })) as unknown as typeof fetch

    await expect(apiFetch('/api/chat/start')).rejects.toMatchObject({
      status: 502,
      payload: expect.objectContaining({
        error: 'non-json-response',
        contentType: 'text/html',
        preview: '<html>proxy error</html>',
        path: '/api/chat/start',
      }),
    })
  })

  it('rejects invalid JSON with diagnostics', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response('{ nope', {
      status: 200,
      headers: { 'content-type': 'application/json; charset=utf-8' },
    })) as unknown as typeof fetch

    await expect(apiFetch('/api/broken')).rejects.toMatchObject({
      status: 200,
      payload: expect.objectContaining({
        error: 'invalid-json',
        path: '/api/broken',
      }),
    })
  })

  it('returns an empty object for successful empty responses', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(new Response(null, { status: 204 })) as unknown as typeof fetch

    await expect(apiFetch('/api/auth/logout')).resolves.toEqual({})
  })

  it('falls back to HTTP status string when error field is absent', async () => {
    // kills the mutation payload.error || `HTTP ${res.status}` -> payload.error (drops the fallback)
    globalThis.fetch = vi.fn().mockResolvedValue(jsonResponse({}, { status: 403 })) as unknown as typeof fetch

    await expect(apiFetch('/api/admin')).rejects.toMatchObject({
      status: 403,
      message: 'HTTP 403',
    })
  })

  it('treats missing content-type header as non-JSON (убивает || "" мутацию)', async () => {
    // A Response with a body but no content-type: contentType = null -> || "" gives "" -> isJson=false -> throw
    globalThis.fetch = vi.fn().mockImplementation(() =>
      Promise.resolve(new Response('plain text', { status: 200 }))
    ) as unknown as typeof fetch

    await expect(apiFetch('/api/plain')).rejects.toMatchObject({
      payload: expect.objectContaining({ error: 'non-json-response' }),
    })
  })

  it('exposes text preview limited to 240 chars in non-JSON error', async () => {
    // kills the mutation text.slice(0, 240) -> text.slice(0, 0)
    const longHtml = '<html>' + 'x'.repeat(300) + '</html>'
    globalThis.fetch = vi.fn().mockImplementation(() =>
      Promise.resolve(new Response(longHtml, {
        status: 502,
        headers: { 'content-type': 'text/html' },
      }))
    ) as unknown as typeof fetch

    const err = await apiFetch('/api/gone').catch(e => e)
    expect(err.payload.preview).toMatch(/^<html>/)
    expect(err.payload.preview.length).toBeLessThanOrEqual(240)
  })
})

describe('roleHome P0 redirects', () => {
  it.each([
    ['owner', '/profile'],
    ['admin', '/profile'],
    ['tester', '/profile'],
    ['billing_admin', '/profile'],
    ['content_admin', '/profile'],
    ['support', '/profile'],
    ['user', '/chat'],
    [undefined, '/chat'],
  ] as const)('maps %s to %s', (role, expected) => {
    expect(roleHome(role)).toBe(expected)
  })

  it('keeps ApiError instanceof Error for existing error boundaries', () => {
    expect(new ApiError('boom', 500)).toBeInstanceOf(Error)
  })
})
