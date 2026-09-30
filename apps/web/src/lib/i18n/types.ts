export type Locale = "ru" | "en"

export const DEFAULT_LOCALE: Locale = "ru"

// The cookie and header names must match middleware.ts.
export const LOCALE_COOKIE = "mindstrata_locale"
export const LOCALE_HEADER = "x-ms-locale"
export const EN_PREFIX = "/en"

export type Dictionary<T> = Record<Locale, T>
