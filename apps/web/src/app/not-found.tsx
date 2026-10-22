import Link from 'next/link'
import { getDict } from '@/lib/i18n/server'
import type { Dictionary } from '@/lib/i18n/types'

export type NotFoundDictionary = {
  title: string
  message: string
  homeButton: string
  chatButton: string
}

export const notFoundTranslations: Dictionary<NotFoundDictionary> = {
  ru: {
    title: '404',
    message: 'Эта страница не существует или была перемещена',
    homeButton: 'На главную',
    chatButton: 'В чат',
  },
  en: {
    title: '404',
    message: 'This page does not exist or has moved',
    homeButton: 'Home',
    chatButton: 'Chat',
  },
}

export default async function NotFound() {
  const t = await getDict(notFoundTranslations)

  return (
    <div className="ms-page" style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: '24px' }}>
      <div className="ms-panel" style={{ maxWidth: '440px', width: '100%', textAlign: 'center', padding: '40px 24px', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '16px' }}>
        <h1 style={{ fontSize: '56px', fontWeight: 800, margin: 0, lineHeight: 1 }}>
          {t.title}
        </h1>
        <p style={{ margin: 0, color: 'var(--muted)', fontSize: '15px', lineHeight: 1.5 }}>
          {t.message}
        </p>
        <div style={{ display: 'flex', gap: '12px', justifyContent: 'center', marginTop: '8px', flexWrap: 'wrap' }}>
          <Link href="/" className="ms-button ms-button-primary" style={{ textDecoration: 'none' }}>
            {t.homeButton}
          </Link>
          <Link href="/chat" className="ms-button ms-button-ghost" style={{ textDecoration: 'none' }}>
            {t.chatButton}
          </Link>
        </div>
      </div>
    </div>
  )
}
