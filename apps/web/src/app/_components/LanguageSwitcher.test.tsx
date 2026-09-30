import { cleanup, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { LocaleProvider } from "@/lib/i18n/LocaleProvider"
import { LanguageSwitcher, shouldShowLanguageSwitcher } from "./LanguageSwitcher"

let pathname = "/"
vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
}))

function renderSwitcher(locale: "ru" | "en" = "ru") {
  return render(
    <LocaleProvider locale={locale}>
      <LanguageSwitcher />
    </LocaleProvider>,
  )
}

describe("LanguageSwitcher", () => {
  beforeEach(() => {
    pathname = "/"
  })

  afterEach(() => {
    cleanup()
  })

  it("временно отключает RU/EN на всех маршрутах", () => {
    expect(shouldShowLanguageSwitcher("/")).toBe(false)
    expect(shouldShowLanguageSwitcher("/en")).toBe(false)
    expect(shouldShowLanguageSwitcher("/profile")).toBe(false)
    expect(shouldShowLanguageSwitcher("/en/profile")).toBe(false)
    expect(shouldShowLanguageSwitcher("/chat")).toBe(false)
    expect(shouldShowLanguageSwitcher("/en/chat")).toBe(false)
    expect(shouldShowLanguageSwitcher("/access")).toBe(false)
  })

  it("не рендерит плашку на /profile", () => {
    pathname = "/profile"
    renderSwitcher()

    expect(screen.queryByRole("button", { name: "RU" })).toBeNull()
    expect(screen.queryByRole("button", { name: "EN" })).toBeNull()
  })

  it("не рендерит плашку в чате", () => {
    pathname = "/chat"
    renderSwitcher()

    expect(screen.queryByRole("button", { name: "RU" })).toBeNull()
    expect(screen.queryByRole("button", { name: "EN" })).toBeNull()
  })
})
