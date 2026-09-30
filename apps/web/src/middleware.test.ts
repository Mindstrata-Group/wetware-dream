import { describe, expect, it } from 'vitest'
import { NextRequest } from 'next/server'
import { middleware } from './middleware'

function makeRequest(pathname: string, cookies: Record<string, string> = {}, headers: Record<string, string> = {}) {
  const url = `http://localhost${pathname}`
  const req = new NextRequest(url, { headers })
  for (const [name, value] of Object.entries(cookies)) {
    req.cookies.set(name, value)
  }
  return req
}

describe('middleware auth guard', () => {
  it('passes unprotected routes without redirect (убивает isProtected мутацию)', () => {
    const response = middleware(makeRequest('/'))
    expect(response.status).toBe(200)
    expect(response.headers.get('location')).toBeNull()
  })

  it('passes /access route unprotected', () => {
    // kills: PROTECTED extension (in case /access gets added by accident)
    const response = middleware(makeRequest('/access'))
    expect(response.status).toBe(200)
  })

  it('redirects /chat without any auth cookie (убивает canEnter мутацию)', () => {
    // kills: session?.value || ... -> && (never lets in)
    const response = middleware(makeRequest('/chat'))
    expect(response.status).toBe(307)
    expect(response.headers.get('location')).toContain('/login')
  })

  it('allows /chat with guest cookie (убивает pathname === /chat мутацию)', () => {
    // kills: pathname === "/chat" -> false (a guest cannot enter)
    const response = middleware(makeRequest('/chat', { mindstrata_guest_user_id: 'guest-123' }))
    expect(response.status).toBe(200)
  })

  it('does NOT allow /profile with only guest cookie (убивает pathname === /chat мутацию)', () => {
    // kills: pathname === "/chat" -> pathname === "/profile"
    const response = middleware(makeRequest('/profile', { mindstrata_guest_user_id: 'guest-123' }))
    expect(response.status).toBe(307)
    expect(response.headers.get('location')).toContain('/login')
  })

  it('allows /profile with session cookie', () => {
    // kills the session?.value -> null mutation
    const response = middleware(makeRequest('/profile', { mindstrata_session: 'sess-abc' }))
    expect(response.status).toBe(200)
  })

  it('allows /chat sub-paths with session cookie (убивает startsWith мутацию)', () => {
    // kills: pathname.startsWith(p + "/") -> false (sub-path /chat/history is not protected)
    const response = middleware(makeRequest('/chat/history', { mindstrata_session: 'sess-abc' }))
    expect(response.status).toBe(200)
  })

  it('redirects /chat sub-path without cookie', () => {
    // kills: pathname.startsWith(p + "/") -> pathname.startsWith(p) (no separator)
    const response = middleware(makeRequest('/chat/history'))
    expect(response.status).toBe(307)
  })

  it('redirects /profile sub-path without cookie', () => {
    const response = middleware(makeRequest('/profile/settings'))
    expect(response.status).toBe(307)
    expect(response.headers.get('location')).toContain('/login')
  })

  it('redirect URL has no query string (убивает loginUrl.search = "" мутацию)', () => {
    // kills: loginUrl.search = "" -> not reset (URL like /login?some=param)
    const response = middleware(makeRequest('/chat?foo=bar'))
    const location = response.headers.get('location') ?? ''
    expect(new URL(location).search).toBe('')
    expect(new URL(location).pathname).toBe('/login')
  })

  it('protected-guard имеет приоритет над locale-redirect: авторизован + en-cookie на /chat → редирект на /en/chat, а не rewrite сразу', () => {
    // resolvedPathname=/chat (not hasEnPrefix) -> cookieLocale=en -> locale=en, redirectToEn=true.
    // isProtected=true, but there is a session -> canEnter=true, the early return does NOT fire.
    // Then the redirectToEn=true block redirects to /en/chat (not a rewrite).
    const response = middleware(makeRequest('/chat', { mindstrata_session: 'sess-abc', mindstrata_locale: 'en' }))
    expect(response.status).toBe(307)
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/en/chat')
  })

  it('protected-guard блокирует НЕавторизованного даже с en-cookie: редирект сразу на /en/login, а не на /login с последующим en-redirect', () => {
    // Kills the order mutation: if redirectToEn were checked before isProtected,
    // the result would be /en/chat (rewrite) instead of an immediate /en/login.
    const response = middleware(makeRequest('/chat', { mindstrata_locale: 'en' }))
    expect(response.status).toBe(307)
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/en/login')
  })
})

describe('middleware locale cookie attributes', () => {
  it('locale cookie: точные maxAge/path/sameSite (убивает мутации числовой константы и опций)', () => {
    const response = middleware(makeRequest('/'))
    const cookie = response.cookies.get('mindstrata_locale')
    expect(cookie?.maxAge).toBe(60 * 60 * 24 * 365)
    expect(cookie?.path).toBe('/')
    expect(cookie?.sameSite).toBe('lax')
  })
})

