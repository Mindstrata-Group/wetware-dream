// EN/RU dictionary for the landing page. For locale=ru the default texts come
// from the CMS (site_content) with RU fallbacks in page.tsx; this dictionary is used
// only for static UI labels (buttons, badges).
// For locale=en the CMS is not used for the hero: the hero text comes from here.
// Demo conversations for both locales are loaded live from /api/public/demo-modes.

import type { Dictionary } from "@/lib/i18n/types"
import { OPERATOR_FOOTER_EN, OPERATOR_FOOTER_RU } from "@/lib/operator"

type HomeText = {
  logout: string
  registerShort: string
  registerFull: string
  demoBadge: string
  dirigentName: string
  dirigentBanner: (modeName: string) => string
  entry: {
    login: string
    profile: string
    chat: string
    access: string
  }
  hero: {
    title: string
    subtitle: string
    ctaLabel: string
    ctaPrimaryLabel: string
    legalLinkLabel: string
  }
  footerCopyright: string
  featureLinks: {
    blog: string
    shop: string
  }
}

export const homeText: Dictionary<HomeText> = {
  ru: {
    logout: "Выйти",
    registerShort: "Регистрация",
    registerFull: "Зарегистрироваться",
    demoBadge: "демо",
    dirigentName: "Стратум",
    dirigentBanner: (modeName) => `похоже, здесь нужен режим «${modeName}». Переключаю и продолжаю разбор.`,
    entry: {
      login: "Войти",
      profile: "В профиль",
      chat: "Войти в чат",
      access: "Получить доступ",
    },
    hero: {
      title: "Опишите задачу — Стратум сам выберет режим",
      subtitle: "Внутри {{modes_count}} режимов для текста, решений, переговоров и сложных ситуаций. Пишите обычным языком: Стратум поймёт задачу, переключит режим и продолжит разбор.",
      ctaLabel: "Посмотреть, как это работает",
      ctaPrimaryLabel: "Разобрать свою задачу",
      legalLinkLabel: "Правовая информация",
    },
    footerCopyright: OPERATOR_FOOTER_RU,
    featureLinks: {
      blog: "Блог",
      shop: "Магазин решений",
    },
  },
  en: {
    logout: "Log out",
    registerShort: "Sign up",
    registerFull: "Create account",
    demoBadge: "demo",
    dirigentName: "Stratum",
    dirigentBanner: (modeName) => `looks like this needs the "${modeName}" mode. Switching and continuing the analysis.`,
    entry: {
      login: "Log in",
      profile: "Profile",
      chat: "Open chat",
      access: "Get access",
    },
    hero: {
      title: "Describe your task — Stratum picks the right mode",
      subtitle: "There are {{modes_count}} modes inside for writing, decisions, negotiations and tricky situations. Just describe it in plain language — Stratum will understand the task, switch to the right mode and continue the analysis.",
      ctaLabel: "See how it works",
      ctaPrimaryLabel: "Work through your task",
      legalLinkLabel: "Legal information",
    },
    footerCopyright: OPERATOR_FOOTER_EN,
    featureLinks: {
      blog: "Blog",
      shop: "Shop",
    },
  },
}
