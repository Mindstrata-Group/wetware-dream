export type ShopTabContent = {
  label: string
  groupName: string
  description?: string
  badge?: string
  enabled?: boolean | string
}

export type ShopTariffGroupContent = {
  groupName: string
  title?: string
  description?: string
  badge?: string
  enabled?: boolean | string
}

export type ShopPeriodContent = {
  id: string
  label: string
  months: number | string
  discountPercent?: number | string
  badge?: string
  enabled?: boolean | string
}

export type ShopBenefitContent = {
  title: string
  body: string
  groupName?: string
  tariffName?: string
  enabled?: boolean | string
}

export type ShopPaymentNoteContent = {
  title: string
  body: string
  enabled?: boolean | string
}

export type ShopLifecycleStatusContent = {
  status: string
  label: string
  description: string
  enabled?: boolean | string
}

export const DEFAULT_SHOP_TABS: ShopTabContent[] = [
  {
    label: 'Для руководителя',
    groupName: '*',
    description: 'Разобрать завал задач, сложный разговор, цену, текст или решение без ручного выбора режима.',
    badge: 'сам выберет режим',
    enabled: true,
  },
]

export const DEFAULT_SHOP_TARIFF_GROUPS: ShopTariffGroupContent[] = [
  {
    groupName: '*',
    title: 'Готовые решения',
    description: 'Выберите срок доступа. Режимы внутри тарифа будут подбираться по задаче в чате.',
    badge: 'доступ сразу после оплаты',
    enabled: true,
  },
]

export const DEFAULT_SHOP_PERIODS: ShopPeriodContent[] = [
  { id: '1m', label: '1 месяц', months: 1, enabled: true },
  { id: '3m', label: '3 месяца', months: 3, discountPercent: 5, badge: 'скидка 5%', enabled: true },
  { id: '12m', label: '1 год', months: 12, discountPercent: 15, badge: 'скидка 15%', enabled: true },
]

export const DEFAULT_SHOP_BENEFITS: ShopBenefitContent[] = [
  {
    title: 'Не нужно угадывать режим',
    body: 'Опишите ситуацию обычными словами: Стратум сам подключит подходящую методику.',
    groupName: '*',
    tariffName: '',
    enabled: true,
  },
  {
    title: 'Живой следующий шаг',
    body: 'После уточнений получите не общую справку, а ход, который можно проверить в деле.',
    groupName: '*',
    tariffName: '',
    enabled: true,
  },
]

export const DEFAULT_SHOP_PAYMENT_NOTES: ShopPaymentNoteContent[] = []

export const DEFAULT_SHOP_LIFECYCLE_STATUSES: ShopLifecycleStatusContent[] = []

export function shopTabEnabled(tab: ShopTabContent): boolean {
  return shopItemEnabled(tab.enabled)
}

export function shopItemEnabled(value: unknown): boolean {
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    return !['false', '0', 'no', 'off', 'выкл', 'draft', 'disabled'].includes(normalized)
  }
  return true
}

export function normalizeShopTabs(items: ShopTabContent[]): ShopTabContent[] {
  const source = items.length > 0 ? items : DEFAULT_SHOP_TABS
  return source
    .map((item) => ({
      label: String(item.label || item.groupName || '').trim(),
      groupName: String(item.groupName || item.label || '').trim(),
      description: String(item.description || '').trim(),
      badge: String(item.badge || '').trim(),
      enabled: item.enabled,
    }))
    .filter((item) => item.label && item.groupName && shopTabEnabled(item))
}

export function normalizeShopTariffGroups(items: ShopTariffGroupContent[]): ShopTariffGroupContent[] {
  const source = items.length > 0 ? items : DEFAULT_SHOP_TARIFF_GROUPS
  return source
    .map((item) => ({
      groupName: String(item.groupName || '*').trim(),
      title: String(item.title || item.groupName || '').trim(),
      description: String(item.description || '').trim(),
      badge: String(item.badge || '').trim(),
      enabled: item.enabled,
    }))
    .filter((item) => item.groupName && shopItemEnabled(item.enabled))
}

export function normalizeShopPeriods(items: ShopPeriodContent[]): Array<ShopPeriodContent & { months: number }> {
  const source = items.length > 0 ? items : DEFAULT_SHOP_PERIODS
  return source
    .map((item) => {
      const months = typeof item.months === 'number' ? item.months : Number(String(item.months || '').trim())
      const discountPercent = typeof item.discountPercent === 'number'
        ? item.discountPercent
        : Number(String(item.discountPercent ?? '0').trim().replace(',', '.'))
      return {
        id: String(item.id || months || '').trim(),
        label: String(item.label || '').trim(),
        months,
        discountPercent: Number.isFinite(discountPercent) ? Math.max(0, Math.min(95, discountPercent)) : 0,
        badge: String(item.badge || '').trim(),
        enabled: item.enabled,
      }
    })
    .filter((item) => item.id && item.label && Number.isFinite(item.months) && item.months > 0 && shopItemEnabled(item.enabled))
}

export function normalizeShopBenefits(items: ShopBenefitContent[]): ShopBenefitContent[] {
  const source = items.length > 0 ? items : DEFAULT_SHOP_BENEFITS
  return source
    .map((item) => ({
      title: String(item.title || '').trim(),
      body: String(item.body || '').trim(),
      groupName: String(item.groupName || '*').trim(),
      tariffName: String(item.tariffName || '').trim(),
      enabled: item.enabled,
    }))
    .filter((item) => item.title && item.body && shopItemEnabled(item.enabled))
}

export function normalizeShopPaymentNotes(items: ShopPaymentNoteContent[]): ShopPaymentNoteContent[] {
  const source = items.length > 0 ? items : DEFAULT_SHOP_PAYMENT_NOTES
  return source
    .map((item) => ({
      title: String(item.title || '').trim(),
      body: String(item.body || '').trim(),
      enabled: item.enabled,
    }))
    .filter((item) => item.title && item.body && shopItemEnabled(item.enabled))
}

export function normalizeShopLifecycleStatuses(items: ShopLifecycleStatusContent[]): ShopLifecycleStatusContent[] {
  const source = items.length > 0 ? items : DEFAULT_SHOP_LIFECYCLE_STATUSES
  return source
    .map((item) => ({
      status: String(item.status || '').trim(),
      label: String(item.label || item.status || '').trim(),
      description: String(item.description || '').trim(),
      enabled: item.enabled,
    }))
    .filter((item) => item.status && item.label && item.description && shopItemEnabled(item.enabled))
}