describe('middleware pass-through vs rewrite', () => {
  it('обычный не-/en путь использует NextResponse.next() — БЕЗ x-middleware-rewrite заголовка', () => {
    const response = middleware(makeRequest('/about'))
    expect(response.status).toBe(200)
    expect(response.headers.get('x-middleware-rewrite')).toBeNull()
    expect(response.headers.get('x-middleware-request-x-ms-locale')).toBe('ru')
  })

  it('статический файл с точкой в НЕпоследнем сегменте пути НЕ считается статикой (убивает regex $ anchor мутацию)', () => {
    // STATIC_FILE = /\.[^/]+$/: the dot must be in the last segment.
    const response = middleware(makeRequest('/v1.2/about'))
    expect(response.status).toBe(200)
    // Since it is not static, it goes through normal localisation (the cookie is set).
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('ru')
    expect(response.headers.get('x-middleware-request-x-ms-locale')).toBe('ru')
  })

  it('статический файл не получает ни locale-cookie, ни x-ms-locale заголовок (ранний return до всей остальной логики)', () => {
    const response = middleware(makeRequest('/favicon.ico'))
    expect(response.status).toBe(200)
    expect(response.cookies.get('mindstrata_locale')).toBeUndefined()
    expect(response.headers.get('x-middleware-request-x-ms-locale')).toBeNull()
  })
})

describe('middleware locale routing', () => {
  it('defaults to ru when there is no locale signal (no cookie, no Accept-Language)', () => {
    const response = middleware(makeRequest('/'))
    expect(response.status).toBe(200)
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('ru')
    expect(response.headers.get('location')).toBeNull()
  })

  it('first-time visitor with English Accept-Language stays on RU without auto-redirect', () => {
    // geo auto-redirect is disabled: EN is chosen only explicitly via LanguageSwitcher
    const response = middleware(makeRequest('/about', {}, { 'accept-language': 'en-US,en;q=0.9' }))
    expect(response.status).toBe(200)
    expect(response.headers.get('location')).toBeNull()
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('ru')
  })

  it('keeps ru for Russian Accept-Language', () => {
    const response = middleware(makeRequest('/', {}, { 'accept-language': 'ru-RU,ru;q=0.9,en;q=0.8' }))
    expect(response.status).toBe(200)
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('ru')
  })

  it('CF-IPCountry and Accept-Language are ignored without explicit cookie: defaults to RU', () => {
    // without an explicit choice cookie: always RU, regardless of geo headers
    const response = middleware(
      makeRequest('/', {}, { 'cf-ipcountry': 'US', 'accept-language': 'en-US,en;q=0.9' }),
    )
    expect(response.status).toBe(200)
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('ru')
  })

  it('rewrites /en/* to the unprefixed page and sets x-ms-locale=en', () => {
    const response = middleware(makeRequest('/en/about'))
    expect(response.status).toBe(200)
    expect(response.headers.get('x-middleware-rewrite')).toBe('http://localhost/about')
    expect(response.headers.get('x-middleware-request-x-ms-locale')).toBe('en')
    expect(response.cookies.get('mindstrata_locale')?.value).toBe('en')
  })

  it('rewrites bare /en to the home page', () => {
    const response = middleware(makeRequest('/en'))
    expect(response.headers.get('x-middleware-rewrite')).toBe('http://localhost/')
  })

  it('returning ru-cookie visitor stays on unprefixed URLs even with English Accept-Language', () => {
    const response = middleware(
      makeRequest('/about', { mindstrata_locale: 'ru' }, { 'accept-language': 'en-US,en;q=0.9' }),
    )
    expect(response.status).toBe(200)
    expect(response.headers.get('location')).toBeNull()
    expect(response.headers.get('x-middleware-request-x-ms-locale')).toBe('ru')
  })

  it('returning en-cookie visitor on an unprefixed URL is redirected to /en/*', () => {
    const response = middleware(makeRequest('/about', { mindstrata_locale: 'en' }))
    expect(response.status).toBe(307)
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/en/about')
  })

  it('redirects unauthenticated /en/chat to /en/login', () => {
    const response = middleware(makeRequest('/en/chat'))
    expect(response.status).toBe(307)
    expect(new URL(response.headers.get('location') ?? '').pathname).toBe('/en/login')
  })

  it('does not localize static files', () => {
    const response = middleware(makeRequest('/brand/logo-mark.png', {}, { 'accept-language': 'en-US,en;q=0.9' }))
    expect(response.status).toBe(200)
    expect(response.headers.get('location')).toBeNull()
    expect(response.cookies.get('mindstrata_locale')).toBeUndefined()
  })
})
