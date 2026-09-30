import { describe, expect, it } from 'vitest'

import {
  DEFAULT_SHOP_BENEFITS,
  DEFAULT_SHOP_TABS,
  DEFAULT_SHOP_TARIFF_GROUPS,
  normalizeShopBenefits,
  normalizeShopLifecycleStatuses,
  normalizeShopPaymentNotes,
  normalizeShopPeriods,
  normalizeShopTabs,
  normalizeShopTariffGroups,
  shopItemEnabled,
} from './shopContent'

describe('shopContent', () => {
  it('treats admin disable words as false for runtime-hideable shop blocks', () => {
    expect(shopItemEnabled(false)).toBe(false)
    expect(shopItemEnabled(true)).toBe(true)  // kills the return !value mutation
    expect(shopItemEnabled('off')).toBe(false)
    expect(shopItemEnabled('выкл')).toBe(false)
    expect(shopItemEnabled('disabled')).toBe(false)
    expect(shopItemEnabled('0')).toBe(false)
    expect(shopItemEnabled('false')).toBe(false)
    expect(shopItemEnabled('no')).toBe(false)
    expect(shopItemEnabled('draft')).toBe(false)
    expect(shopItemEnabled('да')).toBe(true)
    expect(shopItemEnabled(undefined)).toBe(true)
    expect(shopItemEnabled(42)).toBe(true)  // not boolean, not string -> true
  })

  it('keeps only enabled and complete tabs, periods and copy blocks', () => {
    expect(normalizeShopTabs([
      { label: 'Для спикера', groupName: 'Спикеры', enabled: true },
      { label: 'Архив', groupName: 'Архив', enabled: 'off' },
      { label: '', groupName: '', enabled: true },
    ])).toEqual([
      { label: 'Для спикера', groupName: 'Спикеры', description: '', badge: '', enabled: true },
    ])

    expect(normalizeShopPeriods([
      { id: '1m', label: '1 месяц', months: '1', discountPercent: '0', enabled: true },
      { id: '3m', label: '3 месяца', months: 3, discountPercent: '150', enabled: true },
      { id: 'bad', label: 'Ошибка', months: 'abc', enabled: true },
      { id: 'hidden', label: 'Скрытый', months: 12, enabled: 'draft' },
    ])).toEqual([
      { id: '1m', label: '1 месяц', months: 1, discountPercent: 0, badge: '', enabled: true },
      { id: '3m', label: '3 месяца', months: 3, discountPercent: 95, badge: '', enabled: true },
    ])

    expect(normalizeShopBenefits([
      { title: 'Контроль', body: 'Видно продление', enabled: true },
      { title: 'Скрыто', body: 'Не показывать', enabled: 'disabled' },
    ])).toEqual([
      { title: 'Контроль', body: 'Видно продление', groupName: '*', tariffName: '', enabled: true },
    ])

    expect(normalizeShopPaymentNotes([
      { title: 'Оплата', body: 'Через YooKassa', enabled: true },
      { title: 'Пусто', body: '', enabled: true },
    ])).toEqual([
      { title: 'Оплата', body: 'Через YooKassa', enabled: true },
    ])

    expect(normalizeShopLifecycleStatuses([
      { status: 'renewal', label: 'Продление', description: 'В профиле можно отключить', enabled: true },
      { status: 'hidden', label: 'Скрыто', description: 'Не показывать', enabled: 'no' },
    ])).toEqual([
      { status: 'renewal', label: 'Продление', description: 'В профиле можно отключить', enabled: true },
    ])
  })

  it('normalizes tariff groups and falls back to default when empty', () => {
    // normalizeShopTariffGroups is not covered at all
    expect(normalizeShopTariffGroups([
      { groupName: 'Бизнес', title: 'Для бизнеса', description: 'Описание', enabled: true },
      { groupName: 'Скрытая', enabled: 'off' },
    ])).toEqual([
      { groupName: 'Бизнес', title: 'Для бизнеса', description: 'Описание', badge: '', enabled: true },
    ])
    // groupName || '*' fallback
    expect(normalizeShopTariffGroups([
      { groupName: '', title: 'Без группы', description: 'Д', enabled: true },
    ])).toEqual([
      { groupName: '*', title: 'Без группы', description: 'Д', badge: '', enabled: true },
    ])
    // empty array → DEFAULT_SHOP_TARIFF_GROUPS
    const fromDefault = normalizeShopTariffGroups([])
    expect(fromDefault.length).toBeGreaterThan(0)
    expect(fromDefault[0].groupName).toBeDefined()
  })

  it('clamps period discount between 0 and 95 and excludes zero-month periods', () => {
    // kills the mutations Math.max(0,...) and Math.min(95,...) and months > 0
    expect(normalizeShopPeriods([
      { id: 'neg', label: 'Скидка отрицательная', months: 1, discountPercent: -10, enabled: true },
    ])[0].discountPercent).toBe(0)

    expect(normalizeShopPeriods([
      { id: 'zero-months', label: 'Нулевой срок', months: 0, enabled: true },
    ])).toEqual([])
  })

  it('falls back to DEFAULT blocks when empty arrays are passed', () => {
    expect(normalizeShopTabs([])).toEqual(normalizeShopTabs(DEFAULT_SHOP_TABS))
    expect(normalizeShopBenefits([])[0]?.title).toBeDefined()
  })

  it('normalizeShopLifecycleStatuses filters items missing required fields', () => {
    // kills the mutations: the item.status && ... && item.description condition
    // An empty label is not filtered (falls back to status); an empty status/description is filtered
    expect(normalizeShopLifecycleStatuses([
      { status: '', label: 'Метка', description: 'Описание', enabled: true }, // empty status -> filtered
      { status: 'active', label: 'Метка', description: '', enabled: true },  // empty description -> filtered
      { status: 'renewal', label: 'Продление', description: 'Ок', enabled: true },
    ])).toEqual([
      { status: 'renewal', label: 'Продление', description: 'Ок', enabled: true },
    ])
  })

  it('normalizeShopBenefits uses groupName * as default and filters items with empty body', () => {
    // kills: item.groupName || '*' -> item.groupName and the item.title && item.body condition
    expect(normalizeShopBenefits([
      { title: 'С группой', body: 'Текст', groupName: 'Спикеры', enabled: true },
      { title: 'Без группы', body: 'Текст', enabled: true },
      { title: 'Без тела', body: '', enabled: true },
    ])).toEqual([
      { title: 'С группой', body: 'Текст', groupName: 'Спикеры', tariffName: '', enabled: true },
      { title: 'Без группы', body: 'Текст', groupName: '*', tariffName: '', enabled: true },
    ])
  })

  it('keeps default storefront copy focused on tasks and automatic mode selection', () => {
    expect(DEFAULT_SHOP_TABS[0].badge).toBe('сам выберет режим')
    expect(DEFAULT_SHOP_TABS[0].description).toMatch(/без ручного выбора режима/i)
    expect(DEFAULT_SHOP_TARIFF_GROUPS[0].description).toMatch(/подбираться по задаче/i)
    expect(DEFAULT_SHOP_BENEFITS[0].title).toBe('Не нужно угадывать режим')
    expect(DEFAULT_SHOP_BENEFITS[0].body).toMatch(/подходящую методику/i)
  })
})
