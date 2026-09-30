import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../_utils'

export const dynamic = 'force-dynamic'
export const revalidate = 0

const CACHE_CONTROL = 'public, max-age=60, s-maxage=300, stale-while-revalidate=86400'

export async function GET() {
  try {
    const upstream = await fetch(buildApiUrl('/api/public/tariffs'), withUpstreamTimeout({
      next: { revalidate: 300 },
    }))

    const payload = await upstream.text()
    const headers = new Headers()
    headers.set('content-type', upstream.headers.get('content-type') || 'application/json')
    headers.set('cache-control', upstream.ok ? CACHE_CONTROL : 'no-store')

    if (!upstream.ok) {
      console.error('[api/public/tariffs] upstream error', {
        status: upstream.status,
        body: payload,
      })
    }

    return new NextResponse(payload, {
      status: upstream.status,
      headers,
    })
  } catch (error) {
    console.error('[api/public/tariffs] fetch failed', error)
    return NextResponse.json(
      { ok: false, error: 'tariffs_unavailable' },
      {
        status: 503,
        headers: { 'cache-control': 'no-store' },
      },
    )
  }
}
