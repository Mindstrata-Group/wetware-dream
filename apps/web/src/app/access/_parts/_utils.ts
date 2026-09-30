import type { Locale } from '@/lib/i18n/types'

export function buildPromoSourceLink(baseUrl: string, message: string) {
  try {
    const url = new URL(baseUrl)
    url.searchParams.set('text', message)
    return url.toString()
  } catch {
    return baseUrl
  }
}

export function normalizePromoModes(
  modes?: Array<{ modeId?: number; modeName?: string; activeTo?: string }>,
) {
  return Array.isArray(modes)
    ? modes
        .map(item => ({
          modeId: item.modeId,
          name: String(item.modeName || ''),
          activeTo: item.activeTo,
        }))
        .filter(item => item.name)
    : []
}

export function formatActiveTo(activeTo?: string, locale: Locale = 'ru') {
  if (!activeTo) return locale === 'en' ? 'no end date' : 'без даты окончания'
  try {
    return new Date(activeTo).toLocaleString(locale === 'en' ? 'en-US' : 'ru-RU')
  } catch {
    return activeTo
  }
}

export const errorMap: Record<string, string> = {
  empty: 'Введите промокод',
  not_found: 'Такого промокода нет',
  not_started: 'Промокод ещё не активен',
  expired: 'Промокод истёк',
  limit_reached: 'Лимит активаций исчерпан',
  already_used: 'Вы уже использовали этот промокод',
  auth_required:
    'Сначала войдите через Яндекс, затем промокод применится к аккаунту',
  archived: 'Промокод отключён',
  server_error: 'Ошибка сервера при проверке промокода',
}
