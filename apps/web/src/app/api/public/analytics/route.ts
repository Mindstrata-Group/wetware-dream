import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../_utils'

export const dynamic = 'force-dynamic'
export const revalidate = 0

// Counter settings are changed from the admin panel; the cache is short so that a change
// of the counter number reaches visitors within minutes, not hours.
const CACHE_CONTROL = 'public, max-age=60, s-maxage=300, stale-while-revalidate=3600'

export async function GET() {
  try {
    const upstream = await fetch(buildApiUrl('/api/public/analytics'), withUpstreamTimeout({
      next: { revalidate: 60 },
    }))

    const payload = await upstream.text()
    const headers = new Headers()
    headers.set('content-type', upstream.headers.get('content-type') || 'application/json')
    headers.set('cache-control', upstream.ok ? CACHE_CONTROL : 'no-store')

    if (!upstream.ok) {
      console.error('[api/public/analytics] upstream error', {
        status: upstream.status,
        body: payload,
      })
    }

    return new NextResponse(payload, {
      status: upstream.status,
      headers,
    })
  } catch (error) {
    console.error('[api/public/analytics] fetch failed', error)
    return NextResponse.json(
      { ok: false, error: 'analytics_unavailable' },
      {
        status: 503,
        headers: { 'cache-control': 'no-store' },
      },
    )
  }
}
