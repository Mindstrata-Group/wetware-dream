"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function ResultsView() {
  const {
    exportSort, summaryPrompts, exportResult, exportSummaryResult, setExportSummaryResult,
    exportSummaryHistory, exportMeta, exportFilters, setExportFilters, filteredUsers,
    filteredPromos, promoModeSet, filteredExportModes, sortButton, toggleExportSort,
    exportMessages, summarizeExport, downloadTxt, formatDate, promoStateLabel,
    MessageContent, toggleNumberSelection, ExportLimitSlider,
  } = useAdminPageContext();

  return (
    <>
{/* ── SUMMARISATION RESULT ── */}
{exportSummaryResult && (
  <div className="ms-info-box ms-markdown-result">
    <strong style={{ fontSize: 14 }}>
      Ответ нейросети по выгрузке
    </strong>
    <MessageContent content={exportSummaryResult} />
  </div>
)}

{/* ── SUMMARISATION HISTORY ── */}
{exportSummaryHistory.length > 0 && (
  <div className="ms-admin-list">
    <div
      style={{
        fontSize: 14,
        padding: "4px 0",
      }}
    >
      История последних резюмирований
    </div>
    {exportSummaryHistory.map((item, i) => (
      <button
        key={`${item.summaryId || i}`}
        type="button"
        className="ms-info-box ms-summary-history-item"
        onClick={() =>
          setExportSummaryResult(String(item.result || ""))
        }
      >
        <strong>
          {item.createdAt ? formatDate(String(item.createdAt)) : "только что"}{" "}
          · {item.messageCount} сообщений · ~{item.approxTokens}{" "}
          токенов
        </strong>
        <span>{String(item.result || "").slice(0, 300)}</span>
      </button>
    ))}
  </div>
)}

{/* ── MESSAGE PREVIEW ── */}
{exportResult.length > 0 && (
  <div className="ms-admin-list">
    <div
      style={{
        fontSize: 14,
        padding: "4px 0",
      }}
    >
      Превью (первые 20 из {exportResult.length})
    </div>
    {exportResult.slice(0, 20).map((m, i) => (
      <div key={i} className="ms-info-box">
        <strong style={{ fontSize: 12 }}>
          {formatDate(String(m.createdAt))} · {m.modeName} ·{" "}
          <span
            style={{
              color:
                m.role === "assistant"
                  ? "var(--accent-strong)"
                  : "inherit",
            }}
          >
            {m.role}
          </span>
        </strong>
        <span style={{ fontSize: 13 }}>
          {String(m.content).slice(0, 240)}
          {String(m.content).length > 240 ? "…" : ""}
        </span>
      </div>
    ))}
  </div>
)}
    </>
  );
}
