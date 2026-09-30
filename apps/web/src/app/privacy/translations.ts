import type { Dictionary } from "@/lib/i18n/types"

type DocTileText = { title: string; subtitle?: string; hint: string; badge?: string }

type PrivacyText = {
  updated: string
  shell: {
    backToDocs: string
    home: string
    brandPrefix: string
    lastUpdated: string
    footerBackToDocs: string
  }
  hub: {
    title: string
    subtitle: string
    navHome: string
    navRegister: string
    openLabel: string
    footerHome: string
  }
  docs: {
    terms: DocTileText
    personalData: DocTileText
    consent: DocTileText
    cookies: DocTileText
    payments: DocTileText
    contacts: DocTileText
    withdraw: DocTileText
  }
}

export const privacyText: Dictionary<PrivacyText> = {
  ru: {
    updated: "1 мая 2026 г.",
    shell: {
      backToDocs: "← Документы",
      home: "Главная",
      brandPrefix: "Mindstrata · ",
      lastUpdated: "Последнее обновление: ",
      footerBackToDocs: "← К документам",
    },
    hub: {
      title: "Документы сервиса",
      subtitle: "Полный список юридических документов Mindstrata. Согласия — отдельными документами, как требует закон.",
      navHome: "← Главная",
      navRegister: "Регистрация",
      openLabel: "Открыть →",
      footerHome: "← На главную",
    },
    docs: {
      terms: { title: "Пользовательское соглашение / Оферта", hint: "Условия использования сервиса, права и обязанности сторон." },
      personalData: { title: "Политика обработки персональных данных", subtitle: "152-ФЗ", hint: "Какие данные собираем, для чего и как храним." },
      consent: { title: "Согласие на обработку персональных данных", subtitle: "отдельный документ · 152-ФЗ", hint: "Отдельный документ, который вы даёте при регистрации.", badge: "152-ФЗ" },
      cookies: { title: "Политика cookies", hint: "Технические cookies работают всегда; аналитики у нас нет." },
      payments: { title: "Правила оплаты, подписки и возврата", hint: "Тарифы, продление, отмена и возврат средств." },
      contacts: { title: "Реквизиты и контакты", hint: "Оператор, ИНН, адрес для обращений." },
      withdraw: { title: "Отозвать согласие / удалить данные", hint: "Как прекратить обработку и удалить аккаунт." },
    },
  },
  en: {
    updated: "May 1, 2026",
    shell: {
      backToDocs: "← Documents",
      home: "Home",
      brandPrefix: "Mindstrata · ",
      lastUpdated: "Last updated: ",
      footerBackToDocs: "← Back to documents",
    },
    hub: {
      title: "Service documents",
      subtitle: "Full list of Mindstrata's legal documents. Consents are provided as separate documents, as required by law.",
      navHome: "← Home",
      navRegister: "Sign up",
      openLabel: "Open →",
      footerHome: "← Back to home",
    },
    docs: {
      terms: { title: "Terms of Use / Public Offer", hint: "Terms of use, rights and obligations of the parties." },
      personalData: { title: "Personal Data Processing Policy", subtitle: "Federal Law 152-FZ", hint: "What data we collect, why, and how we store it." },
      consent: { title: "Consent to Personal Data Processing", subtitle: "separate document · Federal Law 152-FZ", hint: "A separate document you agree to during registration.", badge: "152-FZ" },
      cookies: { title: "Cookie Policy", hint: "Technical cookies always run; we have no analytics." },
      payments: { title: "Payment, Subscription & Refund Rules", hint: "Plans, renewal, cancellation, and refunds." },
      contacts: { title: "Company Details & Contacts", hint: "Operator details, tax ID, contact address." },
      withdraw: { title: "Withdraw Consent / Delete Data", hint: "How to stop processing and delete your account." },
    },
  },
}
