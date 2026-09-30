"use client";

import { MS } from "../_theme";
import { formatBytes, formatDate } from "../_utils";
import { QRCard } from "../_components/QRCard";
import type { HistoryItem } from "../_types";

type HistoryListProps = {
  promo: string;
  history: HistoryItem[];
  setResult: React.Dispatch<React.SetStateAction<string>>;
};

export function HistoryList({ promo, history, setResult }: HistoryListProps) {
  return (
    <>
          {/* ── RIGHT ── */}
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <QRCard promo={promo} />

            {/* History */}
            <div
              style={{
                background: MS.surface,
                border: `1px solid ${MS.ink10}`,
                borderRadius: 14,
                padding: "18px 20px",
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 10 }}>
                История резюмирований
              </div>
              {history.length === 0 ? (
                <div style={{ fontSize: 12, color: MS.ink50 }}>
                  Здесь будут сохранённые резюме этого промокода — нажмите на запись, чтобы
                  открыть её результат снова даже после обновления страницы.
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                  {history.map((h, i) => (
                    <button
                      key={`${h.summaryId}-${i}`}
                      type="button"
                      onClick={() => setResult(String(h.result || ""))}
                      style={{
                        textAlign: "left",
                        padding: "10px 12px",
                        borderRadius: 10,
                        border: `1px solid ${MS.ink10}`,
                        background: MS.surfaceSoft,
                        cursor: "pointer",
                        fontFamily: "inherit",
                        display: "flex",
                        flexDirection: "column",
                        gap: 4,
                      }}
                    >
                      <div
                        style={{
                          display: "flex",
                          justifyContent: "space-between",
                          gap: 8,
                        }}
                      >
                        <span
                          style={{
                            fontSize: 12,
                            fontWeight: 500,
                            color: MS.ink,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {h.modeLabel}
                        </span>
                        <span
                          style={{
                            fontSize: 11,
                            color: MS.ink50,
                            fontVariantNumeric: "tabular-nums",
                            flexShrink: 0,
                          }}
                        >
                          {formatDate(h.createdAt)}
                        </span>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: MS.ink50,
                          fontVariantNumeric: "tabular-nums",
                        }}
                      >
                        {h.messageCount.toLocaleString("ru-RU")} сообщений ·{" "}
                        {formatBytes(h.sourceBytes)} · ~
                        {h.approxTokens.toLocaleString("ru-RU")} токенов
                      </div>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
    </>
  );
}
