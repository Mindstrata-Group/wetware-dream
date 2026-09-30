"use client";

import Link from "next/link";
import { MS, FONT_HEAD, FONT_MONO } from "../_parts/_theme";
import { formatActiveTo } from "../_parts/_utils";
import { useT } from "@/lib/i18n/LocaleProvider";
import { accessText } from "../translations";
import type { Locale } from "@/lib/i18n/types";

export function SuccessView({
  promoInfo,
  successMessage,
  countdown,
  goHref,
  locale,
}: any) {
  const t = useT(accessText);
  const renderModes = (
    list: Array<{ modeId?: number; name: string; activeTo?: string }>,
  ) => {
    if (!list.length) return <span style={{ color: MS.ink50 }}>—</span>;
    return (
      <span>
        {list.map((m, i) => (
          <span key={`${m.modeId ?? m.name}-${i}`}>
            {i > 0 ? ", " : ""}
            {m.name}{" "}
            <span style={{ color: MS.ink50 }}>
              ({t.success.untilPrefix} {formatActiveTo(m.activeTo, locale as Locale)})
            </span>
          </span>
        ))}
      </span>
    );
  };

  return (
    <main
      style={{
        position: "relative",
        flex: 1,
        padding: "40px 20px",
        display: "flex",
        justifyContent: "center",
      }}
    >
      <div
        style={{
          width: "100%",
          maxWidth: 580,
          display: "flex",
          flexDirection: "column",
          gap: 16,
        }}
      >
        <div
          style={{
            display: "inline-flex",
            alignSelf: "flex-start",
            alignItems: "center",
            gap: 8,
            padding: "6px 12px",
            borderRadius: 8,
            background: MS.greenLight,
            border: `1px solid ${MS.green}`,
            color: MS.greenDark,
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          <div
            style={{
              width: 8,
              height: 8,
              borderRadius: "50%",
              background: MS.green,
            }}
          />
          {promoInfo.roleUpgraded ? t.success.roleUpgradedBadge : t.success.accessGrantedBadge}
        </div>

        <h1
          style={{
            fontFamily: FONT_HEAD,
            fontWeight: 600,
            fontSize: 32,
            letterSpacing: "-0.01em",
            margin: 0,
            lineHeight: 1.15,
          }}
        >
          {promoInfo.roleUpgraded ? t.success.roleUpgradedBadge : t.success.accessGrantedBadge}
        </h1>

        <div>
          <div
            style={{
              fontSize: 12,
              color: MS.ink70,
              marginBottom: 6,
              fontWeight: 500,
            }}
          >
            {t.success.promoLabel}
          </div>
          <div
            style={{
              minHeight: 56,
              padding: "14px",
              border: `1px solid ${MS.ink20}`,
              borderRadius: 10,
              background: MS.surface,
              fontFamily: FONT_MONO,
              fontSize: 14,
              color: MS.ink,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              letterSpacing: "0.5px",
              wordBreak: "break-all",
              textAlign: "center",
            }}
          >
            {promoInfo.code}
          </div>
        </div>

        {successMessage && (
          <div
            style={{
              padding: 14,
              borderRadius: 10,
              background: MS.greenLight,
              border: `1px solid ${MS.green}`,
              color: MS.greenDark,
              fontSize: 13,
              lineHeight: 1.5,
            }}
          >
            {successMessage}
          </div>
        )}

        <div
          style={{
            padding: 16,
            borderRadius: 12,
            background: MS.surface,
            border: `1px solid ${MS.ink10}`,
            boxShadow: "0 10px 30px -15px rgba(13,27,22,0.1)",
          }}
        >
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "160px 1fr",
              gap: "10px 16px",
              fontSize: 13,
              lineHeight: 1.55,
            }}
          >
            <div style={{ color: MS.ink50 }}>{t.success.promoLabel}:</div>
            <div
              style={{
                fontFamily: FONT_MONO,
                fontSize: 12,
                wordBreak: "break-all",
              }}
            >
              {promoInfo.code}
            </div>

            {promoInfo.roleUpgraded ? (
              <>
                <div style={{ color: MS.ink50 }}>{t.success.roleLabel}</div>
                <div style={{ fontWeight: 600 }}>{t.success.roleValue}</div>

                <div style={{ color: MS.ink, fontWeight: 500 }}>
                  {t.success.autoRedirectAdmin}
                </div>
                <div
                  style={{
                    fontWeight: 600,
                    fontVariantNumeric: "tabular-nums",
                  }}
                >
                  {countdown} {t.success.seconds}
                </div>
              </>
            ) : (
              <>
                <div style={{ color: MS.ink50 }}>{t.success.accessPeriodLabel}</div>
                <div>
                  {promoInfo.accessDays
                    ? t.success.accessDaysValue(promoInfo.accessDays)
                    : t.success.accessPeriodFallback}
                </div>

                <div style={{ color: MS.ink50 }}>{t.success.modesLabel}</div>
                <div>
                  {promoInfo.modes.length ? (
                    renderModes(promoInfo.modes)
                  ) : (
                    <span style={{ color: MS.ink50 }}>
                      {t.success.modesEmptyFallback}
                    </span>
                  )}
                </div>

                <div style={{ color: MS.ink50 }}>{t.success.grantedLabel}</div>
                <div>{renderModes(promoInfo.grantedModes)}</div>

                <div style={{ color: MS.ink50 }}>{t.success.extendedLabel}</div>
                <div>{renderModes(promoInfo.extendedModes)}</div>

                <div style={{ color: MS.ink, fontWeight: 500 }}>
                  {t.success.autoRedirectChat}
                </div>
                <div
                  style={{
                    fontWeight: 600,
                    fontVariantNumeric: "tabular-nums",
                  }}
                >
                  {countdown} {t.success.seconds}
                </div>
              </>
            )}
          </div>
        </div>

        <div
          style={{
            display: "flex",
            gap: 8,
            flexWrap: "wrap",
            marginTop: 4,
          }}
        >
          <Link
            href={goHref}
            style={{
              height: 48,
              padding: "0 22px",
              borderRadius: 12,
              background: MS.green,
              color: "#fff",
              fontSize: 15,
              fontWeight: 600,
              border: "none",
              cursor: "pointer",
              fontFamily: "inherit",
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              textDecoration: "none",
            }}
          >
            {promoInfo.roleUpgraded ? t.success.goAdmin : t.success.goChat}
          </Link>
          <Link
            href="/"
            style={{
              height: 48,
              padding: "0 22px",
              borderRadius: 12,
              border: `1px solid ${MS.ink20}`,
              background: MS.surface,
              color: MS.ink,
              fontSize: 15,
              fontWeight: 500,
              display: "inline-flex",
              alignItems: "center",
              textDecoration: "none",
            }}
          >
            {t.success.backHome}
          </Link>
        </div>
      </div>
    </main>
  );
}
