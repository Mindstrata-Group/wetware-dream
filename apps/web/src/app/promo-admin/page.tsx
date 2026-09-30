"use client";
import {
  Suspense,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { usePromoStatusLoader } from "./_hooks/usePromoStatusLoader";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { BrandLogo } from "@/components/BrandLogo";
import { MessageContent } from "@/components/MessageContent";
import { ApiError, apiFetch } from "@/lib/api";
import { MS, FONT_HEAD, FONT_BODY, FONT_MONO } from "./_theme";
import type {
  PromoStatus, PromptOption, ModeOption, StatusResponse, SummarizeResponse, HistoryItem,
} from "./_types";
import {
  promoAccessUrl, absolutePromoAccessUrl, formatDate, formatBytes, ghostBtnStyle,
} from "./_utils";
import { CenterMessage } from "./_components/CenterMessage";
import { QRCard } from "./_components/QRCard";
import { HistoryList } from "./_views/HistoryList";
import { SummarizeForm } from "./_views/SummarizeForm";
function PromoAdminContent() {
  const params = useSearchParams();
  const promo = (params?.get("promo") || "").trim();
  const promoKey = (params?.get("key") || params?.get("token") || "").trim();
  const [mobile, setMobile] = useState(false);
  useEffect(() => {
    const check = () => setMobile(window.innerWidth < 900);
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, []);
  const [modeIds, setModeIds] = useState<number[]>([]);
  const [result, setResult] = useState("");
  const [loadingSummary, setLoadingSummary] = useState(false);
  const [error, setError] = useState("");
  const [errorDebug, setErrorDebug] = useState<unknown>(null);
  const {
    status,
    setStatus,
    prompts,
    modes,
    history,
    setHistory,
    promptId,
    setPromptId,
    loadingStatus,
  } = usePromoStatusLoader(promo, promoKey, setError);
  const selectedModeNames = useMemo(() => {
    if (!modeIds.length) return "Все режимы промокода";
    return modes
      .filter((m) => modeIds.includes(m.id))
      .map((m) => m.name)
      .join(", ");
  }, [modeIds, modes]);
  async function summarize() {
    if (!promptId) return;
    setLoadingSummary(true);
    setError("");
    setErrorDebug(null);
    setResult("");
    try {
      const json = await apiFetch<SummarizeResponse>("/api/promo-admin/summarize", {
        method: "POST",
        body: JSON.stringify({
          code: promo,
          key: promoKey,
          modeIds,
          promptId,
        }),
      });
      setResult(json.result || "");
      setHistory((prev) =>
        [{ ...json, modeLabel: selectedModeNames }, ...prev].slice(0, 10),
      );
      setStatus((prev) =>
        prev
          ? {
              ...prev,
              summaryUsed: json.used,
              summaryRemaining: json.remaining ?? prev.summaryRemaining,
            }
          : prev,
      );
    } catch (e) {
      const message = e instanceof Error ? e.message : "Ошибка резюмирования";
      setErrorDebug(e instanceof ApiError ? e.debug : null);
      setError(
        message.includes("summary limit") ? "Лимит резюмирований закончился" : message,
      );
    } finally {
      setLoadingSummary(false);
    }
  }
  function downloadResult() {
    if (!result) return;
    const blob = new Blob([result], { type: "text/markdown;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `promo-${promo}-summary.md`;
    a.click();
    URL.revokeObjectURL(url);
  }
  function copyAccessUrl() {
    if (typeof navigator === "undefined" || !navigator.clipboard) return;
    navigator.clipboard.writeText(absolutePromoAccessUrl(promo)).catch(() => {});
  }
  if (loadingStatus) return <CenterMessage>Загружаем данные промокода…</CenterMessage>;
  if (!status && error) {
    return (
      <div
        style={{
          minHeight: "100vh",
          background: MS.bg,
          fontFamily: FONT_BODY,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: 20,
        }}
      >
        <div
          style={{
            maxWidth: 440,
            width: "100%",
            background: MS.surface,
            border: `1px solid ${MS.ink10}`,
            borderRadius: 20,
            padding: "40px 32px",
            textAlign: "center",
            boxShadow: "0 20px 60px -20px rgba(13,27,22,0.15)",
          }}
        >
          <div
            style={{
              width: 52,
              height: 52,
              borderRadius: 14,
              background: MS.warnLight,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              margin: "0 auto 16px",
              fontSize: 24,
            }}
          >
            🔒
          </div>
          <h2
            style={{
              fontFamily: FONT_HEAD,
              fontWeight: 600,
              fontSize: 18,
              margin: "0 0 10px",
              color: MS.ink,
            }}
          >
            Доступ закрыт
          </h2>
          <p
            style={{
              fontSize: 14,
              color: MS.ink50,
              margin: "0 0 22px",
              lineHeight: 1.55,
            }}
          >
            {error}
          </p>
          <Link
            href="/"
            className="ms-button ms-button-primary ms-button-xs"
            style={{ textDecoration: "none" }}
          >
            На главную
          </Link>
        </div>
      </div>
    );
  }
  if (!status) return <CenterMessage>Данные промокода не загружены.</CenterMessage>;
  const padX = mobile ? 16 : 32;
  const now = Date.now();
  const activeNow =
    !(status?.activeFrom && new Date(status.activeFrom).getTime() > now) &&
    !(status?.activeTo && new Date(status.activeTo).getTime() < now);
  return (
    <div style={{ minHeight: "100vh", background: MS.bg, fontFamily: FONT_BODY, color: MS.ink, position: "relative" }}>
      <div aria-hidden style={{ position: "absolute", inset: 0, opacity: 0.4, backgroundImage: "radial-gradient(circle, var(--foreground) 1px, transparent 1px)", backgroundSize: "24px 24px", pointerEvents: "none" }} />
      {/* Header */}
      <header
        style={{
          position: "sticky",
          top: 0,
          zIndex: 20,
          height: mobile ? 56 : 64,
          background: "color-mix(in oklab, var(--card) 88%, transparent)",
          backdropFilter: "blur(12px)",
          WebkitBackdropFilter: "blur(12px)",
          borderBottom: `1px solid ${MS.ink10}`,
          padding: `0 ${padX}px`,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <BrandLogo />
          {!mobile && (
            <>
              <span style={{ height: 22, width: 1, background: MS.ink20 }} />
              <span style={{ fontSize: 13, fontWeight: 500, color: MS.ink70 }}>
                Управление промокодом
              </span>
            </>
          )}
        </div>
        <Link
          href="/"
          className="ms-button ms-button-ghost ms-button-xs"
          style={{ textDecoration: "none" }}
        >
          ← Главная
        </Link>
      </header>
      <div
        style={{
          position: "relative",
          maxWidth: 1080,
          margin: "0 auto",
          padding: `${mobile ? 24 : 36}px ${padX}px`,
        }}
      >
        {/* Promo header */}
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "flex-start",
            gap: 12,
            flexWrap: "wrap",
            marginBottom: 22,
          }}
        >
          <div style={{ minWidth: 0, flex: 1 }}>
            <div
              style={{
                fontSize: 11,
                color: MS.ink50,
                textTransform: "uppercase",
                letterSpacing: "0.6px",
              }}
            >
              Промокод
            </div>
            <h1
              style={{
                fontFamily: FONT_MONO,
                fontWeight: 600,
                fontSize: mobile ? 24 : 32,
                letterSpacing: "-0.01em",
                margin: "6px 0 0",
                color: MS.greenDark,
                wordBreak: "break-all",
              }}
            >
              {status?.code || promo || "—"}
            </h1>
          </div>
          <span
            style={{
              fontSize: 12,
              padding: "4px 10px",
              borderRadius: 6,
              background: activeNow ? MS.greenLight : MS.surfaceSoft,
              color: activeNow ? MS.greenDark : MS.ink50,
              border: `1px solid ${activeNow ? "rgba(29,158,117,0.3)" : MS.ink10}`,
              fontWeight: 500,
            }}
          >
            {activeNow ? "активен" : "неактивен"}
          </span>
        </div>
        {/* Inline error if status loaded but action failed */}
        {error && status && (
          <div
            style={{
              background: MS.warnLight,
              border: "1px solid rgba(196,69,69,0.25)",
              borderRadius: 12,
              padding: "12px 14px",
              marginBottom: 16,
              color: "#7E2A2A",
              fontSize: 13,
            }}
          >
            {error}
            {errorDebug ? (
              <details style={{ marginTop: 8 }}>
                <summary style={{ cursor: "pointer", fontSize: 12 }}>
                  Расширенный лог live AI
                </summary>
                <pre
                  style={{
                    fontSize: 11,
                    marginTop: 8,
                    padding: 10,
                    background: "rgba(0,0,0,0.04)",
                    borderRadius: 8,
                    overflow: "auto",
                  }}
                >
                  {JSON.stringify(errorDebug, null, 2)}
                </pre>
              </details>
            ) : null}
          </div>
        )}
        {/* Stats */}
        <div
          style={{
            display: "grid",
            gridTemplateColumns: mobile ? "repeat(2,1fr)" : "repeat(4,1fr)",
            gap: 10,
            marginBottom: 22,
          }}
        >
          {[
            { l: "Активаций", v: String(status.usedCount ?? 0) },
            { l: "Сообщений", v: String(status.messageCount ?? 0) },
            { l: "Без 1-го сообщения", v: String(status.activationsWithoutMessages ?? 0) },
            { l: "Лимит", v: status.maxUses ? String(status.maxUses) : "∞" },
            {
              l: "Активен до",
              v: status.activeTo
                ? new Date(status.activeTo).toLocaleDateString("ru-RU")
                : "—",
            },
            {
              l: "Резюме осталось",
              v:
                status.summaryRemaining != null
                  ? String(status.summaryRemaining)
                  : status.summaryLimit
                    ? String(status.summaryLimit)
                    : "∞",
            },
          ].map((s) => (
            <div
              key={s.l}
              style={{
                background: MS.surface,
                border: `1px solid ${MS.ink10}`,
                borderRadius: 12,
                padding: "14px 16px",
              }}
            >
              <div
                style={{
                  fontSize: 11,
                  color: MS.ink50,
                  textTransform: "uppercase",
                  letterSpacing: "0.5px",
                }}
              >
                {s.l}
              </div>
              <div
                style={{
                  fontFamily: FONT_HEAD,
                  fontSize: mobile ? 22 : 26,
                  fontWeight: 600,
                  marginTop: 6,
                  fontVariantNumeric: "tabular-nums",
                  color: MS.ink,
                }}
              >
                {s.v}
              </div>
            </div>
          ))}
        </div>
        {/* Two-column body */}
        <div
          style={{
            display: "grid",
            gridTemplateColumns: mobile ? "1fr" : "1.4fr 1fr",
            gap: 16,
          }}
        >
          <SummarizeForm
            modes={modes}
            modeIds={modeIds}
            setModeIds={setModeIds}
            prompts={prompts}
            promptId={promptId}
            setPromptId={setPromptId}
            summarize={summarize}
            loadingSummary={loadingSummary}
            status={status}
            copyAccessUrl={copyAccessUrl}
            mobile={mobile}
            result={result}
            history={history}
            downloadResult={downloadResult}
          />
          <HistoryList promo={promo} history={history} setResult={setResult} />
        </div>
      </div>
      <style>{`@keyframes ms-spin { to { transform: rotate(360deg); } }`}</style>
    </div>
  );
}
export default function PromoAdminPage() {
  return (
    <Suspense fallback={<CenterMessage>Загрузка…</CenterMessage>}>
      <PromoAdminContent />
    </Suspense>
  );
}
