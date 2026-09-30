"use client";

import { Suspense, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { BrandLogo } from "@/components/BrandLogo";
import { useSiteContent } from "@/lib/useSiteContent";
import {
  getStorefrontFeatureLinks,
  isFeatureEnabled,
  getString,
} from "@/lib/siteContent";
import { useLocale, useT } from "@/lib/i18n/LocaleProvider";
import { accessText } from "./translations";

// SOLID-split 2026-05-31: theme/types/utils + DotsBackground moved out.
import { MS, FONT_HEAD, FONT_BODY, FONT_MONO } from "./_parts/_theme";
import type {
  AccessStatusResponse,
  ApplyResponse,
  PromoInfo,
} from "./_parts/_types";
import {
  buildPromoSourceLink,
  normalizePromoModes,
} from "./_parts/_utils";
import { DotsBackground } from "./_parts/DotsBackground";
import { InputView } from "./_views/InputView";
import { LoadingView } from "./_views/LoadingView";
import { SuccessView } from "./_views/SuccessView";
import { OPERATOR_TELEGRAM_URL } from "@/lib/operator";
import { publicApiBase } from "@/lib/apiBase";

// ── Config ─────────────────────────────────────────────────────────────────────
const TEST_ACCESS_MESSAGE =
  "Добрый день! Вышлите пожалуйста мне тестовый доступ к Стратуму!";


// ── API helpers (preserved from working version) ──────────────────────────────
async function fetchWithApiFallback(
  input:
    | "/api/access/status"
    | "/api/access/promocode/apply"
    | "/api/promo/validate",
  init?: RequestInit,
) {
  const primary = await fetch(input, init);
  if (primary.status < 500) return primary;
  return fetch(`${publicApiBase}${input}`, init);
}

async function fetchWithTimeout(
  input: string,
  init: RequestInit,
  timeoutMs = 12000,
) {
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(input, { ...init, signal: controller.signal });
  } finally {
    window.clearTimeout(timer);
  }
}

async function applyPromoWithFallback(code: string) {
  const payload = JSON.stringify({ code });
  const init: RequestInit = {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: payload,
  };

  // Order matches the previously working file: hit the validator first,
  // then the canonical apply endpoint, then absolute fallbacks for dev/proxy issues.
  const targets = [
    "/api/promo/validate",
    "/api/access/promocode/apply",
    `${publicApiBase}/api/promo/validate`,
    `${publicApiBase}/api/access/promocode/apply`,
  ] as const;

  let lastResponse: Response | null = null;

  for (const target of targets) {
    try {
      const res = await fetchWithTimeout(target, init);
      lastResponse = res;
      if (res.ok || res.status < 500) return res;
    } catch {
      // try next target
    }
  }

  if (lastResponse) return lastResponse;
  throw new Error("promo_request_failed");
}

// (removed: _parts/_utils (helpers))

// (removed: _parts/_utils)

// (removed: _parts/_utils)

// (removed: _parts/_utils (errorMap))

// (removed: _parts/_types)

// ── Decorative dot background (replaces <DotsBackground/> from mockup) ────────
// (removed: _parts/DotsBackground)

