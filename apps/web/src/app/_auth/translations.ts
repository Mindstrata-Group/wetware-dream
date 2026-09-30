import type { Dictionary } from "@/lib/i18n/types"

type AuthText = {
  nav: {
    home: string
    register: string
    login: string
  }
  login: {
    title: string
    subtitle: string
    yandexButton: string
    retryAccount: string
    registerAccount: string
    registerLink: string
    promoLink: string
    loading: string
  }
  register: {
    title: string
    subtitle: string
    yandexButton: string
    agreeError: string
    agreePrefix: string
    privacyLink: string
    agreeSuffix: string
    promoLink: string
    haveAccount: string
    loginLink: string
  }
  oauthErrors: {
    oauth_not_configured: string
    oauth: string
    oauth_state: string
    oauth_expired: string
    oauth_profile: string
    oauth_account: string
    guest_access: string
    session: string
    default: string
  }
}

export const authText: Dictionary<AuthText> = {
  ru: {
    nav: {
      home: "← Главная",
      register: "Регистрация",
      login: "Войти",
    },
    login: {
      title: "Вход",
      subtitle: "Войдите через Яндекс — займёт пару секунд.",
      yandexButton: "Войти с Яндекс ID",
      retryAccount: "Войти другим аккаунтом →",
      registerAccount: "Зарегистрироваться с Яндекс ID →",
      registerLink: "Регистрация →",
      promoLink: "Ввести промокод",
      loading: "Загрузка…",
    },
    register: {
      title: "Регистрация",
      subtitle: "Создайте аккаунт через Яндекс — это займёт несколько секунд.",
      yandexButton: "Зарегистрироваться с Яндекс ID",
      agreeError: "Примите политику конфиденциальности, чтобы продолжить.",
      agreePrefix: "Поставьте галочку, чтобы принять",
      privacyLink: "политику конфиденциальности",
      agreeSuffix: "и условия использования сервиса",
      promoLink: "Ввести промокод",
      haveAccount: "Уже есть аккаунт?",
      loginLink: "Войти",
    },
    oauthErrors: {
      oauth_not_configured: "Социальный вход пока не настроен на сервере.",
      oauth: "Социальный вход не завершён.",
      oauth_state: "Сессия входа устарела. Попробуйте ещё раз.",
      oauth_expired: "Сессия входа истекла. Попробуйте ещё раз.",
      oauth_profile: "Не удалось получить профиль из соцсети.",
      oauth_account: "Этот Яндекс ID ещё не зарегистрирован в Стратуме. Зарегистрируйтесь или выберите Яндекс-аккаунт, который уже привязан к вашему профилю.",
      guest_access: "Вход найден, но гостевой промокод и историю не удалось перенести в аккаунт. Попробуйте войти ещё раз; если повторится — напишите в поддержку.",
      session: "Не удалось создать сессию входа.",
      default: "Ошибка социального входа.",
    },
  },
  en: {
    nav: {
      home: "← Home",
      register: "Sign up",
      login: "Log in",
    },
    login: {
      title: "Log in",
      subtitle: "Sign in with Yandex — takes a couple of seconds.",
      yandexButton: "Log in with Yandex ID",
      retryAccount: "Log in with a different account →",
      registerAccount: "Sign up with Yandex ID →",
      registerLink: "Sign up →",
      promoLink: "Enter promo code",
      loading: "Loading…",
    },
    register: {
      title: "Sign up",
      subtitle: "Create an account with Yandex — it only takes a few seconds.",
      yandexButton: "Sign up with Yandex ID",
      agreeError: "Accept the privacy policy to continue.",
      agreePrefix: "Check the box to accept the",
      privacyLink: "privacy policy",
      agreeSuffix: "and the service terms of use",
      promoLink: "Enter promo code",
      haveAccount: "Already have an account?",
      loginLink: "Log in",
    },
    oauthErrors: {
      oauth_not_configured: "Social login is not configured on the server yet.",
      oauth: "Social login was not completed.",
      oauth_state: "The login session has expired. Please try again.",
      oauth_expired: "The login session has expired. Please try again.",
      oauth_profile: "Could not retrieve the profile from the social network.",
      oauth_account: "This Yandex ID is not registered in Stratum yet. Sign up or choose the Yandex account already linked to your profile.",
      guest_access: "Sign-in succeeded, but guest promo access and history could not be moved to the account. Try signing in again; contact support if it repeats.",
      session: "Could not create a login session.",
      default: "Social login error.",
    },
  },
}
