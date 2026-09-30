"use client"

import { usePathname } from "next/navigation"
import { EN_PREFIX, LOCALE_COOKIE } from "@/lib/i18n/types"
import { useLocale } from "@/lib/i18n/LocaleProvider"

const COOKIE_MAX_AGE = 60 * 60 * 24 * 365
const LANGUAGE_SWITCHER_ENABLED = false

function withoutEnPrefix(pathname: string): string {
  if (pathname === EN_PREFIX) return "/"
  if (pathname.startsWith(`${EN_PREFIX}/`)) return pathname.slice(EN_PREFIX.length)
  return pathname
}

export function shouldShowLanguageSwitcher(pathname: string): boolean {
  if (!LANGUAGE_SWITCHER_ENABLED) return false
  const bare = withoutEnPrefix(pathname)
  return bare === "/" || bare === "/profile"
}

export function LanguageSwitcher() {
  const locale = useLocale()
  const pathname = usePathname() ?? "/"
  if (!shouldShowLanguageSwitcher(pathname)) return null

  const bare = withoutEnPrefix(pathname)
  const ruHref = bare
  const enHref = bare === "/" ? EN_PREFIX : `${EN_PREFIX}${bare}`

  function go(target: "ru" | "en", href: string) {
    document.cookie = `${LOCALE_COOKIE}=${target}; path=/; max-age=${COOKIE_MAX_AGE}; samesite=lax`
    window.location.href = href
  }

  return (
    <div
      style={{
        position: "fixed",
        bottom: 12,
        left: 12,
        zIndex: 1000,
        display: "flex",
        gap: 2,
        background: "var(--card)",
        border: "1px solid var(--line)",
        borderRadius: 999,
        padding: 2,
        boxShadow: "var(--shadow)",
        fontSize: 12,
        fontWeight: 600,
      }}
    >
      <button
        type="button"
        className="ms-button-ghost ms-button-xs"
        onClick={() => go("ru", ruHref)}
        aria-current={locale === "ru"}
        style={{
          border: "none",
          borderRadius: 999,
          padding: "4px 10px",
          cursor: "pointer",
          background: locale === "ru" ? "var(--accent)" : "transparent",
          color: locale === "ru" ? "var(--brand-on)" : "var(--muted)",
        }}
      >
        RU
      </button>
      <button
        type="button"
        className="ms-button-ghost ms-button-xs"
        onClick={() => go("en", enHref)}
        aria-current={locale === "en"}
        style={{
          border: "none",
          borderRadius: 999,
          padding: "4px 10px",
          cursor: "pointer",
          background: locale === "en" ? "var(--accent)" : "transparent",
          color: locale === "en" ? "var(--brand-on)" : "var(--muted)",
        }}
      >
        EN
      </button>
    </div>
  )
}
