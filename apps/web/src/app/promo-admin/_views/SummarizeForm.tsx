"use client";

import { MessageContent } from "@/components/MessageContent";
import { MS, FONT_BODY } from "../_theme";
import { formatDate, ghostBtnStyle } from "../_utils";
import type { HistoryItem, ModeOption, PromoStatus, PromptOption } from "../_types";

type SummarizeFormProps = {
  modes: ModeOption[];
  modeIds: number[];
  setModeIds: React.Dispatch<React.SetStateAction<number[]>>;
  prompts: PromptOption[];
  promptId: number | null;
  setPromptId: React.Dispatch<React.SetStateAction<number | null>>;
  summarize: () => void;
  loadingSummary: boolean;
  status: PromoStatus;
  copyAccessUrl: () => void;
  mobile: boolean;
  result: string;
  history: HistoryItem[];
  downloadResult: () => void;
};

export function SummarizeForm({ modes, modeIds, setModeIds, prompts, promptId, setPromptId, summarize, loadingSummary, status, copyAccessUrl, mobile, result, history, downloadResult }: SummarizeFormProps) {
  const messageCount = status.messageCount ?? 0;
  const activationsWithoutMessages = status.activationsWithoutMessages ?? 0;
  const cannotSummarize = messageCount <= 0;
  const summarizeDisabled = loadingSummary || !promptId || cannotSummarize;
  return (
    <>
          {/* ── LEFT ── */}
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            {/* Modes */}
            <div
              style={{
                background: MS.surface,
                border: `1px solid ${MS.ink10}`,
                borderRadius: 14,
                padding: "18px 20px",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
                Доступные режимы
              </div>
              <div
                style={{
                  fontSize: 12,
                  color: MS.ink50,
                  marginBottom: 10,
                  lineHeight: 1.5,
                }}
              >
                Нажмите на карточки, чтобы ограничить резюме конкретными режимами. Если
                ничего не выбрано — берём все режимы промокода.
              </div>
              <div style={{ display: "flex", gap: 6, marginBottom: 10 }}>
                <button
                  type="button"
                  onClick={() => setModeIds(modes.map((m) => m.id))}
                  style={ghostBtnStyle("sm")}
                >
                  Выбрать все
                </button>
                <button
                  type="button"
                  onClick={() => setModeIds([])}
                  style={ghostBtnStyle("sm")}
                >
                  Убрать все
                </button>
              </div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                {modes.length === 0 && (
                  <span style={{ fontSize: 12, color: MS.ink50 }}>
                    У промокода пока нет привязанных режимов.
                  </span>
                )}
                {modes.map((m) => {
                  const on = modeIds.includes(m.id);
                  return (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() =>
                        setModeIds((ids) =>
                          on ? ids.filter((id) => id !== m.id) : [...ids, m.id],
                        )
                      }
                      style={{
                        height: 32,
                        padding: "0 12px",
                        borderRadius: 999,
                        fontSize: 13,
                        fontWeight: 500,
                        cursor: "pointer",
                        fontFamily: "inherit",
                        border: `1px solid ${on ? MS.green : MS.ink20}`,
                        background: on ? MS.greenLight : MS.surface,
                        color: on ? MS.greenDark : MS.ink70,
                        transition: "all 0.15s",
                      }}
                    >
                      {m.name}
                    </button>
                  );
                })}
              </div>
            </div>

            {/* Prompt template */}
            <div
              style={{
                background: MS.surface,
                border: `1px solid ${MS.ink10}`,
                borderRadius: 14,
                padding: "18px 20px",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
                Режим резюмирования
              </div>
              <div style={{ fontSize: 12, color: MS.ink50, marginBottom: 10 }}>
                Выберите шаблон — он задаёт структуру и стиль Markdown-резюме.
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {prompts.length === 0 && (
                  <span style={{ fontSize: 12, color: MS.ink50 }}>
                    Шаблоны резюмирования не настроены.
                  </span>
                )}
                {prompts.map((p) => {
                  const on = p.id === promptId;
                  return (
                    <button
                      key={p.id}
                      type="button"
                      onClick={() => setPromptId(p.id)}
                      style={{
                        textAlign: "left",
                        padding: "10px 12px",
                        borderRadius: 10,
                        cursor: "pointer",
                        fontFamily: "inherit",
                        border: `1px solid ${on ? MS.green : MS.ink10}`,
                        background: on ? MS.greenLight : MS.surface,
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "center",
                        gap: 8,
                        transition: "all 0.15s",
                      }}
                    >
                      <span
                        style={{
                          fontSize: 13,
                          fontWeight: on ? 500 : 400,
                          color: on ? MS.greenDark : MS.ink,
                        }}
                      >
                        {p.name}
                        {p.isDefault && (
                          <span
                            style={{ fontSize: 11, color: MS.ink50, fontWeight: 400 }}
                          >
                            {" · по умолчанию"}
                          </span>
                        )}
                      </span>
                      {on && <span style={{ color: MS.green, fontSize: 16 }}>✓</span>}
                    </button>
                  );
                })}
              </div>
            </div>

            <div
              style={{
                background: cannotSummarize ? MS.warnLight : MS.greenLight,
                border: `1px solid ${cannotSummarize ? "rgba(196,69,69,0.25)" : "rgba(29,158,117,0.25)"}`,
                borderRadius: 14,
                padding: "14px 16px",
                fontSize: 13,
                color: cannotSummarize ? "#7E2A2A" : MS.greenDark,
                lineHeight: 1.5,
              }}
            >
              <div style={{ fontWeight: 600, marginBottom: 4 }}>
                Данные для резюмирования
              </div>
              <div>
                Уже есть сообщений: {messageCount.toLocaleString("ru-RU")}.
              </div>
              <div>
                Активаций без первого сообщения: {activationsWithoutMessages.toLocaleString("ru-RU")}.
              </div>
              {cannotSummarize && (
                <div style={{ marginTop: 6, fontWeight: 500 }}>
                  Резюмирование недоступно: пока нечего резюмировать.
                </div>
              )}
            </div>

            {/* Actions */}
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              <button
                type="button"
                onClick={summarize}
                disabled={summarizeDisabled}
                className="ms-button ms-button-primary ms-button-xs"
                style={{
                  flex: mobile ? 1 : "0 1 auto",
                  minWidth: mobile ? "100%" : 220,
                  opacity: summarizeDisabled ? 0.6 : 1,
                  cursor: summarizeDisabled ? "not-allowed" : "pointer",
                }}
              >
                {loadingSummary ? "Резюмируем…" : "Начать резюмирование"}
              </button>
              <button
                type="button"
                onClick={copyAccessUrl}
                className="ms-button ms-button-ghost ms-button-xs"
                style={{
                  flex: mobile ? 1 : "0 1 auto",
                }}
              >
                Скопировать ссылку QR
              </button>
            </div>

            {/* Loading row */}
            {loadingSummary && (
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  padding: "12px 14px",
                  borderRadius: 12,
                  background: MS.greenLight,
                  border: "1px solid rgba(29,158,117,0.25)",
                  fontSize: 13,
                  color: MS.greenDark,
                }}
              >
                <span
                  style={{
                    width: 14,
                    height: 14,
                    borderRadius: "50%",
                    border: `2px solid ${MS.green}`,
                    borderTopColor: "transparent",
                    animation: "ms-spin 0.8s linear infinite",
                  }}
                />
                <span>Резюмирование выполняется, ожидаем ответ нейросети…</span>
              </div>
            )}

            {/* Result */}
            {result && (
              <div
                style={{
                  background: MS.surface,
                  border: `1px solid ${MS.ink10}`,
                  borderRadius: 14,
                  padding: "18px 20px",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "flex-start",
                    gap: 8,
                    marginBottom: 10,
                    flexWrap: "wrap",
                  }}
                >
                  <div>
                    <div style={{ fontSize: 13, fontWeight: 500 }}>
                      Результат резюмирования
                    </div>
                    {history[0] && (
                      <div style={{ fontSize: 11, color: MS.ink50, marginTop: 2 }}>
                        {formatDate(history[0].createdAt)} · {history[0].modeLabel} ·{" "}
                        {history[0].messageCount.toLocaleString("ru-RU")} сообщений · ~
                        {history[0].approxTokens.toLocaleString("ru-RU")} токенов
                      </div>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={downloadResult}
                    style={ghostBtnStyle("sm")}
                  >
                    Скачать MD
                  </button>
                </div>
                <div
                  style={{
                    fontFamily: FONT_BODY,
                    fontSize: 14,
                    lineHeight: 1.65,
                    color: MS.ink,
                    padding: 16,
                    background: MS.surfaceSoft,
                    borderRadius: 10,
                    border: `1px solid ${MS.ink05}`,
                  }}
                >
                  <MessageContent content={result} />
                </div>
              </div>
            )}
          </div>
    </>
  );
}
