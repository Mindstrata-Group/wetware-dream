import { describe, expect, it } from 'vitest'

import { LOCAL_API_BASE_URL, resolvePublicApiBase, resolveServerApiBase } from './apiBase'

describe('resolvePublicApiBase', () => {
  it('falls back to the local API when the variable is not set', () => {
    expect(resolvePublicApiBase(undefined)).toBe(LOCAL_API_BASE_URL)
  })

  // An empty value means "same origin": one prebuilt image serves any domain.
  it('keeps an empty value so the browser calls the site itself', () => {
    expect(resolvePublicApiBase('')).toBe('')
  })

  it('uses an explicit URL as is', () => {
    expect(resolvePublicApiBase('https://example.com')).toBe('https://example.com')
  })
})

describe('resolveServerApiBase', () => {
  it('prefers the in-network API address', () => {
    expect(resolveServerApiBase('http://api:18080', 'https://example.com')).toBe('http://api:18080')
  })

  it('never returns a relative base on the server', () => {
    expect(resolveServerApiBase(undefined, '')).toBe(LOCAL_API_BASE_URL)
    expect(resolveServerApiBase('', undefined)).toBe(LOCAL_API_BASE_URL)
  })

  it('falls back to the public URL when no in-network address is set', () => {
    expect(resolveServerApiBase(undefined, 'https://example.com')).toBe('https://example.com')
  })
})
