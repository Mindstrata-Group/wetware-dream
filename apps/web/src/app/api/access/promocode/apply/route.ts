import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../../_utils'

export async function POST(request: Request) {
  const raw = await request.text()
  try {
    const upstream = await fetch(buildApiUrl('/api/access/promocode/apply'), withUpstreamTimeout({
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        cookie: request.headers.get('cookie') || '',
      },
      body: raw,
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
    return NextResponse.json({ ok: false, code: 'server_error', error: 'Ошибка подключения к API' }, { status: 502 })
  }
}
