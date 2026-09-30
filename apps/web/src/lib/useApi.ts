'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { apiFetch } from './api'

type CacheEntry<T> = {
  expiresAt: number
  data: T
}

type UseApiOptions<T> = {
  enabled?: boolean
  init?: RequestInit
  cacheTtlMs?: number
  onSuccess?: (data: T) => void
  onError?: (error: Error) => void
}

type UseApiState<T> = {
  data: T | null
  loading: boolean
  error: Error | null
  refetch: () => Promise<T | null>
}

const DEFAULT_CACHE_TTL_MS = 30_000
const cache = new Map<string, CacheEntry<unknown>>()
const inFlight = new Map<string, Promise<unknown>>()

function cacheKey(path: string, init?: RequestInit): string {
  const method = init?.method || 'GET'
  const body = typeof init?.body === 'string' ? init.body : ''
  return `${method} ${path} ${body}`
}

function getCached<T>(key: string): T | null {
  const hit = cache.get(key)
  if (!hit) return null
  if (Date.now() >= hit.expiresAt) {
    cache.delete(key)
    return null
  }
  return hit.data as T
}

function request<T>(key: string, path: string, init: RequestInit | undefined, cacheTtlMs: number): Promise<T> {
  const existing = inFlight.get(key) as Promise<T> | undefined
  if (existing) return existing

  const promise = apiFetch<T>(path, init)
    .then((data) => {
      cache.set(key, { data, expiresAt: Date.now() + cacheTtlMs })
      return data
    })
    .finally(() => {
      inFlight.delete(key)
    })

  inFlight.set(key, promise)
  return promise
}

export function clearApiCache(path?: string) {
  if (!path) {
    cache.clear()
    return
  }
  for (const key of cache.keys()) {
    if (key.includes(` ${path} `)) cache.delete(key)
  }
}

export function useApi<T>(path: string | null, options: UseApiOptions<T> = {}): UseApiState<T> {
  const { enabled = true, init, cacheTtlMs = DEFAULT_CACHE_TTL_MS, onSuccess, onError } = options
  const key = useMemo(() => (path ? cacheKey(path, init) : ''), [path, init])
  const [data, setData] = useState<T | null>(() => (key ? getCached<T>(key) : null))
  const [loading, setLoading] = useState(Boolean(path && enabled && !data))
  const [error, setError] = useState<Error | null>(null)
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  const refetch = useCallback(async () => {
    if (!path) return null

    const hit = getCached<T>(key)
    if (hit) {
      setData(hit)
      setLoading(false)
      setError(null)
      onSuccess?.(hit)
      return hit
    }

    setLoading(true)
    setError(null)
    try {
      const next = await request<T>(key, path, init, cacheTtlMs)
      if (mounted.current) {
        setData(next)
        setError(null)
        onSuccess?.(next)
      }
      return next
    } catch (err) {
      const normalized = err instanceof Error ? err : new Error(String(err))
      if (mounted.current) {
        setError(normalized)
        onError?.(normalized)
      }
      return null
    } finally {
      if (mounted.current) setLoading(false)
    }
  }, [cacheTtlMs, init, key, onError, onSuccess, path])

  useEffect(() => {
    if (!path || !enabled) {
      setLoading(false)
      return
    }
    void refetch()
  }, [enabled, path, refetch])

  return { data, loading, error, refetch }
}
