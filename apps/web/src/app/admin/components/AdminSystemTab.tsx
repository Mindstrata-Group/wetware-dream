"use client";

import { useEffect, useState } from "react";
import { useAdminPageContext } from "./AdminPageContext";
import { apiBase } from "../../chat/utils";
import { SYSTEM_ACTIONS, SYSTEM_LINKS } from "@/lib/opsLinks";

const COUNT_LABELS: Record<string, string> = {
  users: "Пользователей",
  activeUsers: "Активных пользователей",
  sessions: "Активных сессий",
  modes: "Режимов",
  dialogs: "Диалогов",
  messages: "Сообщений",
};

type ClientLogEvent = {
  type: string;
  ts?: number;
  composing?: boolean;
  domLen?: number;
  stateLen?: number;
  wasComposing?: boolean;
  extra?: string;
};

type ClientLogEntry = {
  at: string;
  userId: number;
  role: string;
  ua: string;
  path: string;
  events: ClientLogEvent[];
};

export function AdminSystemTab() {
  const { status, loadSystem } = useAdminPageContext();

  const counts: Record<string, number> = status?.counts || {};

  const [clientLogs, setClientLogs] = useState<ClientLogEntry[] | null>(null);
  const [clientLogsEnabled, setClientLogsEnabled] = useState<boolean | null>(null);
  const [clientLogsLoading, setClientLogsLoading] = useState(false);
  const [clientLogsToggling, setClientLogsToggling] = useState(false);
  const [clientLogsError, setClientLogsError] = useState<string | null>(null);
  const [expandedRow, setExpandedRow] = useState<number | null>(null);

  // ── Yandex.Metrica ──
  const [metrikaId, setMetrikaId] = useState("");
  const [metrikaParams, setMetrikaParams] = useState("");
  const [metrikaLoaded, setMetrikaLoaded] = useState(false);
  const [metrikaSaving, setMetrikaSaving] = useState(false);
  const [metrikaMsg, setMetrikaMsg] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await fetch(`${apiBase}/api/admin/analytics-settings`, { credentials: "include" });
        const json = await res.json().catch(() => ({}));
        if (!res.ok || json.ok === false) throw new Error(json.error || `HTTP ${res.status}`);
        if (cancelled) return;
        setMetrikaId(json.metrikaCounterId || "");
        setMetrikaParams(json.metrikaParams || "");
        setMetrikaLoaded(true);
      } catch (e) {
        if (!cancelled) setMetrikaMsg({ kind: "error", text: e instanceof Error ? e.message : "Ошибка загрузки" });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function saveMetrika() {
    setMetrikaSaving(true);
    setMetrikaMsg(null);
    try {
      const res = await fetch(`${apiBase}/api/admin/analytics-settings`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metrikaCounterId: metrikaId.trim(), metrikaParams: metrikaParams.trim() }),
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok || json.ok === false) throw new Error(json.error || `HTTP ${res.status}`);
      setMetrikaMsg({ kind: "ok", text: "Сохранено. На сайте обновится в течение ~5 минут." });
    } catch (e) {
      setMetrikaMsg({ kind: "error", text: e instanceof Error ? e.message : "Ошибка сохранения" });
    } finally {
      setMetrikaSaving(false);
    }
  }

  async function loadClientLogs() {
    setClientLogsLoading(true);
    setClientLogsError(null);
    try {
      const res = await fetch(`${apiBase}/api/admin/client-logs`, { credentials: "include" });
      const json = await res.json().catch(() => ({}));
      if (!res.ok || json.ok === false) throw new Error(json.error || `HTTP ${res.status}`);
      const entries: ClientLogEntry[] = json.entries || [];
      setClientLogsEnabled(json.enabled === true);
      setClientLogs(entries.slice(-200).reverse());
    } catch (e) {
      setClientLogsError(e instanceof Error ? e.message : "Ошибка загрузки");
    } finally {
      setClientLogsLoading(false);
    }
  }

  async function toggleClientLogs(enable: boolean) {
    setClientLogsToggling(true);
    setClientLogsError(null);
    try {
      const res = await fetch(`${apiBase}/api/admin/client-logs`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled: enable }),
      });
      const json = await res.json().catch(() => ({}));
      if (!res.ok || json.ok === false) throw new Error(json.error || `HTTP ${res.status}`);
      setClientLogsEnabled(enable);
      if (!enable) setClientLogs(null); // the buffer was reset on the server
    } catch (e) {
      setClientLogsError(e instanceof Error ? e.message : "Ошибка");
    } finally {
      setClientLogsToggling(false);
    }
  }

  return (
    <div className="card ms-admin-card">
      <div className="ms-admin-card-head">
        <h2>Система</h2>
        <button
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={loadSystem}
        >
          Обновить
        </button>
      </div>

      {/* ── Entity counters ── */}
      <div className="ms-stat-grid" style={{ marginBottom: 20 }}>
        {Object.entries(counts).map(([k, v]) => (
          <div key={k} className="ms-stat-card">
            <span>{COUNT_LABELS[k] || k}</span>
            <strong>{Number(v).toLocaleString("ru-RU")}</strong>
          </div>
        ))}
        {Object.keys(counts).length === 0 && (
          <div className="ms-stat-card">
            <span>Статус</span>
            <strong style={{ fontSize: 16 }}>Загрузка...</strong>
          </div>
        )}
      </div>

      {SYSTEM_LINKS.length > 0 && (
      <div className="ms-admin-service-links" aria-label="Служебные панели">
        {SYSTEM_LINKS.map((link) => (
          <a
            key={link.href}
            className="ms-button ms-button-primary ms-button-xs"
            href={link.href}
            target="_blank"
            rel="noreferrer"
          >
            {link.label}
          </a>
        ))}
      </div>
      )}

      {SYSTEM_ACTIONS.length > 0 && (
      <div className="ms-admin-service-links" aria-label="Операции окружения">
        {SYSTEM_ACTIONS.map((link) => (
          <a
            key={link.href}
            className="ms-button ms-button-ghost ms-button-xs"
            href={link.href}
            target="_blank"
            rel="noreferrer"
            title={link.title}
          >
            {link.label}
          </a>
        ))}
      </div>
      )}

      {(SYSTEM_LINKS.length > 0 || SYSTEM_ACTIONS.length > 0) && (
      <div
        className="ms-info-box"
        style={{ marginTop: 16, fontSize: 13, lineHeight: 1.55 }}
      >
        Быстрые ссылки ведут только в панели, которые можно открывать из админки.
        {SYSTEM_ACTIONS.length > 0 && (
          <>
            {" "}Кнопка Main → Stage открывает ручной workflow, который принудительно
            сбрасывает ветку staging на origin/main и запускает обычный staging deploy.
          </>
        )}
        {" "}Логи и метрики остаются внутренними эксплуатационными инструментами.
      </div>
      )}

      {/* ── Yandex.Metrica ── */}
      <div style={{ marginTop: 24 }} aria-label="Яндекс.Метрика">
        <strong style={{ fontSize: 14, display: "block", marginBottom: 8 }}>Яндекс.Метрика</strong>
        <div style={{ display: "flex", flexDirection: "column", gap: 8, maxWidth: 560 }}>
          <label style={{ fontSize: 13 }}>
            Номер счётчика
            <input
              className="ms-input"
              type="text"
              inputMode="numeric"
              placeholder="например 98765432 (пусто = выключено)"
              value={metrikaId}
              onChange={(e) => setMetrikaId(e.target.value)}
              disabled={!metrikaLoaded}
              style={{ display: "block", width: "100%", marginTop: 4 }}
            />
          </label>
          <label style={{ fontSize: 13 }}>
            Параметры init (JSON)
            <textarea
              className="ms-input"
              rows={4}
              placeholder='{"clickmap":true,"trackLinks":true,"accurateTrackBounce":true,"webvisor":false}'
              value={metrikaParams}
              onChange={(e) => setMetrikaParams(e.target.value)}
              disabled={!metrikaLoaded}
              style={{ display: "block", width: "100%", marginTop: 4, fontFamily: "monospace", fontSize: 12 }}
            />
          </label>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <button
              className="ms-button ms-button-primary ms-button-xs"
              onClick={() => void saveMetrika()}
              disabled={!metrikaLoaded || metrikaSaving}
            >
              {metrikaSaving ? "Сохранение..." : "Сохранить"}
            </button>
            {metrikaMsg && (
              <span style={{ fontSize: 12, color: metrikaMsg.kind === "ok" ? "#1D9E75" : "red" }}>
                {metrikaMsg.text}
              </span>
            )}
          </div>
          <div style={{ fontSize: 12, color: "#888", lineHeight: 1.5 }}>
            Номер счётчика — из кабинета Метрики (Настройки → Счётчик). Код вставлять не нужно:
            сайт сам подключает tag.js с этим номером на всех публичных страницах
            (разделы /admin, /promo-admin и /tester не считаются).
          </div>
        </div>
      </div>

      {/* ── Textarea client logs ── */}
      <div style={{ marginTop: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 8, flexWrap: "wrap" }}>
          <strong style={{ fontSize: 14 }}>Клиент-логи (textarea)</strong>
          <button
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={() => void loadClientLogs()}
            disabled={clientLogsLoading}
          >
            {clientLogsLoading ? "Загрузка..." : clientLogs !== null ? "Обновить" : "Статус"}
          </button>
          {clientLogsEnabled !== null && (
            <button
              className={`ms-button ms-button-xs ${clientLogsEnabled ? "ms-button-ghost" : "ms-button-primary"}`}
              onClick={() => void toggleClientLogs(!clientLogsEnabled)}
              disabled={clientLogsToggling}
              style={{ minWidth: 100 }}
            >
              {clientLogsToggling ? "..." : clientLogsEnabled ? "Выключить логи" : "Включить логи"}
            </button>
          )}
          {clientLogsEnabled !== null && (
            <span style={{ fontSize: 12, color: clientLogsEnabled ? "#1D9E75" : "#999", fontWeight: 500 }}>
              {clientLogsEnabled ? "включены" : "выключены"}
            </span>
          )}
          {clientLogs && (
            <span style={{ fontSize: 12, color: "#666" }}>{clientLogs.length} записей</span>
          )}
        </div>
        {clientLogsError && (
          <div style={{ color: "red", fontSize: 13, marginBottom: 8 }}>{clientLogsError}</div>
        )}
        {clientLogs && clientLogs.length === 0 && (
          <div style={{ fontSize: 13, color: "#888" }}>Нет записей за последние 24 часа</div>
        )}
        {clientLogs && clientLogs.length > 0 && (
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", fontSize: 12, borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ background: "#f5f5f5", textAlign: "left" }}>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd" }}>at</th>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd" }}>userId</th>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd" }}>role</th>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd", maxWidth: 160 }}>ua</th>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd" }}>events</th>
                  <th style={{ padding: "4px 8px", borderBottom: "1px solid #ddd" }}>раскрыть</th>
                </tr>
              </thead>
              <tbody>
                {clientLogs.map((entry, i) => (
                  <>
                    <tr key={i} style={{ borderBottom: "1px solid #eee" }}>
                      <td style={{ padding: "3px 8px", whiteSpace: "nowrap" }}>{entry.at}</td>
                      <td style={{ padding: "3px 8px" }}>{entry.userId}</td>
                      <td style={{ padding: "3px 8px" }}>{entry.role}</td>
                      <td style={{ padding: "3px 8px", maxWidth: 160, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={entry.ua}>
                        {entry.ua.slice(0, 60)}{entry.ua.length > 60 ? "…" : ""}
                      </td>
                      <td style={{ padding: "3px 8px" }}>{entry.events.length}</td>
                      <td style={{ padding: "3px 8px" }}>
                        <button
                          className="ms-button ms-button-ghost ms-button-xs"
                          onClick={() => setExpandedRow(expandedRow === i ? null : i)}
                          style={{ fontSize: 11 }}
                        >
                          {expandedRow === i ? "свернуть" : "раскрыть"}
                        </button>
                      </td>
                    </tr>
                    {expandedRow === i && (
                      <tr key={`${i}-expanded`}>
                        <td colSpan={6} style={{ padding: "4px 8px 8px 16px", background: "#fafafa" }}>
                          <div style={{ fontFamily: "monospace", fontSize: 11, lineHeight: 1.6 }}>
                            {entry.events.map((ev, j) => (
                              <div key={j}>
                                [{ev.type}
                                {ev.composing !== undefined ? ` composing=${String(ev.composing)}` : ""}
                                {ev.wasComposing !== undefined ? ` wasComposing=${String(ev.wasComposing)}` : ""}
                                {ev.domLen !== undefined ? ` domLen=${ev.domLen}` : ""}
                                {ev.stateLen !== undefined ? ` stateLen=${ev.stateLen}` : ""}
                                {ev.extra ? ` extra=${ev.extra}` : ""}
                                {ev.ts ? ` @${ev.ts}` : ""}]
                              </div>
                            ))}
                          </div>
                        </td>
                      </tr>
                    )}
                  </>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
