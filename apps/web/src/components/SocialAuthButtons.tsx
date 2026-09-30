'use client'

import { useEffect, useState } from 'react'

type ProvidersResponse = {
  ok: boolean
  providers: {
    yandex?: boolean
  }
}

type SocialAuthButtonsProps = {
  next?: string
  showForceAccount?: boolean
}

export function SocialAuthButtons({ next = '/profile', showForceAccount = false }: SocialAuthButtonsProps) {
  const [providers, setProviders] = useState<ProvidersResponse['providers'] | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false

    async function loadProviders() {
      try {
        const res = await fetch('/api/auth/oauth/providers', { credentials: 'include', cache: 'no-store' })
        const json = await res.json()
        if (!res.ok || json.ok === false) throw new Error(json.error || `HTTP ${res.status}`)
        if (!cancelled) setProviders(json.providers || {})
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : 'Не удалось проверить OAuth')
      }
    }

    void loadProviders()
    return () => {
      cancelled = true
    }
  }, [])

  const yandexEnabled = Boolean(providers?.yandex)

  if (error) return <div className="ms-error-box">{error}</div>
  if (!providers) return <div className="ms-info-box">Проверяем доступные способы входа...</div>

  if (!yandexEnabled) {
    return (
      <div className="ms-error-box">
        Вход через Яндекс не настроен. Для запуска задайте YANDEX_CLIENT_ID и YANDEX_CLIENT_SECRET в API и примените SQL-миграции.
      </div>
    )
  }

  const startUrl = `/api/auth/oauth/yandex/start?next=${encodeURIComponent(next)}`

  return (
    <div className="ms-social-auth">
      <a
        className="ms-button ms-button-primary ms-yandex-button"
        href={startUrl}
      >
        <span className="ms-yandex-icon" aria-hidden="true">Я</span>
        <span>Войти через Яндекс</span>
      </a>
      {showForceAccount ? (
        <a
          className="ms-button ms-button-ghost ms-yandex-button"
          href={`${startUrl}&force_account=1`}
        >
          <span className="ms-yandex-icon" aria-hidden="true">Я</span>
          <span>Войти другим Яндекс-аккаунтом</span>
        </a>
      ) : null}
    </div>
  )
}
