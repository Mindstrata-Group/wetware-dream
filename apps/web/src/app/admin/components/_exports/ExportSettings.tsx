"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function ExportSettings() {
  const {
    exportSort, summaryPrompts, exportResult, exportSummaryResult, setExportSummaryResult,
    exportSummaryHistory, exportMeta, exportFilters, setExportFilters, filteredUsers,
    filteredPromos, promoModeSet, filteredExportModes, sortButton, toggleExportSort,
    exportMessages, summarizeExport, downloadTxt, formatDate, promoStateLabel,
    MessageContent, toggleNumberSelection, ExportLimitSlider,
  } = useAdminPageContext();

  return (
    <>
{/* ── EXPORT SETTINGS ── */}
<div
  style={{
    display: "grid",
    gridTemplateColumns: "1fr 1fr",
    gap: 12,
  }}
  className="ms-export-settings-grid"
>
  <label>
    Что выгружать
    <select
      value={exportFilters.roleFilter}
      onChange={(e) =>
        setExportFilters((v) => ({
          ...v,
          roleFilter: e.target.value,
        }))
      }
    >
      <option value="all">Весь чат: пользователь + ИИ</option>
      <option value="assistant">Только ответы ИИ</option>
      <option value="user">Только вопросы пользователя</option>
    </select>
  </label>

  <ExportLimitSlider
    value={exportFilters.limit}
    onCommit={(limit) =>
      setExportFilters((v) => ({ ...v, limit }))
    }
  />
</div>

<div
  style={{
    display: "grid",
    gridTemplateColumns: "1fr 1fr",
    gap: 12,
  }}
  className="ms-export-settings-grid"
>
  <label>
    Дата от
    <input
      type="date"
      value={exportFilters.dateFrom}
      onChange={(e) =>
        setExportFilters((v) => ({
          ...v,
          dateFrom: e.target.value,
        }))
      }
    />
  </label>
  <label>
    Дата до
    <input
      type="date"
      value={exportFilters.dateTo}
      onChange={(e) =>
        setExportFilters((v) => ({
          ...v,
          dateTo: e.target.value,
        }))
      }
    />
  </label>
</div>

{/* ── SUMMARISATION PROMPT ── */}
<label className="ms-inline-check">
  <input
    type="checkbox"
    checked={exportFilters.withSummary}
    onChange={(e) =>
      setExportFilters((v) => ({
        ...v,
        withSummary: e.target.checked,
      }))
    }
  />{" "}
  С резюмированием
</label>

<label>
  Шаблон промпта
  <select
    value={exportFilters.promptId}
    onChange={(e) => {
      const p = summaryPrompts.find(
        (x) => String(x.id) === e.target.value
      );
      setExportFilters((v) => ({
        ...v,
        promptId: e.target.value,
        customPrompt: p?.prompt || v.customPrompt,
      }));
    }}
  >
    <option value="">Без базы промптов</option>
    {summaryPrompts.map((p) => (
      <option key={p.id} value={p.id}>
        {p.name}
        {p.isDefault ? " · по умолчанию" : ""}
      </option>
    ))}
  </select>
</label>

<label>
  Текст промпта (можно редактировать)
  <textarea
    rows={5}
    value={exportFilters.customPrompt}
    onChange={(e) =>
      setExportFilters((v) => ({
        ...v,
        customPrompt: e.target.value,
      }))
    }
  />
</label>

{/* ── ACTION BUTTONS ── */}
<div
  style={{
    display: "flex",
    flexWrap: "wrap",
    gap: 10,
  }}
  className="ms-export-actions-grid"
>
  <button
    type="button"
    className="ms-button ms-button-primary ms-button-xs"
    onClick={exportMessages}
  >
    Собрать экспорт
  </button>
  <button
    type="button"
    className="ms-button ms-button-primary ms-button-xs"
    onClick={summarizeExport}
  >
    Начать резюмирование
  </button>
  <button
    type="button"
    className="ms-button ms-button-ghost ms-button-xs"
    onClick={downloadTxt}
  >
    Скачать TXT UTF-8
  </button>
</div>

{/* ── COUNTERS ── */}
<div className="ms-stat-grid">
  <div className="ms-stat-card">
    <span>Сообщений</span>
    <strong>
      {exportMeta.messageCount || exportResult.length}
    </strong>
  </div>
  <div className="ms-stat-card">
    <span>Размер TXT</span>
    <strong>
      {Math.ceil((exportMeta.sourceBytes || 0) / 1024)} КБ
    </strong>
  </div>
  <div className="ms-stat-card">
    <span>Примерно токенов</span>
    <strong>
      {(exportMeta.approxTokens || 0).toLocaleString("ru-RU")}
    </strong>
  </div>
</div>

    </>
  );
}
