import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../../_utils'

function copySetCookie(upstream: Response, response: NextResponse) {
  const setCookie = upstream.headers.get('set-cookie')
  if (setCookie) response.headers.set('set-cookie', setCookie)
}

export async function GET(request: Request, { params }: { params: Promise<{ path?: string[] }> }) {
  const { path = [] } = await params
  const url = new URL(request.url)
  const upstreamPath = `/api/auth/oauth/${path.map(encodeURIComponent).join('/')}`
  const upstreamUrl = `${buildApiUrl(upstreamPath)}${url.search}`

  try {
    const upstream = await fetch(upstreamUrl, withUpstreamTimeout({
      method: 'GET',
      headers: { cookie: request.headers.get('cookie') || '' },
      redirect: 'manual',
      cache: 'no-store',
    }))

    const location = upstream.headers.get('location')
    if (location && upstream.status >= 300 && upstream.status < 400) {
      const response = NextResponse.redirect(location, upstream.status)
      copySetCookie(upstream, response)
      return response
    }

    const payload = await upstream.text()
    const response = new NextResponse(payload, {
      status: upstream.status,
      headers: { 'content-type': upstream.headers.get('content-type') || 'application/json' },
    })
    copySetCookie(upstream, response)
    return response
  } catch {
    return NextResponse.json({ ok: false, error: 'oauth_api_unavailable' }, { status: 502 })
  }
}
