// EN/RU dictionary for the /about page. For locale=ru the default texts
// come from the CMS (site_content) with RU fallbacks in page.tsx; this dictionary
// is used only for static UI labels (badge, buttons, footer).
// For locale=en the CMS is not used: all text comes from here.

import type { Dictionary } from "@/lib/i18n/types"
import type { AboutTile, Comparison, Feature } from "./_parts/_content"
import { COMPARISONS, FEATURES, FALLBACK_TILES } from "./_parts/_content"
import { OPERATOR_FOOTER_EN, OPERATOR_FOOTER_RU } from "@/lib/operator"

type AboutText = {
  badge: string
  introTitle: string
  introSubtitle: string
  stats: {
    modesLabel: string
    expertsValue: string
    expertsLabel: string
  }
  comparisonTitle: string
  comparisonHeaders: {
    aspect: string
    regular: string
    stratum: string
  }
  comparisons: Comparison[]
  featuresTitle: string
  features: Feature[]
  howTitle: string
  tiles: AboutTile[]
  ctaTitle: string
  ctaSubtitle: string
  ctaButtons: {
    promo: string
    login: string
  }
  nav: {
    home: string
    tryIt: string
  }
  footer: {
    copyright: string
    links: {
      home: string
      privacy: string
      access: string
    }
  }
}

const tilesEn: AboutTile[] = [
  { id: 1, title: "Log in", body: "Via Yandex ID — in 5 seconds." },
  { id: 2, title: "Enter a promo code", body: "Get free access to the first mode." },
  { id: 3, title: "Describe your task", body: "No need to look for a prompt — the AI will understand what you need on its own." },
  { id: 4, title: "Get your answer", body: "One chat — multiple perspectives, pick the mode you need from the suggestions." },
]

const comparisonsEn: Comparison[] = [
  {
    aspect: "How the conversation starts",
    regular: "A blank page — you come up with the prompt yourself, set the tone, and provide the context",
    stratum: "The mode is already loaded: professional instructions, conversation structure and methodology are all set up in advance",
  },
  {
    aspect: "Who controls the response format",
    regular: "The neural network picks the style itself — sometimes an essay, sometimes a list, sometimes a conversation",
    stratum: "The format is set by the mode: an interview follows the 4D methodology, reframing follows the NLP structure, analysis follows a systems approach",
  },
  {
    aspect: "Depth of specialization",
    regular: "Knows a bit about everything — broad but unfocused",
    stratum: "Each mode is a separate expert with a narrow specialization and specific tools",
  },
  {
    aspect: "Switching context",
    regular: "You need to manually explain a task change or start a new chat",
    stratum: "The Conductor itself detects that a mode switch is needed and switches — you just write your task",
  },
  {
    aspect: "History and memory",
    regular: "Every new chat starts from scratch — context from past sessions isn't preserved",
    stratum: "The dialogue is saved, and the session ends with a summary and a practical next step",
  },
  {
    aspect: "Protection from hallucinations",
    regular: "Confidently answers even when it doesn't know — there's no system-level limiter",
    stratum: "An immutable guardrail in every mode: the neural network can't go beyond the task or reveal the system prompt",
  },
]

const featuresEn: Feature[] = [
  {
    icon: "🎛",
    title: "Thinking modes",
    body: "Each mode is a separate methodological tool: interview, systems analysis, reframing, meta-model and others.",
  },
  {
    icon: "🎼",
    title: "Conductor",
    body: "A separate orchestration system: analyzes the conversation context and automatically switches the mode when it sees a different tool is needed.",
  },
  {
    icon: "🛡",
    title: "Guardrail",
    body: "An immutable security block is added to every mode on each request — protection against prompt injection, instruction leaks, and unwanted responses.",
  },
  {
    icon: "📋",
    title: "Session summary",
    body: "At the end of each conversation the system generates a summary: what was covered, what the next step is — and saves it to history.",
  },
  {
    icon: "🔑",
    title: "Promo code access",
    body: "A flexible access system: paid modes unlock with a promo code or subscription. The basic mode is available to everyone.",
  },
  {
    icon: "📊",
    title: "Daily quota",
    body: "A fair limit system: see how many messages are left today, when the limit resets, and which modes are still available.",
  },
]

export const aboutText: Dictionary<AboutText> = {
  ru: {
    badge: "Чем мы отличаемся",
    introTitle: "Не просто нейросеть — платформа профессиональных режимов",
    introSubtitle: "Обычная нейросеть — универсальный инструмент без фокуса. СТРАТУМ — это набор специализированных методологий, которые сами выбирают подход под вашу задачу.",
    stats: {
      modesLabel: "режимов",
      expertsValue: "100%",
      expertsLabel: "инструкции от экспертов",
    },
    comparisonTitle: "Сравнение с обычной нейросетью",
    comparisonHeaders: {
      aspect: "Аспект",
      regular: "Обычная нейросеть",
      stratum: "СТРАТУМ",
    },
    comparisons: COMPARISONS,
    featuresTitle: "Что внутри",
    features: FEATURES,
    howTitle: "Как это работает",
    tiles: FALLBACK_TILES,
    ctaTitle: "Попробуйте прямо сейчас",
    ctaSubtitle: "Первый режим бесплатно — просто введите промокод или войдите через Яндекс.",
    ctaButtons: {
      promo: "Ввести промокод",
      login: "Войти",
    },
    nav: {
      home: "← На главную",
      tryIt: "Попробовать",
    },
    footer: {
      copyright: OPERATOR_FOOTER_RU,
      links: {
        home: "Главная",
        privacy: "Политика конфиденциальности",
        access: "Получить доступ",
      },
    },
  },
  en: {
    badge: "What makes us different",
    introTitle: "Not just a neural network — a platform of professional modes",
    introSubtitle: "A regular neural network is a general-purpose tool with no focus. STRATUM is a set of specialized methodologies that automatically pick the right approach for your task.",
    stats: {
      modesLabel: "modes",
      expertsValue: "100%",
      expertsLabel: "expert-written instructions",
    },
    comparisonTitle: "Comparison with a regular neural network",
    comparisonHeaders: {
      aspect: "Aspect",
      regular: "Regular neural network",
      stratum: "STRATUM",
    },
    comparisons: comparisonsEn,
    featuresTitle: "What's inside",
    features: featuresEn,
    howTitle: "How it works",
    tiles: tilesEn,
    ctaTitle: "Try it right now",
    ctaSubtitle: "The first mode is free — just enter a promo code or sign in with Yandex.",
    ctaButtons: {
      promo: "Enter promo code",
      login: "Log in",
    },
    nav: {
      home: "← Home",
      tryIt: "Try it",
    },
    footer: {
      copyright: OPERATOR_FOOTER_EN,
      links: {
        home: "Home",
        privacy: "Privacy Policy",
        access: "Get access",
      },
    },
  },
}
