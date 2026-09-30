import { describe, expect, it, vi } from 'vitest'
import { REGISTER_OAUTH_HREF, buildRegisterOauthHref, redirectToRegisterOauth, safeRegisterNext } from './oauth'

describe('register oauth P0', () => {
  it('keeps Yandex registration redirect pointed at profile onboarding', () => {
    expect(REGISTER_OAUTH_HREF).toBe('/api/auth/oauth/yandex/start?next=%2Fprofile&intent=register')
  })

  it('keeps promo return path when registration starts from promocode activation', () => {
    expect(buildRegisterOauthHref('/access?force=promo')).toBe('/api/auth/oauth/yandex/start?next=%2Faccess%3Fforce%3Dpromo&intent=register')
  })

  it('rejects unsafe registration return paths', () => {
    expect(safeRegisterNext('https://evil.test')).toBe('/profile')
    expect(safeRegisterNext('//evil.test')).toBe('/profile')
    expect(safeRegisterNext('/admin')).toBe('/profile')
    expect(safeRegisterNext('/access?force=promo')).toBe('/access?force=promo')
  })

  it('redirects through Location.assign so component tests can guard the OAuth edge', () => {
    const location = { assign: vi.fn() }
    redirectToRegisterOauth('/access?force=promo', location as unknown as Pick<Location, 'assign'>)
    expect(location.assign).toHaveBeenCalledWith('/api/auth/oauth/yandex/start?next=%2Faccess%3Fforce%3Dpromo&intent=register')
  })
})
