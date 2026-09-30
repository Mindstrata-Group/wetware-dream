import type { Dictionary } from "@/lib/i18n/types"

type AccessText = {
  nav: { login: string }
  suspenseLoading: string
  helpPrefix: string
  testAccessLinkLabel: string
  shopButtonLabel: string
  featureLinks: { blog: string; shop: string }
  errors: Record<string, string>
  promoServerError: string
  successRoleUpgraded: (code: string) => string
  successAccessGranted: (days: number | null) => string
  input: {
    title: string
    promoLabel: string
    placeholder: string
    submitBusy: string
    submitIdle: string
    loginLink: string
    homeLink: string
    privacyLink: string
  }
  loading: {
    checkingAccess: string
  }
  success: {
    roleUpgradedBadge: string
    accessGrantedBadge: string
    promoLabel: string
    roleLabel: string
    roleValue: string
    autoRedirectAdmin: string
    autoRedirectChat: string
    seconds: string
    accessPeriodLabel: string
    accessDaysValue: (days: number) => string
    accessPeriodFallback: string
    modesLabel: string
    modesEmptyFallback: string
    grantedLabel: string
    extendedLabel: string
    goAdmin: string
    goChat: string
    backHome: string
    untilPrefix: string
  }
}

export const accessText: Dictionary<AccessText> = {
  ru: {
    nav: { login: "← Войти" },
    suspenseLoading: "Загрузка…",
    helpPrefix: "Тестовый доступ можно запросить здесь:",
    testAccessLinkLabel: "написать в Telegram",
    shopButtonLabel: "Посмотреть готовые решения",
    featureLinks: { blog: "Блог", shop: "Магазин решений" },
    errors: {
      empty: "Введите промокод",
      not_found: "Такого промокода нет",
      not_started: "Промокод ещё не активен",
      expired: "Промокод истёк",
      limit_reached: "Лимит активаций исчерпан",
      already_used: "Вы уже использовали этот промокод",
      auth_required: "Сначала войдите через Яндекс, затем промокод применится к аккаунту",
      archived: "Промокод отключён",
      server_error: "Ошибка сервера при проверке промокода",
    },
    promoServerError: "Сервер промокодов не отвечает. Попробуйте ещё раз через несколько секунд.",
    successRoleUpgraded: (code) => `Промокод ${code} активирован. Аккаунту выдана роль администратора.`,
    successAccessGranted: (days) => `Доступ открыт на ${days !== null ? days : "указанное число"} дней. Сейчас перейдём в чат с нужным режимом.`,
    input: {
      title: "Введите промокод",
      promoLabel: "Промокод",
      placeholder: "XXXX-XXXX-XXXX",
      submitBusy: "Проверяем…",
      submitIdle: "Активировать",
      loginLink: "Войти →",
      homeLink: "На главную",
      privacyLink: "Политика конфиденциальности",
    },
    loading: {
      checkingAccess: "Проверяем текущий доступ…",
    },
    success: {
      roleUpgradedBadge: "Роль обновлена",
      accessGrantedBadge: "Доступ открыт",
      promoLabel: "Промокод",
      roleLabel: "Роль:",
      roleValue: "admin",
      autoRedirectAdmin: "Автопереход в админку через:",
      autoRedirectChat: "Автопереход в чат через:",
      seconds: "сек.",
      accessPeriodLabel: "Срок доступа:",
      accessDaysValue: (days) => `${days} дн.`,
      accessPeriodFallback: "согласно тарифу",
      modesLabel: "Режимы:",
      modesEmptyFallback: "Будут доступны в чате",
      grantedLabel: "Открыты впервые:",
      extendedLabel: "Продлены:",
      goAdmin: "Перейти в админку →",
      goChat: "Перейти в чат →",
      backHome: "Вернуться на главную",
      untilPrefix: "до",
    },
  },
  en: {
    nav: { login: "← Log in" },
    suspenseLoading: "Loading…",
    helpPrefix: "You can request test access here:",
    testAccessLinkLabel: "message us on Telegram",
    shopButtonLabel: "View ready-made solutions",
    featureLinks: { blog: "Blog", shop: "Shop" },
    errors: {
      empty: "Enter a promo code",
      not_found: "This promo code doesn't exist",
      not_started: "This promo code isn't active yet",
      expired: "This promo code has expired",
      limit_reached: "Activation limit reached",
      already_used: "You've already used this promo code",
      auth_required: "Log in with Yandex first, then the promo code will apply to your account",
      archived: "This promo code is disabled",
      server_error: "Server error while checking the promo code",
    },
    promoServerError: "The promo code server isn't responding. Please try again in a few seconds.",
    successRoleUpgraded: (code) => `Promo code ${code} activated. Your account was granted the administrator role.`,
    successAccessGranted: (days) => `Access granted for ${days !== null ? days : "the specified number of"} days. Redirecting you to the chat with the right mode.`,
    input: {
      title: "Enter promo code",
      promoLabel: "Promo code",
      placeholder: "XXXX-XXXX-XXXX",
      submitBusy: "Checking…",
      submitIdle: "Activate",
      loginLink: "Log in →",
      homeLink: "Home",
      privacyLink: "Privacy Policy",
    },
    loading: {
      checkingAccess: "Checking current access…",
    },
    success: {
      roleUpgradedBadge: "Role updated",
      accessGrantedBadge: "Access granted",
      promoLabel: "Promo code",
      roleLabel: "Role:",
      roleValue: "admin",
      autoRedirectAdmin: "Redirecting to admin in:",
      autoRedirectChat: "Redirecting to chat in:",
      seconds: "sec.",
      accessPeriodLabel: "Access period:",
      accessDaysValue: (days) => `${days} days`,
      accessPeriodFallback: "per plan",
      modesLabel: "Modes:",
      modesEmptyFallback: "Will be available in chat",
      grantedLabel: "Unlocked for the first time:",
      extendedLabel: "Extended:",
      goAdmin: "Go to admin →",
      goChat: "Go to chat →",
      backHome: "Back to home",
      untilPrefix: "until",
    },
  },
}
