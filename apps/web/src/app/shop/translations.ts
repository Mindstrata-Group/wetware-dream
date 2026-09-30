import type { Dictionary } from "@/lib/i18n/types"

type ShopText = {
  hero: { title: string; subtitle: string }
  loadingText: string
  emptyState: string
  tabsLabel: string
  closed: { title: string; subtitle: string }
  nav: { chatLabel: string; accessLabel: string }
  periodsLabel: string
  defaultGroupLabel: string
  buyLabel: string
  payingLabel: string
  pricePeriodLabel: string
  totalPriceTemplate: string
  discountLabel: (percent: number) => string
  dailyLimitTemplate: string
  solutionCountTemplate: string
  modesLabel: string
  autoRenewLabel: string
  oneTimeLabel: string
  paymentHint: string
  periodEveryMonth: string
  periodEveryMonths: (months: number) => string
  renewHintOn: string
  renewHintOff: string
  paymentNoLinkError: string
  paymentGenericError: string
}

export const shopText: Dictionary<ShopText> = {
  ru: {
    hero: {
      title: "Магазин решений",
      subtitle: "Выберите срок доступа. В чате Стратум сам выберет режим под вашу задачу.",
    },
    loadingText: "Загрузка тарифов...",
    emptyState: "Сейчас нет активных публичных решений. В админке включите тарифы, доступные для подписки.",
    tabsLabel: "Разделы магазина решений",
    closed: {
      title: "Магазин решений пока закрыт",
      subtitle: "Раздел временно скрыт администратором.",
    },
    nav: { chatLabel: "В чат", accessLabel: "Получить доступ" },
    periodsLabel: "Срок подписки",
    defaultGroupLabel: "Тарифы",
    buyLabel: "Купить решение",
    payingLabel: "Создаём платёж...",
    pricePeriodLabel: "в месяц",
    totalPriceTemplate: "Итого за {{months}} мес.: {{total}}",
    discountLabel: (percent) => `скидка ${percent}%`,
    dailyLimitTemplate: "До {{count}} сообщений в день",
    solutionCountTemplate: "Режимов внутри: {{count}}",
    modesLabel: "Режимы внутри",
    autoRenewLabel: "Продлевать автоматически: {{period}}",
    oneTimeLabel: "Разово, без продления",
    paymentHint: "Доступ сразу после оплаты.",
    periodEveryMonth: "каждый месяц",
    periodEveryMonths: (months) => `каждые ${months} мес.`,
    renewHintOn: "Можно отключить перед оплатой",
    renewHintOff: "Карта не сохранится для следующих списаний",
    paymentNoLinkError: "Платёж создан без ссылки на оплату. Напишите администратору.",
    paymentGenericError: "Не удалось создать платёж",
  },
  en: {
    hero: {
      title: "Shop",
      subtitle: "Choose your access period. Stratum will pick the right mode for your task in the chat.",
    },
    loadingText: "Loading plans...",
    emptyState: "There are no active public plans right now. Enable subscribable plans in the admin panel.",
    tabsLabel: "Shop sections",
    closed: {
      title: "The shop is currently closed",
      subtitle: "This section has been temporarily hidden by the administrator.",
    },
    nav: { chatLabel: "Chat", accessLabel: "Get access" },
    periodsLabel: "Subscription period",
    defaultGroupLabel: "Plans",
    buyLabel: "Buy plan",
    payingLabel: "Creating payment...",
    pricePeriodLabel: "per month",
    totalPriceTemplate: "Total for {{months}} mo.: {{total}}",
    discountLabel: (percent) => `discount ${percent}%`,
    dailyLimitTemplate: "Up to {{count}} messages per day",
    solutionCountTemplate: "Modes included: {{count}}",
    modesLabel: "Modes included",
    autoRenewLabel: "Auto-renew: {{period}}",
    oneTimeLabel: "One-time, no renewal",
    paymentHint: "Access starts right after payment.",
    periodEveryMonth: "every month",
    periodEveryMonths: (months) => `every ${months} mo.`,
    renewHintOn: "You can turn this off before paying",
    renewHintOff: "Your card won't be saved for future charges",
    paymentNoLinkError: "Payment created without a payment link. Please contact the administrator.",
    paymentGenericError: "Failed to create payment",
  },
}