// ── Content ────────────────────────────────────────────────────────────────────
function AccessPageContent() {
  const router = useRouter();
  const locale = useLocale();
  const t = useT(accessText);
  const searchParams = useSearchParams();
  const forcePromo = searchParams?.get("force") === "promo";
  const promoFromLink = searchParams?.get("promo") || "";

  // The content of the "Get a promo code" block is edited from the admin panel (CMS).
  // Falls back to env variables -> defaults if the CMS does not return the key.
  const content = useSiteContent();
  const helpPrefix = locale === "en" ? t.helpPrefix : getString(
    content,
    "access.promo.help_prefix",
    "Тестовый доступ можно запросить здесь:",
  );
  const testAccessBaseUrl = getString(
    content,
    "access.promo.help_link_href",
    process.env.NEXT_PUBLIC_TEST_ACCESS_URL || OPERATOR_TELEGRAM_URL,
  );
  const testAccessLinkLabel = locale === "en" ? t.testAccessLinkLabel : getString(
    content,
    "access.promo.help_link_label",
    process.env.NEXT_PUBLIC_TEST_ACCESS_LABEL || "написать в Telegram",
  );
  const shopEnabled = isFeatureEnabled(content, "shop");
  const featureLinksRaw = getStorefrontFeatureLinks(content);
  const featureLinks = locale === "en"
    ? featureLinksRaw.map((link) => ({ ...link, label: link.feature === "blog" ? t.featureLinks.blog : t.featureLinks.shop }))
    : featureLinksRaw;
  const accessShopButtonLabel = locale === "en" ? t.shopButtonLabel : getString(
    content,
    "access.shop_button_label",
    "Посмотреть готовые решения",
  );
  const testAccessUrl = buildPromoSourceLink(
    testAccessBaseUrl,
    TEST_ACCESS_MESSAGE,
  );

  const [code, setCode] = useState("");
  const [checking, setChecking] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [successMessage, setSuccessMessage] = useState("");
  const [promoInfo, setPromoInfo] = useState<PromoInfo | null>(null);
  const [countdown, setCountdown] = useState(10);
  const [autoApplyPending, setAutoApplyPending] = useState(false);

  // 1. Pre-fill from URL / pending localStorage promocode
  useEffect(() => {
    const linkPromo = promoFromLink.trim();
    const pending = window.localStorage.getItem("mindstrata_pending_promo");
    const nextCode = (linkPromo || pending || "").toUpperCase();
    if (nextCode) {
      setCode(nextCode);
      setAutoApplyPending(true);
    }
    if (pending) window.localStorage.removeItem("mindstrata_pending_promo");
  }, [promoFromLink]);

  // 2. Skip the form if the user already has access
  useEffect(() => {
    let ignore = false;
    async function checkAccess() {
      if (forcePromo || promoFromLink) {
        setChecking(false);
        return;
      }
      try {
        const res = await fetchWithApiFallback("/api/access/status", {
          credentials: "include",
          cache: "no-store",
        });
        const json = (await res
          .json()
          .catch(() => ({}))) as AccessStatusResponse;
        if (res.ok && json.ok && json.hasAccess) {
          router.replace("/chat");
          return;
        }
      } catch {
        // stay on form
      }
      if (!ignore) setChecking(false);
    }
    void checkAccess();
    return () => {
      ignore = true;
    };
  }, [router, forcePromo, promoFromLink]);

  const applyCurrentCode = useCallback(
    async (promoCode: string) => {
      setSubmitting(true);
      setError("");
      setSuccessMessage("");
      setPromoInfo(null);
      setCountdown(10);

      try {
        const normalizedCode = promoCode.toUpperCase().trim();
        if (!normalizedCode) {
          setError(t.errors.empty);
          return;
        }
        const res = await applyPromoWithFallback(normalizedCode);
        const json = (await res.json().catch(() => ({}))) as ApplyResponse;

        if (!res.ok || !json.ok) {
          const codeKey = String(json.code || json.errorCode || "server_error");
          if (json.requiresAuth || codeKey === "auth_required") {
            window.localStorage.setItem(
              "mindstrata_pending_promo",
              normalizedCode,
            );
            router.push(
              `/register?next=${encodeURIComponent("/access?force=promo")}`,
            );
            return;
          }
          setError(t.errors[codeKey] || json.error || t.errors.server_error);
          return;
        }

        const modes = normalizePromoModes(json.modes);
        const grantedModes = normalizePromoModes(json.grantedModes);
        const extendedModes = normalizePromoModes(json.extendedModes);
        const promoModeIds = modes
          .map((m) => m.modeId)
          .filter((id): id is number => typeof id === "number");
        if (promoModeIds.length) {
          window.localStorage.setItem(
            "ms_chat_selected_mode_ids",
            JSON.stringify([...new Set(promoModeIds)]),
          );
        }

        const finalCode = String(json.code || normalizedCode).toUpperCase();
        setPromoInfo({
          code: finalCode,
          accessDays:
            typeof json.accessDays === "number" ? json.accessDays : undefined,
          modes,
          grantedModes,
          extendedModes,
          modeIds: [...new Set(promoModeIds)],
          roleUpgraded: Boolean(json.roleUpgraded),
        });
        setSuccessMessage(
          json.roleUpgraded
            ? t.successRoleUpgraded(finalCode)
            : t.successAccessGranted(
                typeof json.accessDays === "number" ? json.accessDays : null,
              ),
        );
      } catch {
        setError(t.promoServerError);
      } finally {
        setSubmitting(false);
        setAutoApplyPending(false);
      }
    },
    [router, t],
  );

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await applyCurrentCode(code);
  }

  // 3. Auto-apply preloaded code once status check is done
  useEffect(() => {
    if (!checking && autoApplyPending && code) {
      void applyCurrentCode(code);
    }
  }, [checking, autoApplyPending, code, applyCurrentCode]);

  const promoChatHref = useCallback(() => {
    if (!promoInfo || promoInfo.roleUpgraded) return "/admin";
    const ids = promoInfo.modeIds.filter((id) => Number.isFinite(id));
    return ids.length ? `/chat?modes=${ids.join(",")}` : "/chat";
  }, [promoInfo]);

  // 4. Countdown auto-redirect on success
  useEffect(() => {
    if (!promoInfo) return;
    if (countdown <= 0) {
      router.replace(promoChatHref());
      return;
    }
    const timer = window.setTimeout(
      () => setCountdown((prev) => prev - 1),
      1000,
    );
    return () => window.clearTimeout(timer);
  }, [promoInfo, countdown, router, promoChatHref]);

  // ── Shared shell pieces ────────────────────────────────────────────────────
  const header = (
    <header
      style={{
        position: "sticky",
        top: 0,
        zIndex: 20,
        minHeight: 64,
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 8,
        flexWrap: "wrap",
        padding: "10px 32px",
        background: "rgba(var(--card-rgb), 0.88)",
        backdropFilter: "blur(12px)",
        borderBottom: `1px solid ${MS.ink10}`,
      }}
    >
      <BrandLogo priority />
      <nav
        style={{
          display: "flex",
          gap: 8,
          alignItems: "center",
          flexWrap: "wrap",
          justifyContent: "flex-end",
        }}
      >
        {featureLinks.map((link) => (
          <Link
            key={link.href}
            href={link.href}
            style={{
              display: "inline-flex",
              alignItems: "center",
              height: 36,
              padding: "0 12px",
              borderRadius: 10,
              border: `1px solid ${MS.ink20}`,
              background: "rgba(var(--card-rgb), 0.88)",
              color: MS.ink,
              fontSize: 13,
              textDecoration: "none",
            }}
          >
            {link.label}
          </Link>
        ))}
        <Link
          href="/login"
          style={{
            display: "inline-flex",
            alignItems: "center",
            height: 36,
            padding: "0 14px",
            borderRadius: 10,
            border: `1px solid ${MS.ink20}`,
            background: "rgba(var(--card-rgb), 0.88)",
            color: MS.ink,
            fontSize: 13,
            textDecoration: "none",
          }}
        >
          {t.nav.login}
        </Link>
      </nav>
    </header>
  );

  const pageWrapper = (children: React.ReactNode, dotOpacity = 0.4) => (
    <div
      style={{
        minHeight: "100vh",
        background: MS.bg,
        fontFamily: FONT_BODY,
        color: MS.ink,
        position: "relative",
        display: "flex",
        flexDirection: "column",
      }}
    >
      <DotsBackground opacity={dotOpacity} />
      {header}
      {children}
    </div>
  );

  // ── LOADING state ──────────────────────────────────────────────────────────
  if (checking) {
    return pageWrapper(<LoadingView />);
  }

  // ── SUCCESS state ──────────────────────────────────────────────────────────
  if (promoInfo) {
    return pageWrapper(
      <SuccessView
        promoInfo={promoInfo}
        successMessage={successMessage}
        countdown={countdown}
        goHref={promoChatHref()}
        locale={locale}
      />,
      0.35,
    );
  }

  // ── ENTRY state ────────────────────────────────────────────────────────────
  const busy = submitting || autoApplyPending;

  return pageWrapper(
    <InputView
      helpPrefix={helpPrefix}
      testAccessUrl={testAccessUrl}
      testAccessLinkLabel={testAccessLinkLabel}
      shopLink={
        shopEnabled ? { href: "/shop", label: accessShopButtonLabel } : null
      }
      code={code}
      setCode={setCode}
      error={error}
      setError={setError}
      busy={busy}
      submit={submit}
    />,
  );
}

export default function AccessPage() {
  const t = useT(accessText);
  return (
    <Suspense
      fallback={
        <div
          style={{
            minHeight: "100vh",
            background: MS.bg,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: MS.ink50,
            fontFamily: FONT_BODY,
          }}
        >
          {t.suspenseLoading}
        </div>
      }
    >
      <AccessPageContent />
    </Suspense>
  );
}
