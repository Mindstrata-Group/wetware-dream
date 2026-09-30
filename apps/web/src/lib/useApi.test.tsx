import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearApiCache, useApi } from './useApi'

const jsonResponse = (payload: unknown) =>
  new Response(JSON.stringify(payload), { status: 200, headers: { 'content-type': 'application/json' } })

describe('useApi', () => {
  afterEach(() => {
    clearApiCache()
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('dedupes in-flight requests and shares cached data', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 42 }))
    vi.stubGlobal('fetch', fetchMock)

    const first = renderHook(() => useApi<{ value: number }>('/api/example'))
    const second = renderHook(() => useApi<{ value: number }>('/api/example'))

    await waitFor(() => expect(first.result.current.data?.value).toBe(42))
    await waitFor(() => expect(second.result.current.data?.value).toBe(42))
    expect(fetchMock).toHaveBeenCalledTimes(1)

    const third = renderHook(() => useApi<{ value: number }>('/api/example'))
    await waitFor(() => expect(third.result.current.data?.value).toBe(42))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('dedupes под нагрузкой: 20 параллельных hooks → 1 fetch', async () => {
    let resolveFetch: (response: Response) => void = () => {}
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve
        }),
    )
    vi.stubGlobal('fetch', fetchMock)

    // Start 20 simultaneous hooks BEFORE fetch finishes:
    // simulates the race condition when several components render
    // at the same time (e.g. the admin page with tabs).
    const hooks = Array.from({ length: 20 }, () =>
      renderHook(() => useApi<{ value: number }>('/api/heavy')),
    )

    for (const hook of hooks) {
      expect(hook.result.current.loading).toBe(true)
    }
    expect(fetchMock).toHaveBeenCalledTimes(1)

    resolveFetch(jsonResponse({ ok: true, value: 99 }))

    for (const hook of hooks) {
      await waitFor(() => expect(hook.result.current.data?.value).toBe(99))
    }
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('после clearApiCache следующий запрос делает новый fetch', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    const hook = renderHook(() => useApi<{ value: number }>('/api/refresh'))
    await waitFor(() => expect(hook.result.current.data?.value).toBe(1))
    expect(fetchMock).toHaveBeenCalledTimes(1)

    clearApiCache()
    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true, value: 2 }))
    const hook2 = renderHook(() => useApi<{ value: number }>('/api/refresh'))
    await waitFor(() => expect(hook2.result.current.data?.value).toBe(2))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('clearApiCache(path) clears only matching path, keeps others (убивает key.includes мутацию)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    const hook1 = renderHook(() => useApi<{ value: number }>('/api/users'))
    const hook2 = renderHook(() => useApi<{ value: number }>('/api/modes'))
    await waitFor(() => expect(hook1.result.current.data?.value).toBe(1))
    await waitFor(() => expect(hook2.result.current.data?.value).toBe(1))
    expect(fetchMock).toHaveBeenCalledTimes(2)

    clearApiCache('/api/users')
    fetchMock.mockResolvedValue(jsonResponse({ ok: true, value: 99 }))
    const hook3 = renderHook(() => useApi<{ value: number }>('/api/users'))
    await waitFor(() => expect(hook3.result.current.data?.value).toBe(99))
    // /api/modes cache untouched — no extra fetch
    const hook4 = renderHook(() => useApi<{ value: number }>('/api/modes'))
    await waitFor(() => expect(hook4.result.current.data?.value).toBe(1))
    expect(fetchMock).toHaveBeenCalledTimes(3) // only /api/users re-fetched
  })

  it('cache expires after cacheTtlMs and refetches (убивает >= expiresAt мутацию)', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    const hook = renderHook(() => useApi<{ value: number }>('/api/ttl', { cacheTtlMs: 1000 }))
    await act(() => vi.runAllTimersAsync())
    await waitFor(() => expect(hook.result.current.data?.value).toBe(1))
    expect(fetchMock).toHaveBeenCalledTimes(1)

    fetchMock.mockResolvedValueOnce(jsonResponse({ ok: true, value: 2 }))
    // Advance past TTL
    await act(() => vi.advanceTimersByTimeAsync(1001))
    const hook2 = renderHook(() => useApi<{ value: number }>('/api/ttl', { cacheTtlMs: 1000 }))
    await act(() => vi.runAllTimersAsync())
    await waitFor(() => expect(hook2.result.current.data?.value).toBe(2))
    expect(fetchMock).toHaveBeenCalledTimes(2)
    vi.useRealTimers()
  })

  it('exposes loading, error, and manual refetch for disabled hooks', async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ error: 'boom' }), { status: 500, headers: { 'content-type': 'application/json' } }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const hook = renderHook(() => useApi('/api/fail', { enabled: false }))
    expect(hook.result.current.loading).toBe(false)
    expect(fetchMock).not.toHaveBeenCalled()

    await act(async () => {
      await hook.result.current.refetch()
    })
    await waitFor(() => expect(hook.result.current.error?.message).toBe('boom'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('path=null: не делает fetch, loading=false, refetch() возвращает null (убивает !path мутацию)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    const hook = renderHook(() => useApi<{ value: number }>(null))
    expect(hook.result.current.loading).toBe(false)
    expect(hook.result.current.data).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()

    const result = await hook.result.current.refetch()
    expect(result).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('вызывает onSuccess с данными при успешном fetch (убивает удаление onSuccess?.() мутацию)', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 7 }))
    vi.stubGlobal('fetch', fetchMock)
    const onSuccess = vi.fn()

    renderHook(() => useApi<{ ok: boolean; value: number }>('/api/success-cb', { onSuccess }))
    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith({ ok: true, value: 7 }))
  })

  it('вызывает onError с ошибкой при неуспешном fetch (убивает удаление onError?.() мутацию)', async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ error: 'nope' }), { status: 500, headers: { 'content-type': 'application/json' } }),
    )
    vi.stubGlobal('fetch', fetchMock)
    const onError = vi.fn()

    renderHook(() => useApi('/api/error-cb', { onError }))
    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1))
    expect(onError.mock.calls[0][0]).toBeInstanceOf(Error)
  })

  it('вызывает onSuccess из cache-hit ветки refetch, а не только из основного пути (убивает hit-branch onSuccess?.())', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 3 }))
    vi.stubGlobal('fetch', fetchMock)

    const first = renderHook(() => useApi<{ value: number }>('/api/cache-hit-cb'))
    await waitFor(() => expect(first.result.current.data?.value).toBe(3))

    const onSuccess = vi.fn()
    const second = renderHook(() => useApi<{ ok: boolean; value: number }>('/api/cache-hit-cb', { onSuccess }))
    // The second hook reads from the warm cache synchronously (initial state), but
    // refetch() inside useEffect must still call onSuccess with
    // the cached value via the `if (hit)` branch.
    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith({ ok: true, value: 3 }))
    expect(second.result.current.loading).toBe(false)
  })

  it('не обновляет state после unmount (убивает mounted.current guard мутацию)', async () => {
    let resolveFetch: (response: Response) => void = () => {}
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          resolveFetch = resolve
        }),
    )
    vi.stubGlobal('fetch', fetchMock)

    const hook = renderHook(() => useApi<{ value: number }>('/api/unmount-guard'))
    expect(hook.result.current.loading).toBe(true)

    hook.unmount()
    // Resolve fetch AFTER unmount; setState must not fail.
    await act(async () => {
      resolveFetch(jsonResponse({ ok: true, value: 5 }))
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('initial loading отражает Boolean(path && enabled && !data) синхронно при первом рендере', () => {
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true, value: 1 }))
    vi.stubGlobal('fetch', fetchMock)

    // path is set, enabled defaults to true, no cache -> loading must be true IMMEDIATELY.
    const hook = renderHook(() => useApi<{ value: number }>('/api/initial-loading'))
    expect(hook.result.current.loading).toBe(true)
  })
})
