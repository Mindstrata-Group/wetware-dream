import { headers } from "next/headers"
import { DEFAULT_LOCALE, LOCALE_HEADER, type Dictionary, type Locale } from "./types"

/**
 * On the server the locale is set by middleware.ts in the x-ms-locale header
 * (after the rewrite /en/* -> the original path without the prefix).
 */
export async function getLocale(): Promise<Locale> {
  const value = (await headers()).get(LOCALE_HEADER)
  return value === "en" ? "en" : DEFAULT_LOCALE
}

export async function getDict<T>(dict: Dictionary<T>): Promise<T> {
  return dict[await getLocale()]
}
