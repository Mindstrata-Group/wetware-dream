import { serverApiBase } from '@/lib/apiBase'

const apiBaseRaw = serverApiBase()

export const UPSTREAM_TIMEOUT_MS = 3500

export function buildApiUrl(path: string) {
  const normalizedBase = apiBaseRaw.replace(/\/+$/, '')
  if (normalizedBase.endsWith('/api') && path.startsWith('/api/')) {
    return `${normalizedBase}${path.slice(4)}`
  }
  return `${normalizedBase}${path}`
}

export function withUpstreamTimeout(init: RequestInit = {}, timeoutMs = UPSTREAM_TIMEOUT_MS): RequestInit {
  if (init.signal) return init
  return { ...init, signal: AbortSignal.timeout(timeoutMs) }
}
