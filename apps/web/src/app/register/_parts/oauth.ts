export const REGISTER_OAUTH_HREF = buildRegisterOauthHref('/profile')

export function safeRegisterNext(raw: string | null | undefined) {
  const next = (raw || '').trim()
  if (!next || !next.startsWith('/') || next.startsWith('//') || next.includes('\\')) return '/profile'
  if (next.startsWith('/admin') || next.startsWith('/tester') || next.startsWith('/expert')) return '/profile'
  return next
}

export function buildRegisterOauthHref(next?: string | null) {
  const params = new URLSearchParams({
    next: safeRegisterNext(next),
    intent: 'register',
  })
  return `/api/auth/oauth/yandex/start?${params.toString()}`
}

export function redirectToRegisterOauth(next?: string | null, location: Pick<Location, 'assign'> = window.location) {
  location.assign(buildRegisterOauthHref(next))
}
