import { NextResponse } from 'next/server'
import { buildApiUrl, withUpstreamTimeout } from '../../_utils'

export async function POST(request: Request) {
  const raw = await request.text()

  let body = raw
  try {
    const parsed = JSON.parse(raw) as { code?: string; promoCode?: string }
    if (!parsed.code && parsed.promoCode) {
      body = JSON.stringify({ ...parsed, code: parsed.promoCode })
    }
  } catch {
    // keep raw body as-is
  }

  try {
    const upstream = await fetch(buildApiUrl('/api/access/promocode/apply'), withUpstreamTimeout({
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        cookie: request.headers.get('cookie') || '',
      },
      body,
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
