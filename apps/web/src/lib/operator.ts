// Legal details of the service operator: who is responsible for the site and
// where to write.
//
// FILL THESE IN BEFORE OPENING THE SITE TO PEOPLE. The values go into the
// public offer, the privacy policy, the footer and the admin panel. With the
// placeholders the site works, but the legal pages speak for an unnamed
// "Operator".

export const OPERATOR = {
  brand: "Mindstrata",
  // Status and name exactly as written in the offer and the privacy policy.
  statusRu: "самозанятый",
  statusEn: "self-employed individual",
  nameRu: "Фамилия Имя Отчество",
  nameRuGenitive: "Фамилии Имени Отчества",
  nameRuShort: "Фамилия И.О.",
  nameEn: "Name Surname",
  nameEnShort: "N. Surname",
  inn: "000000000000",
  email: "privacy@example.com",
  siteUrl: "https://example.com",
  siteLabel: "example.com",
  // Telegram contact used on the landing page and the access page.
  telegramHandle: "your_support_bot",
} as const;

export const OPERATOR_FOOTER_RU = `© 2026 ${OPERATOR.brand} · ${OPERATOR.nameRuShort} · ИНН ${OPERATOR.inn}`;
export const OPERATOR_FOOTER_EN = `© 2026 ${OPERATOR.brand} · ${OPERATOR.nameEnShort} · INN ${OPERATOR.inn}`;
export const OPERATOR_TELEGRAM_URL = `https://t.me/${OPERATOR.telegramHandle}`;
