'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { BrandLogo } from '@/components/BrandLogo'
import { useApi } from '@/lib/useApi'

export default function ExpertPage() {
  const router = useRouter()
  const { data: payload, error: statusError } = useApi<any>('/api/expert/status')
  const [error, setError] = useState('')

  useEffect(() => {
    const message = statusError?.message || ''
    if (!message) { setError(''); return }
    if (message === 'auth required') { router.replace('/login'); return }
    setError(message === 'forbidden' ? 'Нет доступа к кабинету эксперта' : message)
  }, [router, statusError])

  return (
    <div className="ms-page">
      <header className="ms-topbar">
        <BrandLogo className="ms-brand-link ms-brand-left" />
        <nav className="ms-top-actions"><Link className="ms-button ms-button-ghost ms-button-sm" href="/profile">Профиль</Link></nav>
      </header>
      <main className="ms-access-shell">
        <section className="ms-panel ms-access-card">
          <h1>Кабинет эксперта</h1>
          {error ? <div className="ms-error-box">{error}</div> : null}
          {payload ? (
            <>
              <div className="ms-info-box">{payload.message}</div>
              <div className="card">
                <strong>Мои режимы</strong>
                <p className="muted">Создание и публикация режимов появится после включения модерации.</p>
              </div>
            </>
          ) : !error ? <div className="ms-info-box">Загрузка...</div> : null}
        </section>
      </main>
    </div>
  )
}
