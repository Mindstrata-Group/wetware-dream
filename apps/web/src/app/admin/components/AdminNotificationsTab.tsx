"use client";

import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { MessageContent } from "@/components/MessageContent";
import { useAdminPageContext } from "./AdminPageContext";

type Audience = "all" | "admins" | "promocode" | "mode";
type ModeFilter = "wrote" | "has";
type PromoFilter = "wrote" | "not_wrote";
type Channel = "inbox" | "max" | "telegram";

// Text length limits per channel (characters); mirrors the backend.
const CHANNEL_TEXT_LIMITS: Record<Channel, number> = {
  inbox: 10000,
  max: 4000,
  telegram: 4096,
};

type PreviewRecipient = {
  id: number;
  email?: string;
  phone?: string;
  maxLinked: boolean;
  telegramLinked: boolean;
};

type PreviewData = {
  recipientCount: number;
  emailCount: number;
  phoneCount: number;
  maxLinkedCount: number;
  telegramLinkedCount: number;
  recipients: PreviewRecipient[];
  previewTruncated: boolean;
};

type HistoryItem = {
  id: number;
  actorEmail: string;
  audience: string;
  channels: string[];
  title: string;
  body: string;
  recipientCount: number;
  delivered: Record<string, number> | null;
  createdAt: string;
};

const CHANNEL_LABELS: Record<Channel, string> = {
  inbox: "Сайт (входящие)",
  max: "Max",
  telegram: "Telegram",
};

