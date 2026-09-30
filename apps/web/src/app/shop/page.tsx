"use client";

import { useEffect, useMemo, useState } from "react";
import { StorefrontHeader } from "@/components/StorefrontHeader";
import {
  fillVars,
  getArray,
  getBoolean,
  getString,
  type SiteContent,
} from "@/lib/siteContent";
import { useSiteContent } from "@/lib/useSiteContent";
import { useLocale, useT } from "@/lib/i18n/LocaleProvider";
import { shopText } from "./translations";
import { apiFetch } from "@/lib/api";
import {
  formatRub,
  loadPublicTariffs,
  type PublicTariff,
} from "@/lib/publicTariffs";
import {
  normalizeShopBenefits,
  normalizeShopPeriods,
  normalizeShopTabs,
  normalizeShopTariffGroups,
  type ShopBenefitContent,
  type ShopPeriodContent,
  type ShopTabContent,
  type ShopTariffGroupContent,
} from "@/lib/shopContent";

export default function ShopPage() {
  const content = useSiteContent();
  const locale = useLocale();
  const t = useT(shopText);
  const enabled = getBoolean(content, "features.shop_enabled", false);
  const title = locale === "en"
    ? t.hero.title
    : getString(content, "shop.hero.title", "Магазин решений");
  const subtitle = locale === "en"
    ? t.hero.subtitle
    : getString(
        content,
        "shop.hero.subtitle",
        "Выберите срок доступа. В чате Стратум сам выберет режим под вашу задачу.",
      );
  const loadingText = locale === "en"
    ? t.loadingText
    : getString(
        content,
        "shop.loading_text",
        "Загрузка тарифов...",
      );
  const emptyState = locale === "en"
    ? t.emptyState
    : getString(
        content,
        "shop.empty_state",
        "Сейчас нет активных публичных решений. В админке включите тарифы, доступные для подписки.",
      );
  const tablistLabel = locale === "en"
    ? t.tabsLabel
    : getString(
        content,
        "shop.tabs_label",
        "Разделы магазина решений",
      );
  const closedTitle = locale === "en"
    ? t.closed.title
    : getString(
        content,
        "shop.closed.title",
        "Магазин решений пока закрыт",
      );
  const closedSubtitle = locale === "en"
    ? t.closed.subtitle
    : getString(
        content,
        "shop.closed.subtitle",
        "Раздел временно скрыт администратором.",
      );
  const shopTabs = normalizeShopTabs(
    getArray<ShopTabContent>(content, "shop.tabs", []),
  );
  const groupContent = normalizeShopTariffGroups(
    getArray<ShopTariffGroupContent>(content, "shop.tariff_groups", []),
  );
  const periods = normalizeShopPeriods(
    getArray<ShopPeriodContent>(content, "shop.periods", []),
  );
  const benefits = normalizeShopBenefits(
    getArray<ShopBenefitContent>(content, "shop.benefits", []),
  );
  const [tariffs, setTariffs] = useState<PublicTariff[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeGroup, setActiveGroup] = useState("");
  const [activePeriodId, setActivePeriodId] = useState("");

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    loadPublicTariffs()
      .then((items) => {
        if (!cancelled) setTariffs(items);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled]);

  const tariffGroups = useMemo(() => {
    const out = new Map<string, PublicTariff[]>();
    for (const tariff of tariffs) {
      const key = tariff.groupName || t.defaultGroupLabel;
      out.set(key, [...(out.get(key) || []), tariff]);
    }
    return out;
  }, [tariffs, t.defaultGroupLabel]);

  const groupMeta = useMemo(
    () => new Map(groupContent.map((group) => [group.groupName, group])),
    [groupContent],
  );

  const visibleTabs = useMemo(() => {
    if (shopTabs.length > 0) return shopTabs;
    if (groupContent.length > 0) {
      return groupContent.map((group) => ({
        label: group.title || group.groupName,
        groupName: group.groupName,
        description: group.description,
        badge: group.badge,
        enabled: group.enabled,
      }));
    }
    return Array.from(tariffGroups.keys()).map((groupName) => ({
      label: groupName,
      groupName,
      description: groupMeta.get(groupName)?.description || "",
      badge: groupMeta.get(groupName)?.badge || "",
      enabled: true,
    }));
  }, [shopTabs, groupContent, tariffGroups, groupMeta]);

  useEffect(() => {
    if (visibleTabs.length === 0) {
      setActiveGroup("");
      return;
    }
    if (!visibleTabs.some((tab) => tab.groupName === activeGroup)) {
      setActiveGroup(visibleTabs[0].groupName);
    }
  }, [activeGroup, visibleTabs]);

  const selectedTab =
    visibleTabs.find((tab) => tab.groupName === activeGroup) || visibleTabs[0];
  const visibleTariffs =
    selectedTab?.groupName === "*"
      ? tariffs
      : selectedTab
        ? tariffGroups.get(selectedTab.groupName) || []
        : [];
  const selectedPeriod =
    periods.find((period) => period.id === activePeriodId) || periods[0];

  useEffect(() => {
    if (periods.length === 0) {
      setActivePeriodId("");
      return;
    }
    if (!periods.some((period) => period.id === activePeriodId)) {
      setActivePeriodId(periods[0].id);
    }
  }, [activePeriodId, periods]);

  if (!enabled)
    return <ClosedSection title={closedTitle} subtitle={closedSubtitle} />;

  return (
    <main
      style={{
        minHeight: "100dvh",
        background: "var(--background)",
        color: "var(--foreground)",
        fontFamily: "var(--font-golos)",
      }}
    >
      <StorefrontHeader
        active="shop"
        chatLabel={locale === "en" ? t.nav.chatLabel : getString(content, "shop.nav.chat_label", "В чат")}
        accessLabel={locale === "en"
          ? t.nav.accessLabel
          : getString(
              content,
              "shop.nav.login_label",
              "Получить доступ",
            )}
      />
      <section
        style={{ maxWidth: 1120, margin: "0 auto", padding: "48px 24px 72px" }}
      >
        <div style={{ maxWidth: 780 }}>
          <h1
            style={{
              margin: 0,
              fontFamily: "var(--font-unbounded)",
              fontSize: "clamp(28px, 5vw, 48px)",
              lineHeight: 1.08,
              fontWeight: 600,
              letterSpacing: 0,
            }}
          >
            {title}
          </h1>
          <p
            style={{
              margin: "14px 0 0",
              color: "var(--muted)",
              fontSize: 17,
              lineHeight: 1.55,
            }}
          >
            {subtitle}
          </p>
        </div>
        {periods.length > 0 && (
          <div
            aria-label={locale === "en"
              ? t.periodsLabel
              : getString(
                  content,
                  "shop.periods_label",
                  "Срок подписки",
                )}
            style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 26 }}
          >
            {periods.map((period) => (
              <button
                key={period.id}
                type="button"
                aria-pressed={selectedPeriod?.id === period.id}
                onClick={() => setActivePeriodId(period.id)}
                style={{
                  minHeight: 40,
                  padding: "6px 14px",
                  borderRadius: 8,
                  border: `1px solid ${selectedPeriod?.id === period.id ? "var(--accent)" : "var(--line)"}`,
                  background:
                    selectedPeriod?.id === period.id
                      ? "color-mix(in oklab, var(--accent) 12%, var(--card))"
                      : "var(--card)",
                  color:
                    selectedPeriod?.id === period.id
                      ? "var(--accent-strong)"
                      : "var(--foreground)",
                  fontFamily: "inherit",
                  fontWeight: 700,
                  cursor: "pointer",
                }}
              >
                <span>{period.label}</span>
                {period.badge && (
                  <span
                    style={{
                      display: "block",
                      marginTop: 2,
                      color: "var(--muted)",
                      fontSize: 11,
                      fontWeight: 500,
                    }}
                  >
                    {period.badge}
                  </span>
                )}
              </button>
            ))}
          </div>
        )}

        {loading && (
          <p style={{ color: "var(--muted)", marginTop: 28 }}>{loadingText}</p>
        )}

        {!loading && tariffs.length === 0 && (
          <div
            style={{
              marginTop: 28,
              padding: 18,
              border: "1px solid var(--line)",
              borderRadius: 8,
              background: "var(--card)",
              color: "var(--muted)",
            }}
          >
            {emptyState}
          </div>
        )}

        {!loading && tariffs.length > 0 && visibleTabs.length > 0 && (
          <>
            <div
              role="tablist"
              aria-label={tablistLabel}
              style={{
                display: "flex",
                flexWrap: "wrap",
                gap: 8,
                marginTop: 30,
              }}
            >
              {visibleTabs.map((tab) => (
                <button
                  key={tab.groupName}
                  type="button"
                  role="tab"
                  aria-selected={tab.groupName === selectedTab?.groupName}
                  onClick={() => setActiveGroup(tab.groupName)}
                  style={{
                    minHeight: 38,
                    padding: "0 14px",
                    borderRadius: 8,
                    border: `1px solid ${tab.groupName === selectedTab?.groupName ? "var(--accent)" : "var(--line)"}`,
                    background:
                      tab.groupName === selectedTab?.groupName
                        ? "color-mix(in oklab, var(--accent) 12%, var(--card))"
                        : "var(--card)",
                    color:
                      tab.groupName === selectedTab?.groupName
                        ? "var(--accent-strong)"
                        : "var(--foreground)",
                    fontFamily: "inherit",
                    fontWeight: 600,
                    cursor: "pointer",
                  }}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <section style={{ marginTop: 24 }}>
              {selectedTab?.badge && (
                <div
                  style={{
                    display: "inline-flex",
                    marginBottom: 10,
                    padding: "5px 9px",
                    borderRadius: 999,
                    background: "var(--soft)",
                    color: "var(--accent-strong)",
                    fontSize: 12,
                    fontWeight: 700,
                  }}
                >
                  {selectedTab.badge}
                </div>
              )}
              {selectedTab?.description && (
                <p
                  style={{
                    maxWidth: 720,
                    margin: "0 0 14px",
                    color: "var(--muted)",
                    lineHeight: 1.5,
                  }}
                >
                  {selectedTab.description}
                </p>
              )}
              {visibleTariffs.length === 0 ? (
                <div
                  style={{
                    padding: 18,
                    border: "1px solid var(--line)",
                    borderRadius: 8,
                    background: "var(--card)",
                    color: "var(--muted)",
                  }}
                >
                  {emptyState}
                </div>
              ) : (
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))",
                    gap: 14,
                  }}
                >
                  {visibleTariffs.map((tariff) => (
                    <TariffCard
                      key={tariff.id}
                      tariff={tariff}
                      content={content}
                      benefits={benefits}
                      periodMonths={selectedPeriod?.months || 1}
                      discountPercent={Number(selectedPeriod?.discountPercent) || 0}
                    />
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </section>
    </main>
  );
}

function TariffCard({
  tariff,
  content,
  benefits,
  periodMonths,
  discountPercent,
}: {
  tariff: PublicTariff;
  content: SiteContent;
  benefits: ShopBenefitContent[];
  periodMonths: number;
  discountPercent: number;
}) {
  const locale = useLocale();
  const t = useT(shopText);
  const buyLabel = locale === "en"
    ? t.buyLabel
    : getString(
        content,
        "shop.buy_button_label",
        "Купить решение",
      );
  const payingLabel = locale === "en"
    ? t.payingLabel
    : getString(
        content,
        "shop.payment_loading_text",
        "Создаём платёж...",
      );
  const pricePeriodLabel = locale === "en"
    ? t.pricePeriodLabel
    : getString(
        content,
        "shop.price_period_label",
        "в месяц",
      );
  const totalPriceTemplate = locale === "en"
    ? t.totalPriceTemplate
    : getString(
        content,
        "shop.total_price_template",
        "Итого за {{months}} мес.: {{total}}",
      );
  const dailyLimitTemplate = locale === "en"
    ? t.dailyLimitTemplate
    : getString(
        content,
        "shop.daily_limit_template",
        "До {{count}} сообщений в день",
      );
  const solutionCountTemplate = locale === "en"
    ? t.solutionCountTemplate
    : getString(
        content,
        "shop.solution_count_template",
        "Режимов внутри: {{count}}",
      );
  const modesLabel = locale === "en"
    ? t.modesLabel
    : getString(
        content,
        "shop.included_items_label",
        "Режимы внутри",
      );
  const autoRenewLabel = locale === "en"
    ? t.autoRenewLabel
    : getString(
        content,
        "shop.autorenew_checkbox_label",
        "Продлевать автоматически: {{period}}",
      );
  const oneTimeLabel = locale === "en"
    ? t.oneTimeLabel
    : getString(
        content,
        "shop.one_time_checkbox_label",
        "Разово, без продления",
      );
  const paymentHint = locale === "en"
    ? t.paymentHint
    : getString(
        content,
        "shop.payment_hint",
        "Доступ сразу после оплаты.",
      );
  const relevantBenefits = benefits.filter((item) => {
    const groupMatch =
      !item.groupName ||
      item.groupName === "*" ||
      item.groupName === tariff.groupName;
    const tariffMatch =
      !item.tariffName ||
      item.tariffName === "*" ||
      item.tariffName === tariff.name;
    return groupMatch && tariffMatch;
  });
  const [paying, setPaying] = useState(false);
  const [autoRenew, setAutoRenew] = useState(true);
  const [error, setError] = useState("");
  const totalBeforeDiscount = tariff.monthlyPrice * periodMonths;
  const discountRate = Math.max(0, Math.min(95, discountPercent)) / 100;
  const totalPrice = Math.round(totalBeforeDiscount * (1 - discountRate));
  const periodText = locale === "en"
    ? (periodMonths === 1 ? t.periodEveryMonth : t.periodEveryMonths(periodMonths))
    : (periodMonths === 1 ? "каждый месяц" : `каждые ${periodMonths} мес.`);
  const autoRenewText = fillVars(autoRenewLabel, {
    months: periodMonths,
    period: periodText,
  }).replace("каждый месяц", periodText);

  async function buy() {
    if (paying) return;
    setPaying(true);
    setError("");
    try {
      const res = await apiFetch<{ confirmationUrl?: string }>(
        "/api/payments/yookassa/create",
        {
          method: "POST",
          body: JSON.stringify({
            tariffId: tariff.id,
            subscriptionMonths: periodMonths,
            enableAutoRenew: autoRenew,
            returnUrl: `${window.location.origin}/profile`,
          }),
        },
      );
      if (res.confirmationUrl) {
        window.location.href = res.confirmationUrl;
        return;
      }
      setError(t.paymentNoLinkError);
    } catch (e) {
      setError(e instanceof Error ? e.message : t.paymentGenericError);
    } finally {
      setPaying(false);
    }
  }

  return (
    <article
      style={{
        display: "flex",
        flexDirection: "column",
        minHeight: 280,
        padding: 20,
        border: "1px solid var(--line)",
        borderRadius: 8,
        background: "var(--card)",
      }}
    >
      <h3
        style={{
          margin: 0,
          fontFamily: "var(--font-unbounded)",
          fontSize: 20,
          lineHeight: 1.25,
          letterSpacing: 0,
        }}
      >
        {tariff.name}
      </h3>
      {tariff.description && (
        <p
          style={{
            color: "var(--muted)",
            fontSize: 14,
            lineHeight: 1.5,
            margin: "10px 0 0",
          }}
        >
          {tariff.description}
        </p>
      )}
      <div
        style={{
          marginTop: 18,
          display: "flex",
          alignItems: "baseline",
          gap: 6,
        }}
      >
        <strong
          style={{
            fontSize: 30,
            fontFamily: "var(--font-unbounded)",
            letterSpacing: 0,
          }}
        >
          {formatRub(tariff.monthlyPrice)}
        </strong>
        <span style={{ color: "var(--muted)", fontSize: 13 }}>
          {pricePeriodLabel}
        </span>
      </div>
      {periodMonths > 1 && (
        <div style={{ marginTop: 6, color: "var(--muted)", fontSize: 13 }}>
          {totalPriceTemplate
            .replace("{{months}}", String(periodMonths))
            .replace("{{total}}", formatRub(totalPrice))}
          {discountRate > 0 && (
            <span style={{ marginLeft: 6, color: "var(--accent-strong)", fontWeight: 700 }}>
              {t.discountLabel(Math.round(discountRate * 100))}
            </span>
          )}
        </div>
      )}
      <div
        style={{
          marginTop: 14,
          display: "grid",
          gap: 8,
          color: "var(--muted)",
          fontSize: 14,
        }}
      >
        <div>
          {dailyLimitTemplate.replace(
            "{{count}}",
            tariff.dailyMessageLimit.toLocaleString(locale === "en" ? "en-US" : "ru-RU"),
          )}
        </div>
        <div>
          {solutionCountTemplate.replace(
            "{{count}}",
            String(tariff.modes.length || tariff.modeIds.length),
          )}
        </div>
      </div>
      {tariff.modes.length > 0 && (
        <div
          style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 14 }}
        >
          <span
            style={{
              width: "100%",
              color: "var(--muted)",
              fontSize: 12,
              fontWeight: 700,
            }}
          >
            {modesLabel}
          </span>
          {tariff.modes.slice(0, 6).map((mode) => (
            <span
              key={mode.id}
              className="ms-shop-mode-pill"
              aria-label={`${mode.name}: ${mode.welcomeMessage || mode.name}`}
              tabIndex={mode.welcomeMessage ? 0 : undefined}
              style={{
                position: "relative",
                padding: "5px 8px",
                borderRadius: 999,
                background: "var(--soft)",
                color: "var(--accent-strong)",
                fontSize: 12,
                cursor: mode.welcomeMessage ? "help" : "default",
              }}
            >
              {mode.name}
              {mode.welcomeMessage && (
                <span className="ms-shop-mode-tip" role="tooltip">
                  {mode.welcomeMessage}
                </span>
              )}
            </span>
          ))}
          {tariff.modes.length > 6 && (
            <span
              style={{
                padding: "5px 8px",
                color: "var(--muted)",
                fontSize: 12,
              }}
            >
              +{tariff.modes.length - 6}
            </span>
          )}
        </div>
      )}
      {relevantBenefits.length > 0 && (
        <div style={{ display: "grid", gap: 8, marginTop: 14 }}>
          {relevantBenefits.slice(0, 4).map((benefit) => (
            <div
              key={`${benefit.title}-${benefit.body}`}
              style={{
                padding: "9px 10px",
                borderRadius: 8,
                background: "var(--soft)",
                border: "1px solid var(--line)",
              }}
            >
              <div
                style={{
                  color: "var(--foreground)",
                  fontSize: 13,
                  fontWeight: 700,
                }}
              >
                {benefit.title}
              </div>
              <div
                style={{
                  marginTop: 3,
                  color: "var(--muted)",
                  fontSize: 12,
                  lineHeight: 1.45,
                }}
              >
                {benefit.body}
              </div>
            </div>
          ))}
        </div>
      )}
      {error && (
        <div
          style={{
            marginTop: 14,
            color: "#b00020",
            fontSize: 13,
            lineHeight: 1.45,
          }}
        >
          {error}
        </div>
      )}
      <label className="ms-shop-renew-choice">
        <input
          type="checkbox"
          checked={autoRenew}
          disabled={paying}
          onChange={(e) => setAutoRenew(e.target.checked)}
          className="ms-shop-renew-input"
        />
        <span className="ms-shop-renew-track" aria-hidden="true">
          <span className="ms-shop-renew-thumb" />
        </span>
        <span className="ms-shop-renew-copy">
          <span className="ms-shop-renew-main">
            {autoRenew ? autoRenewText : oneTimeLabel}
          </span>
          <span className="ms-shop-renew-hint">
            {autoRenew ? t.renewHintOn : t.renewHintOff}
          </span>
        </span>
      </label>
      {paymentHint && (
        <div style={{ marginTop: 10, color: "var(--muted)", fontSize: 12, lineHeight: 1.45 }}>
          {paymentHint}
        </div>
      )}
      <button
        type="button"
        onClick={() => void buy()}
        disabled={paying}
        style={{
          marginTop: "auto",
          height: 44,
          borderRadius: 10,
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          background: paying ? "var(--muted)" : "var(--accent)",
          color: "#fff",
          fontWeight: 600,
          textDecoration: "none",
          border: 0,
          cursor: paying ? "wait" : "pointer",
          fontFamily: "inherit",
        }}
      >
        {paying ? payingLabel : buyLabel}
      </button>
      <style>{`
        .ms-shop-mode-pill:hover .ms-shop-mode-tip,
        .ms-shop-mode-pill:focus-within .ms-shop-mode-tip {
          opacity: 1;
          transform: translate(0, -50%);
          pointer-events: auto;
        }
        .ms-shop-mode-tip {
          position: absolute;
          left: calc(100% + 8px);
          top: 50%;
          z-index: 20;
          width: min(280px, 78vw);
          padding: 10px 12px;
          border: 1px solid var(--line);
          border-radius: 8px;
          background: var(--card);
          color: var(--foreground);
          box-shadow: 0 14px 34px rgba(15, 50, 53, 0.16);
          font-size: 12px;
          line-height: 1.45;
          opacity: 0;
          pointer-events: none;
          transform: translate(-4px, -50%);
          transition: opacity 150ms ease, transform 150ms ease;
        }
        @media (max-width: 640px) {
          .ms-shop-mode-tip {
            left: 0;
            top: auto;
            bottom: calc(100% + 8px);
            transform: translateY(4px);
          }
          .ms-shop-mode-pill:hover .ms-shop-mode-tip,
          .ms-shop-mode-pill:focus-within .ms-shop-mode-tip {
            transform: translateY(0);
          }
        }
        .ms-shop-renew-choice {
          display: grid;
          grid-template-columns: auto minmax(0, 1fr);
          gap: 10px;
          align-items: center;
          margin-top: 16px;
          padding: 10px;
          border: 1px solid transparent;
          border-radius: 8px;
          color: var(--muted);
          cursor: pointer;
          transition: background 160ms ease, border-color 160ms ease, transform 160ms ease;
        }
        .ms-shop-renew-choice:hover,
        .ms-shop-renew-choice:focus-within {
          background: var(--soft);
          border-color: var(--line);
          transform: translateY(-1px);
        }
        .ms-shop-renew-input {
          position: absolute;
          opacity: 0;
          width: 1px;
          height: 1px;
          pointer-events: none;
        }
        .ms-shop-renew-track {
          width: 38px;
          height: 22px;
          padding: 2px;
          border-radius: 999px;
          background: color-mix(in oklab, var(--accent) 60%, var(--card));
          border: 1px solid color-mix(in oklab, var(--accent) 54%, var(--line));
          transition: background 160ms ease, border-color 160ms ease;
        }
        .ms-shop-renew-thumb {
          display: block;
          width: 16px;
          height: 16px;
          border-radius: 999px;
          background: #fff;
          box-shadow: 0 2px 6px rgba(0, 0, 0, 0.2);
          transform: translateX(16px);
          transition: transform 160ms ease;
        }
        .ms-shop-renew-input:not(:checked) + .ms-shop-renew-track {
          background: var(--card);
          border-color: var(--line);
        }
        .ms-shop-renew-input:not(:checked) + .ms-shop-renew-track .ms-shop-renew-thumb {
          transform: translateX(0);
          background: var(--muted);
        }
        .ms-shop-renew-main {
          display: block;
          color: var(--foreground);
          font-size: 13px;
          font-weight: 650;
          line-height: 1.25;
        }
        .ms-shop-renew-hint {
          display: block;
          margin-top: 2px;
          font-size: 11px;
          line-height: 1.35;
          color: var(--muted);
        }
      `}</style>
    </article>
  );
}

function ClosedSection({
  title,
  subtitle,
}: {
  title: string;
  subtitle: string;
}) {
  return (
    <main
      style={{
        minHeight: "100dvh",
        background: "var(--background)",
        color: "var(--foreground)",
        fontFamily: "var(--font-golos)",
      }}
    >
      <StorefrontHeader active="shop" />
      <section
        style={{ maxWidth: 720, margin: "0 auto", padding: "80px 24px" }}
      >
        <h1
          style={{
            margin: 0,
            fontFamily: "var(--font-unbounded)",
            fontSize: 34,
            lineHeight: 1.15,
            letterSpacing: 0,
          }}
        >
          {title}
        </h1>
        <p style={{ color: "var(--muted)", lineHeight: 1.6 }}>{subtitle}</p>
      </section>
    </main>
  );
}
