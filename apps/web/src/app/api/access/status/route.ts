import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../_utils'

export async function GET(request: Request) {
  try {
    const upstream = await fetch(buildApiUrl('/api/access/status'), withUpstreamTimeout({
      method: 'GET',
      headers: { cookie: request.headers.get('cookie') || '' },
      cache: 'no-store',
    }))
    const payload = await upstream.text()
    const response = new NextResponse(payload, {
      status: upstream.status,
      headers: { 'content-type': upstream.headers.get('content-type') || 'application/json' },
    })

    const setCookie = upstream.headers.get('set-cookie')
    if (setCookie) response.headers.set('set-cookie', setCookie)
    return response
  } catch {
    return NextResponse.json({ ok: false, error: 'access_status_unavailable' }, { status: 502 })
  }
}
