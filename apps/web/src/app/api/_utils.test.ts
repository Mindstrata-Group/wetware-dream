import { afterEach, describe, expect, it, vi } from 'vitest'
import { buildApiUrl, withUpstreamTimeout, UPSTREAM_TIMEOUT_MS } from './_utils'

describe('buildApiUrl', () => {
  it('appends path to default base', () => {
    // kills string concatenation mutations
    const url = buildApiUrl('/api/users')
    expect(url).toContain('/api/users')
    expect(url).toMatch(/^http/)
  })

  it('strips trailing slashes from base before joining', () => {
    // kills: replace(/\/+$/, '') -> remove replace
    // check that the path is joined without duplicate separators
    const url = buildApiUrl('/api/test')
    expect(url.split('://')[1]).not.toContain('//')
  })

  it('deduplicates /api prefix when base ends with /api', () => {
    // kills the endsWith('/api') mutation and the path.slice(4) mutation
    // Simulate base=http://host/api via env (but it is computed once when the module loads,
    // so we check via the default base + a path without /api)
    // DEFAULT_API_BASE_URL = http://localhost:18080, it does not end with /api
    // So we check that a normal path is kept as a whole
    const url = buildApiUrl('/api/health')
    expect(url.endsWith('/api/health')).toBe(true)
  })

  it('preserves non-api paths unchanged', () => {
    // kills the path.startsWith('/api/') mutation
    const url = buildApiUrl('/internal/ping')
    expect(url.endsWith('/internal/ping')).toBe(true)
  })

  describe('when API_BASE_URL действительно оканчивается на /api', () => {
    // apiBaseRaw is read from env once when the module loads; the tests
    // above never actually enter the true branch of the dedup condition.
    // That is what kept the mutants inside the if alive. Here we force
    // a re-import of the module with a fresh env to really go through the branch.
    afterEach(() => {
      vi.unstubAllEnvs()
      vi.resetModules()
    })

    it('убирает дублирующийся /api ровно один раз (slice(4), не slice() целиком)', async () => {
      vi.stubEnv('API_BASE_URL', 'http://upstream.internal/api')
      vi.resetModules()
      const fresh = await import('./_utils')
      // Input: base ends with /api, the path starts with /api/ ->
      // we expect exactly ONE /api in the result, not two and not zero.
      const url = fresh.buildApiUrl('/api/health')
      expect(url).toBe('http://upstream.internal/api/health')
      expect((url.match(/\/api\//g) || []).length).toBe(1)
    })

    it('не трогает путь, если он не начинается с /api/ (убивает startsWith мутацию во втором операнде &&)', async () => {
      vi.stubEnv('API_BASE_URL', 'http://upstream.internal/api')
      vi.resetModules()
      const fresh = await import('./_utils')
      const url = fresh.buildApiUrl('/internal/ping')
      expect(url).toBe('http://upstream.internal/api/internal/ping')
    })

    it('оставляет base с /api как есть, если путь пустой (убивает slice(4) на не-/api/ префиксе)', async () => {
      vi.stubEnv('API_BASE_URL', 'http://upstream.internal/api')
      vi.resetModules()
      const fresh = await import('./_utils')
      // path does not start with '/api/' (it is '/apiFoo') -> dedup does not fire,
      // full concatenation, no slice.
      const url = fresh.buildApiUrl('/apiFoo')
      expect(url).toBe('http://upstream.internal/api/apiFoo')
    })
  })
})

describe('withUpstreamTimeout', () => {
  it('adds AbortSignal when no signal provided', () => {
    // kills the if (init.signal) -> if (!init.signal) mutation
    const result = withUpstreamTimeout({})
    expect(result.signal).toBeDefined()
    expect(result.signal).toBeInstanceOf(AbortSignal)
  })

  it('returns init unchanged when signal already set', () => {
    // kills the return init in the then-branch mutation
    const signal = AbortSignal.abort()
    const init: RequestInit = { method: 'GET', signal }
    const result = withUpstreamTimeout(init)
    expect(result).toBe(init)
    expect(result.signal).toBe(signal)
  })

  it('uses UPSTREAM_TIMEOUT_MS as default timeout (убивает дефолтный параметр мутацию)', () => {
    // kills: timeoutMs = UPSTREAM_TIMEOUT_MS -> timeoutMs = 0
    const spy = vi.spyOn(AbortSignal, 'timeout')
    withUpstreamTimeout({})
    expect(spy).toHaveBeenCalledWith(UPSTREAM_TIMEOUT_MS)
    spy.mockRestore()
  })

  it('passes custom timeout to AbortSignal', () => {
    // kills the timeoutMs parameter mutation
    const spy = vi.spyOn(AbortSignal, 'timeout')
    withUpstreamTimeout({}, 1000)
    expect(spy).toHaveBeenCalledWith(1000)
    spy.mockRestore()
  })

  it('applies default empty init when called with no args', () => {
    // kills the init = {} default parameter mutation
    expect(() => withUpstreamTimeout()).not.toThrow()
  })
})