export function AdminNotificationsTab() {
  const { modes, promocodes } = useAdminPageContext();

  const [audience, setAudience] = useState<Audience>("all");
  const [modeFilter, setModeFilter] = useState<ModeFilter>("wrote");
  const [selectedModeIds, setSelectedModeIds] = useState<number[]>([]);
  const [selectedPromocodeIds, setSelectedPromocodeIds] = useState<number[]>([]);
  const [periodFrom, setPeriodFrom] = useState("");
  const [periodTo, setPeriodTo] = useState("");
  const [channels, setChannels] = useState<Channel[]>(["telegram"]);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [preview, setPreview] = useState<PreviewData | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [historyOffset, setHistoryOffset] = useState(0);
  const [historyFrom, setHistoryFrom] = useState("");
  const [historyTo, setHistoryTo] = useState("");
  const [promoSearch, setPromoSearch] = useState("");
  const [promoPage, setPromoPage] = useState(1);
  const [modeSearch, setModeSearch] = useState("");
  const [modePage, setModePage] = useState(1);
  const [promoFilters, setPromoFilters] = useState<PromoFilter[]>([]);
  const [excludedIds, setExcludedIds] = useState<number[]>([]);
  const [secondDelayHours, setSecondDelayHours] = useState("12");
  const [confirmStage, setConfirmStage] = useState(false);
  const [expandedHistoryId, setExpandedHistoryId] = useState<number | null>(null);
  // Full lists: the admin context loads only the first page
  // (modes 50, promo codes 100); the broadcast audience needs all of them.
  const [fullModes, setFullModes] = useState<{ id: number; name: string }[] | null>(null);
  const [fullPromocodes, setFullPromocodes] = useState<{ id: number; code: string; comment?: string }[] | null>(null);
  const [bonusSetting, setBonusSetting] = useState("");
  const [bonusSaving, setBonusSaving] = useState(false);
  const [bonusNotice, setBonusNotice] = useState("");
  const LIST_PAGE_SIZE = 30;
  const HISTORY_PAGE_SIZE = 20;

  useEffect(() => {
    let cancelled = false;
    async function loadAllPages<T>(url: (offset: number, limit: number) => string, key: string, limit: number): Promise<T[]> {
      const out: T[] = [];
      for (let offset = 0; offset < 5000; offset += limit) {
        const j = await apiFetch<any>(url(offset, limit));
        const page = (j[key] || []) as T[];
        out.push(...page);
        const total = j.total ?? page.length;
        if (out.length >= total || page.length === 0) break;
      }
      return out;
    }
    loadAllPages<{ id: number; name: string }>((o, l) => `/api/admin/modes?limit=${l}&offset=${o}`, "modes", 100)
      .then((all) => { if (!cancelled) setFullModes(all); })
      .catch(() => {});
    loadAllPages<{ id: number; code: string; comment?: string }>((o, l) => `/api/admin/promocodes?limit=${l}&offset=${o}`, "promocodes", 500)
      .then((all) => { if (!cancelled) setFullPromocodes(all); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    apiFetch<{ ok: boolean; items: { key: string; value: unknown }[] }>("/api/admin/site-content")
      .then((res) => {
        const item = (res.items || []).find((it) => it.key === "notifications.bonus_messages");
        if (item && typeof item.value === "number" && item.value > 0) setBonusSetting(String(item.value));
      })
      .catch(() => {});
  }, []);

  async function saveBonusSetting() {
    setBonusSaving(true);
    setBonusNotice("");
    try {
      const value = Math.max(0, Math.floor(Number(bonusSetting) || 0));
      await apiFetch("/api/admin/site-content", {
        method: "POST",
        body: JSON.stringify({ key: "notifications.bonus_messages", value }),
      });
      setBonusNotice(value > 0 ? `Сохранено: +${value} сообщений за подписку.` : "Сохранено: авто-формула (10% от минимального лимита, мин. 2).");
    } catch (e) {
      setBonusNotice(e instanceof Error ? e.message : "Не удалось сохранить");
    } finally {
      setBonusSaving(false);
    }
  }

  const allModes = fullModes ?? modes;
  const allPromocodes = fullPromocodes ?? (promocodes || []);
  const textLimit = Math.min(...(channels.length ? channels : ["inbox" as Channel]).map((ch) => CHANNEL_TEXT_LIMITS[ch]));
  const textLength = title.trim().length + body.trim().length + 4;

  // reset the selection when the audience changes
  useEffect(() => {
    setSelectedModeIds([]);
    setSelectedPromocodeIds([]);
    setPeriodFrom("");
    setPeriodTo("");
    setPreview(null);
  }, [audience]);

  const loadHistory = useCallback((offset = 0, from = "", to = "") => {
    const params = new URLSearchParams({ limit: String(HISTORY_PAGE_SIZE), offset: String(offset) });
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    apiFetch<{ ok: boolean; items: HistoryItem[]; total: number }>(`/api/admin/notifications/history?${params.toString()}`)
      .then((res) => {
        setHistory(res.items || []);
        setHistoryTotal(res.total ?? (res.items || []).length);
        setHistoryOffset(offset);
      })
      .catch(() => {});
    // historyFrom/historyTo are passed as arguments; the callback itself is stable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  useEffect(() => {
    loadHistory();
  }, [loadHistory]);

  function toggleId(list: number[], id: number) {
    return list.includes(id) ? list.filter((x) => x !== id) : [...list, id];
  }

  function toggleChannel(ch: Channel) {
    setChannels((v) => (v.includes(ch) ? v.filter((x) => x !== ch) : [...v, ch]));
    // Channels affect reach; an old preview would be misleading.
    setPreview(null);
  }

  function audiencePayload() {
    return {
      audience,
      promocodeIds: audience === "promocode" ? selectedPromocodeIds : [],
      promoFilters: audience === "promocode" ? promoFilters : [],
      modeIds: audience === "mode" ? selectedModeIds : [],
      modeFilter: audience === "mode" ? modeFilter : undefined,
      periodFrom: periodFrom || undefined,
      periodTo: periodTo || undefined,
      excludeUserIds: excludedIds,
    };
  }

  function validateAudience(): string {
    if (audience === "promocode" && selectedPromocodeIds.length === 0) return "Выберите хотя бы один промокод";
    if (audience === "mode" && selectedModeIds.length === 0) return "Выберите хотя бы один режим";
    return "";
  }

  async function loadPreview() {
    setError("");
    const invalid = validateAudience();
    if (invalid) {
      setError(invalid);
      return;
    }
    setPreviewLoading(true);
    try {
      const res = await apiFetch<PreviewData & { ok: boolean }>("/api/admin/notifications/preview", {
        method: "POST",
        body: JSON.stringify({ ...audiencePayload(), channels }),
      });
      setPreview(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось получить предпросмотр");
    } finally {
      setPreviewLoading(false);
    }
  }

  async function send(e: React.FormEvent) {
    e.preventDefault();
    setNotice("");
    setError("");
    if (!title.trim() || !body.trim()) {
      setError("Заголовок и текст обязательны");
      return;
    }
    const invalid = validateAudience();
    if (invalid) {
      setError(invalid);
      return;
    }
    if (channels.length === 0) {
      setError("Выберите хотя бы один канал");
      return;
    }
    if (textLength > textLimit) {
      setError(`Текст слишком длинный для выбранных каналов: лимит ${textLimit} символов`);
      return;
    }
    // Two-step sending: the first click shows a preview of the message,
    // the second actually sends it.
    if (!confirmStage) {
      setConfirmStage(true);
      return;
    }
    setConfirmStage(false);
    setSending(true);
    try {
      const res = await apiFetch<{ ok: boolean; recipientCount: number; delivered: Record<string, number> }>(
        "/api/admin/notifications/send",
        {
          method: "POST",
          body: JSON.stringify({
            ...audiencePayload(),
            title: title.trim(),
            body: body.trim(),
            channels,
            secondChannelDelayHours: channels.includes("max") && channels.includes("telegram")
              ? Math.max(0, Number(secondDelayHours) || 0)
              : undefined,
          }),
        }
      );
      const parts = Object.entries(res.delivered || {})
        .map(([ch, n]) => `${CHANNEL_LABELS[ch as Channel] ?? ch}: ${n}`)
        .join(", ");
      setNotice(`Отправлено (${res.recipientCount} получателей) — ${parts}`);
      setTitle("");
      setBody("");
      setExcludedIds([]);
      setPreview(null);
      loadHistory(0, historyFrom, historyTo);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Не удалось отправить");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="ms-admin-stack">
      <div className="card ms-admin-card">
        <div className="ms-admin-card-head">
          <h2>Уведомления</h2>
        </div>
        {error && <div className="ms-error-box" style={{ marginBottom: 12 }}>{error}</div>}
        {notice && <div className="ms-success-box" style={{ marginBottom: 12 }}>{notice}</div>}

        <form onSubmit={send} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {/* Audience */}
          <div>
            <div className="ms-field-label">Аудитория</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              {(["all", "admins", "promocode", "mode"] as Audience[]).map((a) => (
                <label key={a} style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", padding: "6px 12px", borderRadius: 8, border: `1px solid ${audience === a ? "var(--accent)" : "var(--line)"}`, background: audience === a ? "var(--soft)" : "var(--card)", fontSize: 13, fontWeight: 600 }}>
                  <input type="radio" name="audience" value={a} checked={audience === a} onChange={() => setAudience(a)} style={{ display: "none" }} />
                  {a === "all" ? "Все" : a === "admins" ? "Админы (тест)" : a === "promocode" ? "По промокодам" : "По режимам"}
                </label>
              ))}
            </div>
          </div>

          {/* Promo codes */}
          {audience === "promocode" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <div>
                <div className="ms-field-label">Активность получивших промокод</div>
                <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  {([["not_wrote", "Получили, но не писали"], ["wrote", "Получили и писали"]] as [PromoFilter, string][]).map(([val, label]) => (
                    <label key={val} style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", padding: "6px 12px", borderRadius: 8, border: `1px solid ${promoFilters.includes(val) ? "var(--accent)" : "var(--line)"}`, background: promoFilters.includes(val) ? "var(--soft)" : "var(--card)", fontSize: 13 }}>
                      <input type="checkbox" checked={promoFilters.includes(val)} onChange={() => { setPromoFilters((v) => v.includes(val) ? v.filter((x) => x !== val) : [...v, val]); setPreview(null); }} style={{ display: "none" }} />
                      {label}
                    </label>
                  ))}
                </div>
                <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>Группы не пересекаются: можно выбрать одну или обе (обе = все с промокодом).</div>
              </div>
              <div>
                <div className="ms-field-label">Период активации промокода (необязательно)</div>
                <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
                  <input type="date" value={periodFrom.slice(0, 10)} onChange={(e) => setPeriodFrom(e.target.value ? e.target.value + "T00:00:00Z" : "")} style={{ width: 150 }} />
                  <span className="muted">—</span>
                  <input type="date" value={periodTo.slice(0, 10)} onChange={(e) => setPeriodTo(e.target.value ? e.target.value + "T23:59:59Z" : "")} style={{ width: 150 }} />
                </div>
              </div>
            </div>
          )}
          {audience === "promocode" && (() => {
            const q = promoSearch.trim().toLowerCase();
            const filtered = allPromocodes.filter((p) =>
              !q || p.code.toLowerCase().includes(q) || (p.comment || "").toLowerCase().includes(q) || String(p.id) === q
            );
            const pages = Math.max(1, Math.ceil(filtered.length / LIST_PAGE_SIZE));
            const page = Math.min(promoPage, pages);
            const slice = filtered.slice((page - 1) * LIST_PAGE_SIZE, page * LIST_PAGE_SIZE);
            return (
              <div>
                <div className="ms-field-label">Промокоды</div>
                <input
                  value={promoSearch}
                  onChange={(e) => { setPromoSearch(e.target.value); setPromoPage(1); }}
                  placeholder="Поиск по коду, комментарию или ID"
                  style={{ marginBottom: 8 }}
                />
                <div className="ms-checkbox-list" style={{ maxHeight: 240, overflow: "auto" }}>
                  {slice.map((p) => (
                    <label key={p.id}>
                      <input
                        type="checkbox"
                        checked={selectedPromocodeIds.includes(p.id)}
                        onChange={() => setSelectedPromocodeIds((v) => toggleId(v, p.id))}
                      />
                      <span>#{p.id} {p.code}{p.comment ? ` · ${p.comment}` : ""}</span>
                    </label>
                  ))}
                  {!filtered.length && <div className="muted" style={{ padding: 8, fontSize: 12 }}>Ничего не найдено</div>}
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 4 }}>
                  <div style={{ fontSize: 11, color: "var(--muted)" }}>Выбрано: {selectedPromocodeIds.length} · Найдено: {filtered.length}</div>
                  {pages > 1 && (
                    <div style={{ display: "flex", gap: 6, alignItems: "center", fontSize: 12 }}>
                      <button type="button" className="ms-button ms-button-xs" disabled={page <= 1} onClick={() => setPromoPage(page - 1)}>‹</button>
                      <span className="muted">{page}/{pages}</span>
                      <button type="button" className="ms-button ms-button-xs" disabled={page >= pages} onClick={() => setPromoPage(page + 1)}>›</button>
                    </div>
                  )}
                </div>
              </div>
            );
          })()}

          {/* Modes */}
          {audience === "mode" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <div>
                <div className="ms-field-label">Фильтр по активности</div>
                <div style={{ display: "flex", gap: 8 }}>
                  {([["wrote", "Реально писал"], ["has", "Просто есть режим"]] as [ModeFilter, string][]).map(([val, label]) => (
                    <label key={val} style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", padding: "6px 12px", borderRadius: 8, border: `1px solid ${modeFilter === val ? "var(--accent)" : "var(--line)"}`, background: modeFilter === val ? "var(--soft)" : "var(--card)", fontSize: 13 }}>
                      <input type="radio" name="modeFilter" value={val} checked={modeFilter === val} onChange={() => setModeFilter(val)} style={{ display: "none" }} />
                      {label}
                    </label>
                  ))}
                </div>
              </div>
              <div>
                <div className="ms-field-label">Период {modeFilter === "wrote" ? "отправки сообщений" : "наличия доступа"} (необязательно)</div>
                <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
                  <input type="date" value={periodFrom} onChange={(e) => setPeriodFrom(e.target.value ? e.target.value + "T00:00:00Z" : "")} placeholder="С" style={{ width: 150 }} />
                  <span className="muted">—</span>
                  <input type="date" value={periodTo} onChange={(e) => setPeriodTo(e.target.value ? e.target.value + "T23:59:59Z" : "")} placeholder="По" style={{ width: 150 }} />
                </div>
              </div>
              {(() => {
                const q = modeSearch.trim().toLowerCase();
                const filtered = allModes.filter((m) => !q || m.name.toLowerCase().includes(q) || String(m.id) === q);
                const pages = Math.max(1, Math.ceil(filtered.length / LIST_PAGE_SIZE));
                const page = Math.min(modePage, pages);
                const slice = filtered.slice((page - 1) * LIST_PAGE_SIZE, page * LIST_PAGE_SIZE);
                return (
                  <div>
                    <div className="ms-field-label">Режимы</div>
                    <input
                      value={modeSearch}
                      onChange={(e) => { setModeSearch(e.target.value); setModePage(1); }}
                      placeholder="Поиск по названию или ID"
                      style={{ marginBottom: 8 }}
                    />
                    <div className="ms-checkbox-list" style={{ maxHeight: 240, overflow: "auto" }}>
                      {slice.map((m) => (
                        <label key={m.id}>
                          <input
                            type="checkbox"
                            checked={selectedModeIds.includes(m.id)}
                            onChange={() => setSelectedModeIds((v) => toggleId(v, m.id))}
                          />
                          <span>#{m.id} {m.name}</span>
                        </label>
                      ))}
                      {!filtered.length && <div className="muted" style={{ padding: 8, fontSize: 12 }}>Ничего не найдено</div>}
                    </div>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 4 }}>
                      <div style={{ fontSize: 11, color: "var(--muted)" }}>Выбрано: {selectedModeIds.length} · Найдено: {filtered.length}</div>
                      {pages > 1 && (
                        <div style={{ display: "flex", gap: 6, alignItems: "center", fontSize: 12 }}>
                          <button type="button" className="ms-button ms-button-xs" disabled={page <= 1} onClick={() => setModePage(page - 1)}>‹</button>
                          <span className="muted">{page}/{pages}</span>
                          <button type="button" className="ms-button ms-button-xs" disabled={page >= pages} onClick={() => setModePage(page + 1)}>›</button>
                        </div>
                      )}
                    </div>
                  </div>
                );
              })()}
            </div>
          )}

          {/* Delivery channels */}
          <div>
            <div className="ms-field-label">Каналы доставки</div>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              {/* "Site (inbox)" is archived: not shown in the choice, but CHANNEL_LABELS/the type are kept for the history of past broadcasts */}
              {(Object.keys(CHANNEL_LABELS) as Channel[]).filter((ch) => ch !== "inbox").map((ch) => (
                <label key={ch} style={{ display: "flex", alignItems: "center", gap: 6, cursor: "pointer", padding: "6px 12px", borderRadius: 8, border: `1px solid ${channels.includes(ch) ? "var(--accent)" : "var(--line)"}`, background: channels.includes(ch) ? "var(--soft)" : "var(--card)", fontSize: 13, fontWeight: 600 }}>
                  <input type="checkbox" checked={channels.includes(ch)} onChange={() => toggleChannel(ch)} style={{ display: "none" }} />
                  {CHANNEL_LABELS[ch]}
                </label>
              ))}
            </div>
            <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 4 }}>
              Max и Telegram получат только пользователи, привязавшие мессенджер. Markdown поддерживается во всех каналах (**жирный**, [ссылка](url)).
            </div>
            {channels.includes("max") && channels.includes("telegram") && (
              <div style={{ display: "flex", gap: 8, alignItems: "center", marginTop: 8, fontSize: 12, flexWrap: "wrap" }}>
                <span>Если привязаны оба мессенджера — во второй слать через</span>
                <input
                  type="number"
                  min={0}
                  step={1}
                  value={secondDelayHours}
                  onChange={(e) => setSecondDelayHours(e.target.value)}
                  style={{ width: 70 }}
                />
                <span>ч. (0 — сразу в оба)</span>
              </div>
            )}
          </div>

          {/* Text */}
          <div>
            <div className="ms-field-label">Заголовок</div>
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Заголовок уведомления" />
          </div>
          <div>
            <div className="ms-field-label">Текст</div>
            <textarea rows={5} value={body} onChange={(e) => { setBody(e.target.value); setConfirmStage(false); }} placeholder="Текст уведомления (поддерживается **жирный** и [ссылка](url))" maxLength={textLimit} />
            <div style={{ fontSize: 11, marginTop: 4, color: textLength > textLimit ? "var(--danger, #C0392B)" : "var(--muted)" }}>
              {textLength}/{textLimit} символов (лимит самого строгого из выбранных каналов{channels.includes("max") ? "; Max: 4000" : ""}{channels.includes("telegram") ? "; Telegram: 4096" : ""})
            </div>
          </div>

          {/* Message preview before confirming the send */}
          {confirmStage && (
            <div style={{ padding: 14, border: "1.5px solid var(--accent)", borderRadius: 12, background: "var(--soft)" }}>
              <div style={{ fontSize: 12, fontWeight: 700, marginBottom: 8, color: "var(--accent-strong)" }}>Так будет выглядеть сообщение — проверьте перед отправкой:</div>
              <div style={{ fontSize: 14, fontWeight: 700, marginBottom: 6 }}>{title.trim()}</div>
              <div style={{ fontSize: 13, lineHeight: 1.5 }}>
                <MessageContent content={body.trim()} />
              </div>
            </div>
          )}

          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <button
              type="submit"
              className="ms-button ms-button-primary ms-button-xs"
              disabled={sending}
            >
              {sending ? "Отправляем…" : confirmStage ? "Подтвердить отправку" : "Отправить"}
            </button>
            {confirmStage && (
              <button type="button" className="ms-button ms-button-xs" onClick={() => setConfirmStage(false)}>
                Отменить
              </button>
            )}
            <button
              type="button"
              className="ms-button ms-button-xs"
              disabled={previewLoading}
              onClick={loadPreview}
            >
              {previewLoading ? "Считаем…" : "Предпросмотр получателей"}
            </button>
          </div>
        </form>

        {/* Audience preview */}
        {preview && (
          <div style={{ marginTop: 16, padding: 14, border: "1px solid var(--line)", borderRadius: 12 }}>
            <div style={{ fontWeight: 700, fontSize: 13, marginBottom: 8 }}>
              Получателей: {preview.recipientCount}
            </div>
            <div className="muted" style={{ fontSize: 12, marginBottom: 10 }}>
              С почтой: {preview.emailCount} · С телефоном: {preview.phoneCount} · Max: {preview.maxLinkedCount} · Telegram: {preview.telegramLinkedCount}
            </div>
            <div className="muted" style={{ fontSize: 11, marginBottom: 6 }}>
              Галочка «не слать» исключает получателя из этой рассылки{excludedIds.length > 0 ? ` (исключено: ${excludedIds.length})` : ""}.
            </div>
            <div style={{ maxHeight: 260, overflow: "auto", display: "grid", gap: 4 }}>
              {preview.recipients.map((r) => {
                const excluded = excludedIds.includes(r.id);
                return (
                  <div key={r.id} style={{ display: "flex", gap: 8, alignItems: "baseline", fontSize: 12, padding: "4px 8px", borderRadius: 6, background: "var(--soft)", flexWrap: "wrap", opacity: excluded ? 0.5 : 1 }}>
                    <label style={{ display: "flex", alignItems: "center", gap: 4, cursor: "pointer", whiteSpace: "nowrap" }}>
                      <input
                        type="checkbox"
                        checked={excluded}
                        onChange={() => setExcludedIds((v) => excluded ? v.filter((x) => x !== r.id) : [...v, r.id])}
                      />
                      <span className="muted">не слать</span>
                    </label>
                    <span style={{ fontWeight: 700 }}>#{r.id}</span>
                    <span>{r.email || <span className="muted">без почты</span>}</span>
                    {r.phone && <span>{r.phone}</span>}
                    {r.maxLinked && <span style={{ color: "var(--accent-strong)", fontWeight: 600 }}>Max</span>}
                    {r.telegramLinked && <span style={{ color: "var(--accent-strong)", fontWeight: 600 }}>TG</span>}
                  </div>
                );
              })}
            </div>
            {preview.previewTruncated && (
              <div className="muted" style={{ fontSize: 11, marginTop: 6 }}>Показаны первые {preview.recipients.length} из {preview.recipientCount}</div>
            )}
          </div>
        )}
      </div>

      {/* Subscription bonus settings */}
      <div className="card ms-admin-card">
        <div className="ms-admin-card-head">
          <h2>Бонус за подписку на уведомления</h2>
        </div>
        {bonusNotice && <div className="ms-success-box" style={{ marginBottom: 10 }}>{bonusNotice}</div>}
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap", fontSize: 13 }}>
          <span>Начислять</span>
          <input
            type="number"
            min={0}
            step={1}
            value={bonusSetting}
            onChange={(e) => setBonusSetting(e.target.value)}
            placeholder="авто"
            style={{ width: 90 }}
          />
          <span>сообщений к дневному лимиту за каждое первое подключение канала (Max / Telegram / пуши / сайт).</span>
          <button type="button" className="ms-button ms-button-xs" disabled={bonusSaving} onClick={saveBonusSetting}>
            {bonusSaving ? "Сохраняем…" : "Сохранить"}
          </button>
        </div>
        <div className="muted" style={{ fontSize: 11, marginTop: 6 }}>
          Пусто или 0 — авто-формула: 10% от минимального текущего лимита пользователя, минимум 2. Повторное подключение бонус не даёт.
        </div>
      </div>

      {/* Send history */}
      <div className="card ms-admin-card">
        <div className="ms-admin-card-head">
          <h2>История отправок</h2>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 12 }}>
          <input type="date" value={historyFrom} onChange={(e) => setHistoryFrom(e.target.value)} style={{ width: 150 }} />
          <span className="muted">—</span>
          <input type="date" value={historyTo} onChange={(e) => setHistoryTo(e.target.value)} style={{ width: 150 }} />
          <button type="button" className="ms-button ms-button-xs" onClick={() => loadHistory(0, historyFrom, historyTo)}>Показать</button>
          {(historyFrom || historyTo) && (
            <button type="button" className="ms-button ms-button-xs" onClick={() => { setHistoryFrom(""); setHistoryTo(""); loadHistory(0, "", ""); }}>Сбросить</button>
          )}
        </div>
        {history.length === 0 ? (
          <div className="muted" style={{ fontSize: 13 }}>Пока не было рассылок</div>
        ) : (
          <div style={{ display: "grid", gap: 8 }}>
            {history.map((item) => (
              <div
                key={item.id}
                onClick={() => setExpandedHistoryId((v) => (v === item.id ? null : item.id))}
                style={{ padding: "10px 12px", border: "1px solid var(--line)", borderRadius: 10, cursor: "pointer" }}
              >
                <div style={{ display: "flex", justifyContent: "space-between", gap: 10, flexWrap: "wrap", alignItems: "baseline" }}>
                  <div style={{ fontSize: 13, fontWeight: 700 }}>{item.title}</div>
                  <div className="muted" style={{ fontSize: 11, whiteSpace: "nowrap" }}>
                    {new Date(item.createdAt).toLocaleString("ru-RU")}
                  </div>
                </div>
                {expandedHistoryId === item.id ? (
                  <div style={{ fontSize: 13, marginTop: 6, lineHeight: 1.5 }}>
                    <MessageContent content={item.body} />
                  </div>
                ) : (
                  <div className="muted" style={{ fontSize: 12, marginTop: 4, whiteSpace: "pre-wrap", overflow: "hidden", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical" }}>
                    {item.body}
                  </div>
                )}
                <div className="muted" style={{ fontSize: 11, marginTop: 6, display: "flex", gap: 10, flexWrap: "wrap" }}>
                  <span>{item.actorEmail || `admin #${item.id}`}</span>
                  <span>аудитория: {item.audience === "all" ? "все" : item.audience === "promocode" ? "промокоды" : "режимы"}</span>
                  <span>каналы: {(item.channels || []).map((ch) => CHANNEL_LABELS[ch as Channel] ?? ch).join(", ")}</span>
                  <span>получателей: {item.recipientCount}</span>
                  {item.delivered && (
                    <span>
                      доставлено: {Object.entries(item.delivered).map(([ch, n]) => `${CHANNEL_LABELS[ch as Channel] ?? ch} ${n}`).join(", ")}
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
        {historyTotal > HISTORY_PAGE_SIZE && (
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginTop: 10, fontSize: 12 }}>
            <span className="muted">
              {historyOffset + 1}–{Math.min(historyOffset + HISTORY_PAGE_SIZE, historyTotal)} из {historyTotal}
            </span>
            <div style={{ display: "flex", gap: 6 }}>
              <button type="button" className="ms-button ms-button-xs" disabled={historyOffset <= 0} onClick={() => loadHistory(Math.max(0, historyOffset - HISTORY_PAGE_SIZE), historyFrom, historyTo)}>‹ Новее</button>
              <button type="button" className="ms-button ms-button-xs" disabled={historyOffset + HISTORY_PAGE_SIZE >= historyTotal} onClick={() => loadHistory(historyOffset + HISTORY_PAGE_SIZE, historyFrom, historyTo)}>Старше ›</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
