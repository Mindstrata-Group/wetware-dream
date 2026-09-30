export type LegalDocTile = {
  slug: string
  title: string
  hint: string
  badge?: string
}

export const DEFAULT_LEGAL_DOCS: LegalDocTile[] = [
  { slug: 'terms', title: 'Пользовательское соглашение / Оферта', hint: 'Условия использования сервиса, права и обязанности сторон.' },
  { slug: 'personal-data', title: 'Политика обработки персональных данных', hint: 'Какие данные собираем, для чего и как храним.' },
  { slug: 'consent', title: 'Согласие на обработку персональных данных', hint: 'Отдельный документ, который вы даёте при регистрации.', badge: '152-ФЗ' },
  { slug: 'cookies', title: 'Политика cookies', hint: 'Технические cookies работают всегда; аналитики у нас нет.' },
  { slug: 'payments', title: 'Правила оплаты, подписки и возврата', hint: 'Тарифы, продление, отмена и возврат средств.' },
  { slug: 'contacts', title: 'Реквизиты и контакты', hint: 'Оператор, ИНН, адрес для обращений.' },
  { slug: 'withdraw', title: 'Отозвать согласие / удалить данные', hint: 'Как прекратить обработку и удалить аккаунт.' },
]

export function legalDocContentKey(slug: string, field: 'title' | 'subtitle' | 'body') {
  return `privacy.doc.${slug}.${field}`
}
