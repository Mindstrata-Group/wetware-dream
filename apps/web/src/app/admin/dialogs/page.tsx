import { cookies } from 'next/headers'
import { redirect } from 'next/navigation'
import { serverApiBase } from '@/lib/apiBase'

async function getDialogs() {
  const apiBase = serverApiBase()
  const cookieHeader = (await cookies()).toString()
  const res = await fetch(`${apiBase}/api/admin/dialogs`, {
    cache: 'no-store',
    headers: cookieHeader ? { cookie: cookieHeader } : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (res.status === 401) redirect('/login')
  if (res.status === 403) return { ok: false, error: 'Нет доступа к админке', dialogs: [] }
  if (!res.ok || data.ok === false) return { ok: false, error: data.error || `HTTP ${res.status}`, dialogs: [] }
  return data
}

export default async function AdminDialogsPage() {
  const data = await getDialogs()
  const dialogs = data.dialogs || []

  return (
    <main className="container">
      <section className="hero">
        <span className="badge">Admin</span>
        <h1 className="title">Диалоги</h1>
      </section>
      <section className="card" style={{ marginTop: 24 }}>
        {data.error ? <div className="ms-error-box">{data.error}</div> : null}
        {!data.error && dialogs.length === 0 ? <div className="ms-info-box">Диалогов нет</div> : null}
        <div style={{ display: 'grid', gap: 12 }}>
          {dialogs.map((dialog: any) => (
            <a key={dialog.dialogId} className="card" href={`/admin/dialogs/${dialog.dialogId}`}>
              <div><strong>Dialog #{dialog.dialogId}</strong> · {dialog.modeName}</div>
              <div className="muted">User #{dialog.userId} · сообщений: {dialog.messageCount}</div>
            </a>
          ))}
        </div>
      </section>
    </main>
  )
}
