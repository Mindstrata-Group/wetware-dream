import { cleanup } from '@testing-library/react'
import { afterEach, vi } from 'vitest'

// Tests with Date (dailyUnlockLabel and others) must be deterministic regardless
// of the server TZ; fix UTC before any initialisation.
process.env.TZ = 'UTC'

// Polyfills for jsdom: it has no IntersectionObserver / ResizeObserver /
// requestAnimationFrame, and they are used in about/counter and elsewhere.

class MockIntersectionObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() { return [] }
  root = null
  rootMargin = ''
  thresholds = []
}
globalThis.IntersectionObserver = MockIntersectionObserver as unknown as typeof IntersectionObserver

class MockResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver

if (typeof globalThis.requestAnimationFrame === 'undefined') {
  globalThis.requestAnimationFrame = (cb: FrameRequestCallback) => setTimeout(cb, 16)
}

const originalFetch = globalThis.fetch

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  if (originalFetch === undefined) {
    delete (globalThis as Partial<typeof globalThis>).fetch
  } else {
    globalThis.fetch = originalFetch
  }
  cleanup()
})
