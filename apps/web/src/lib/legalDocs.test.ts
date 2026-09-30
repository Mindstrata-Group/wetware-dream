import { describe, expect, it } from 'vitest'
import { DEFAULT_LEGAL_DOCS, legalDocContentKey, type LegalDocTile } from './legalDocs'

describe('legalDocContentKey', () => {
  it('builds content key from slug and field (убивает строковые мутации)', () => {
    // kills the "privacy.doc." -> "" or ".${field}" -> "" mutations
    expect(legalDocContentKey('terms', 'title')).toBe('privacy.doc.terms.title')
    expect(legalDocContentKey('terms', 'body')).toBe('privacy.doc.terms.body')
    expect(legalDocContentKey('terms', 'subtitle')).toBe('privacy.doc.terms.subtitle')
    expect(legalDocContentKey('cookies', 'title')).toBe('privacy.doc.cookies.title')
  })

  it('embeds arbitrary slug into the key', () => {
    // kills the `...${slug}...` -> static string mutation
    expect(legalDocContentKey('custom-slug', 'body')).toBe('privacy.doc.custom-slug.body')
  })

  it('порядок сегментов ключа строго slug перед field', () => {
    // kills the swap `${slug}.${field}` -> `${field}.${slug}`
    expect(legalDocContentKey('a', 'title')).toBe('privacy.doc.a.title')
    expect(legalDocContentKey('a', 'title')).not.toBe('privacy.doc.title.a')
  })
})

describe('DEFAULT_LEGAL_DOCS', () => {
  // A full array comparison: one assert kills ALL string mutations
  // (title/hint/badge -> ""), field swaps and structure changes.
  const EXPECTED: LegalDocTile[] = [
    { slug: 'terms', title: 'Пользовательское соглашение / Оферта', hint: 'Условия использования сервиса, права и обязанности сторон.' },
    { slug: 'personal-data', title: 'Политика обработки персональных данных', hint: 'Какие данные собираем, для чего и как храним.' },
    { slug: 'consent', title: 'Согласие на обработку персональных данных', hint: 'Отдельный документ, который вы даёте при регистрации.', badge: '152-ФЗ' },
    { slug: 'cookies', title: 'Политика cookies', hint: 'Технические cookies работают всегда; аналитики у нас нет.' },
    { slug: 'payments', title: 'Правила оплаты, подписки и возврата', hint: 'Тарифы, продление, отмена и возврат средств.' },
    { slug: 'contacts', title: 'Реквизиты и контакты', hint: 'Оператор, ИНН, адрес для обращений.' },
    { slug: 'withdraw', title: 'Отозвать согласие / удалить данные', hint: 'Как прекратить обработку и удалить аккаунт.' },
  ]

  it('точно совпадает с эталонным списком (значения, порядок, badge)', () => {
    expect(DEFAULT_LEGAL_DOCS).toEqual(EXPECTED)
  })

  it('содержит ровно 7 документов', () => {
    // strict equality kills the `length >= 5` and off-by-one mutations
    expect(DEFAULT_LEGAL_DOCS).toHaveLength(7)
  })

  it('только документ consent имеет badge 152-ФЗ', () => {
    // kills the badge -> "" mutations and removing/adding badge
    const withBadge = DEFAULT_LEGAL_DOCS.filter(d => d.badge !== undefined)
    expect(withBadge).toHaveLength(1)
    expect(withBadge[0].slug).toBe('consent')
    expect(withBadge[0].badge).toBe('152-ФЗ')
  })

  it('все slug уникальны и непустые', () => {
    const slugs = DEFAULT_LEGAL_DOCS.map(d => d.slug)
    expect(new Set(slugs).size).toBe(slugs.length)
    for (const s of slugs) expect(s.length).toBeGreaterThan(0)
  })
})
