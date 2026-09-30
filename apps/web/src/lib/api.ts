import { publicApiBase } from './apiBase'

export const apiBase = publicApiBase

export class ApiError extends Error {
  status: number
  payload: any
  debug: any

  constructor(message: string, status: number, payload: any = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.payload = payload
    this.debug = payload?.debug
  }
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`${apiBase}${path}`, {
    ...init,
    credentials: 'include',
    headers: {
      ...(init.body ? { 'Content-Type': 'application/json' } : {}),
      ...(init.headers || {}),
    },
  })

  const text = await res.text()
  const contentType = res.headers.get('content-type') || ''
  const isJson = contentType.toLowerCase().includes('application/json')

  let payload: any = {}
  if (text) {
    if (!isJson) {
      throw new ApiError('Сервер вернул не JSON-ответ. Проверьте роут и прокси после редизайна.', res.status, {
        error: 'non-json-response',
        contentType,
        preview: text.slice(0, 240),
        path,
      })
    }
    try {
      payload = JSON.parse(text)
    } catch {
      throw new ApiError('Некорректный JSON в ответе сервера.', res.status, {
        error: 'invalid-json',
        contentType,
        preview: text.slice(0, 240),
        path,
      })
    }
  }

  if (!res.ok || payload.ok === false) {
    throw new ApiError(payload.error || `HTTP ${res.status}`, res.status, payload)
  }
  return payload as T
}

export function roleHome(role?: string) {
  if (role === 'owner' || role === 'admin' || role === 'tester') return '/profile'
  if (role === 'billing_admin' || role === 'content_admin' || role === 'support') return '/profile'
  return '/chat'
}
