import { cookies } from 'next/headers'
import { redirect } from 'next/navigation'
import { serverApiBase } from '@/lib/apiBase'
import { MessageContent } from '@/components/MessageContent'

async function getDialog(id: string) {
  const apiBase = serverApiBase()
  const cookieHeader = (await cookies()).toString()
  const res = await fetch(`${apiBase}/api/admin/dialogs/${id}`, {
    cache: 'no-store',
    headers: cookieHeader ? { cookie: cookieHeader } : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (res.status === 401) redirect('/login')
  if (res.status === 403) return { ok: false, error: 'Нет доступа к админке' }
  if (!res.ok || data.ok === false) return { ok: false, error: data.error || `HTTP ${res.status}` }
  return data
}

export default async function AdminDialogDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const data = await getDialog(id)
  const dialog = data.dialog
  const messages = Array.isArray(dialog?.messages) ? dialog.messages : []

  return (
    <main className="container">
      <section className="hero">
        <span className="badge">Admin</span>
        <h1 className="title">Dialog #{id}</h1>
      </section>

      <section className="card" style={{ marginTop: 24 }}>
        {data.error ? <div className="ms-error-box">{data.error}</div> : null}
        {!data.error && !dialog ? (
          <p>Диалог не найден.</p>
        ) : !data.error ? (
          <>
            <div className="muted">Режим: {dialog.modeName}</div>
            <div className="muted">User: {dialog.userId}</div>

            <div style={{ display: 'grid', gap: 12, marginTop: 16 }}>
              {messages.length === 0 ? (
                <div className="card">
                  <div className="muted">Сообщений пока нет</div>
                </div>
              ) : (
                messages.map((msg: any) => (
                  <div key={`${msg.id}-${msg.role}`} className="card">
                    <div className="muted">{msg.role}</div>
                    <MessageContent content={msg.content} />
                  </div>
                ))
              )}
            </div>
          </>
        ) : null}
      </section>
    </main>
  )
}
