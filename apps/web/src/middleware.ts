import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";
import { DEFAULT_LOCALE, EN_PREFIX, LOCALE_COOKIE, LOCALE_HEADER, type Locale } from "./lib/i18n/types";

// /profile requires a full auth session; /chat allows a guest cookie,
// because a regular promo code must quickly take a guest to the chat without login.
const PROTECTED = ["/chat", "/profile"];

const LOCALE_COOKIE_MAX_AGE = 60 * 60 * 24 * 365; // 1 year

// Files with static extensions (favicon.ico, /brand/*.png, /fonts/*.woff, etc.)
// are served as is, without localisation.
const STATIC_FILE = /\.[^/]+$/;

/**
 * Visitor geo/language for a first visit without a language choice cookie.
 * CF-IPCountry is set by Cloudflare if the proxy with IP Geolocation is enabled;
 * if it is absent, we rely on the browser's Accept-Language (more reliable than guessing
 * by IP without a geo database).
 */
function detectLocale(request: NextRequest): Locale {
  const cfCountry = request.headers.get("cf-ipcountry");
  if (cfCountry) return cfCountry.toUpperCase() === "RU" ? "ru" : "en";

  const acceptLanguage = request.headers.get("accept-language");
  if (!acceptLanguage) return DEFAULT_LOCALE;

  const primary = acceptLanguage.split(",")[0]?.trim().toLowerCase() ?? "";
  if (!primary) return DEFAULT_LOCALE;
  return primary.startsWith("ru") ? "ru" : "en";
}

function withLocaleCookie(response: NextResponse, locale: Locale): NextResponse {
  response.cookies.set(LOCALE_COOKIE, locale, {
    path: "/",
    maxAge: LOCALE_COOKIE_MAX_AGE,
    sameSite: "lax",
  });
  return response;
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (STATIC_FILE.test(pathname)) return NextResponse.next();

  const hasEnPrefix = pathname === EN_PREFIX || pathname.startsWith(`${EN_PREFIX}/`);
  const resolvedPathname = hasEnPrefix ? pathname.slice(EN_PREFIX.length) || "/" : pathname;

  const cookieLocale = request.cookies.get(LOCALE_COOKIE)?.value;
  let locale: Locale;
  let redirectToEn = false;

  if (hasEnPrefix) {
    locale = "en";
  } else if (cookieLocale === "ru") {
    locale = "ru";
  } else if (cookieLocale === "en") {
    locale = "en";
    redirectToEn = true;
  } else {
    // No explicit language choice -> RU by default, no geo auto-redirect
    locale = DEFAULT_LOCALE;
    redirectToEn = false;
  }

  // The auth guard works with the "logical" path without the /en prefix.
  const isProtected = PROTECTED.some((p) => resolvedPathname === p || resolvedPathname.startsWith(`${p}/`));
  if (isProtected) {
    const session = request.cookies.get("mindstrata_session");
    const guest = request.cookies.get("mindstrata_guest_user_id");
    const canEnter = session?.value || (resolvedPathname === "/chat" && guest?.value);
    if (!canEnter) {
      const loginUrl = request.nextUrl.clone();
      loginUrl.pathname = locale === "en" ? `${EN_PREFIX}/login` : "/login";
      loginUrl.search = "";
      return withLocaleCookie(NextResponse.redirect(loginUrl), locale);
    }
  }

  if (redirectToEn) {
    const target = request.nextUrl.clone();
    target.pathname = EN_PREFIX + resolvedPathname;
    return withLocaleCookie(NextResponse.redirect(target), locale);
  }

  const requestHeaders = new Headers(request.headers);
  requestHeaders.set(LOCALE_HEADER, locale);

  let response: NextResponse;
  if (hasEnPrefix) {
    const target = request.nextUrl.clone();
    target.pathname = resolvedPathname;
    response = NextResponse.rewrite(target, { request: { headers: requestHeaders } });
  } else {
    response = NextResponse.next({ request: { headers: requestHeaders } });
  }

  return withLocaleCookie(response, locale);
}

export const config = {
  matcher: ["/((?!_next/|api/|health|webhooks/).*)"],
};
