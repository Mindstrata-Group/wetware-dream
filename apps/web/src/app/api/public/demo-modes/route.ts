import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../_utils'

// Force this route to be dynamic (runtime-only) so Next.js never tries to
// pre-render / statically generate it during `next build`.  Without this,
// Next.js attempts to call the upstream Go API during the static generation
// phase — where the API is not running — producing 502 errors that bloat
// build logs and add spurious failure noise.
export const dynamic = 'force-dynamic'
export const revalidate = 0

const CACHE_CONTROL = 'public, max-age=60, s-maxage=300, stale-while-revalidate=86400'

export async function GET() {
  try {
    const upstream = await fetch(buildApiUrl('/api/public/demo-modes'), withUpstreamTimeout({
      next: { revalidate: 300 },
    }))

    const payload = await upstream.text()
    const headers = new Headers()

    headers.set('content-type', upstream.headers.get('content-type') || 'application/json')
    headers.set('cache-control', upstream.ok ? CACHE_CONTROL : 'no-store')

    if (!upstream.ok) {
      console.error('[api/public/demo-modes] upstream error', {
        status: upstream.status,
        body: payload,
      })
    }

    return new NextResponse(payload, {
      status: upstream.status,
      headers,
    })
  } catch (error) {
    console.error('[api/public/demo-modes] fetch failed', error)
    return NextResponse.json(
      { ok: false, error: 'demo_modes_unavailable' },
      {
        status: 503,
        headers: { 'cache-control': 'no-store' },
      },
    )
  }
}