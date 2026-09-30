"use client";

import { useEffect, useMemo, useState } from "react";
import { useAdminPageContext } from "../AdminPageContext";

const EXPORT_TABLE_PAGE_SIZE = 10;
type ExportTableKey = "modes" | "users" | "promos";

const initialOpenTables: Record<ExportTableKey, boolean> = {
  modes: false,
  users: false,
  promos: false,
};

const initialTablePages: Record<ExportTableKey, number> = {
  modes: 0,
  users: 0,
  promos: 0,
};

export function FilterColumns() {
  const {
    exportSort, exportFilters, setExportFilters, filteredUsers,
    filteredPromos, promoModeSet, filteredExportModes, sortButton, toggleExportSort,
    formatDate, promoStateLabel, toggleNumberSelection,
  } = useAdminPageContext();
  const [openTables, setOpenTables] = useState(initialOpenTables);
  const [tablePages, setTablePages] = useState(initialTablePages);

  const tableTotals = useMemo(
    () => ({
      modes: filteredExportModes.length,
      users: filteredUsers.length,
      promos: filteredPromos.length,
    }),
    [filteredExportModes.length, filteredUsers.length, filteredPromos.length],
  );

  useEffect(() => {
    setTablePages((current) => {
      let changed = false;
      const next = { ...current };
      (Object.keys(tableTotals) as ExportTableKey[]).forEach((key) => {
        const maxPage = Math.max(0, Math.ceil(tableTotals[key] / EXPORT_TABLE_PAGE_SIZE) - 1);
        if (next[key] > maxPage) {
          next[key] = maxPage;
          changed = true;
        }
      });
      return changed ? next : current;
    });
  }, [tableTotals]);

  function openLatest(key: ExportTableKey) {
    setOpenTables((current) => ({ ...current, [key]: true }));
    setTablePages((current) => ({ ...current, [key]: 0 }));
  }

  function pagedRows<T>(key: ExportTableKey, rows: T[]) {
    if (!openTables[key]) return [];
    const start = tablePages[key] * EXPORT_TABLE_PAGE_SIZE;
    return rows.slice(start, start + EXPORT_TABLE_PAGE_SIZE);
  }

  function renderClosedTableGate(key: ExportTableKey) {
    const total = tableTotals[key];
    if (!openTables[key]) {
      return (
        <div className="ms-info-box" style={{ textAlign: "center", padding: 16 }}>
          <button type="button" className="ms-button ms-button-primary ms-button-xs" onClick={() => openLatest(key)}>
            Открыть последние
          </button>
          <div style={{ marginTop: 8, color: "var(--muted)", fontSize: 12 }}>
            Найдено: {total.toLocaleString("ru-RU")}. Таблица загрузится страницами по {EXPORT_TABLE_PAGE_SIZE}.
          </div>
        </div>
      );
    }
    return null;
  }

  function renderTableGate(key: ExportTableKey, colSpan: number, emptyText: string) {
    const total = tableTotals[key];
    if (!openTables[key]) return null;
    if (total === 0) {
      return (
        <tr>
          <td colSpan={colSpan} style={{ textAlign: "center", color: "var(--muted)", padding: 16 }}>
            {emptyText}
          </td>
        </tr>
      );
    }
    return null;
  }

  function renderPagination(key: ExportTableKey) {
    if (!openTables[key] || tableTotals[key] === 0) return null;
    const totalPages = Math.max(1, Math.ceil(tableTotals[key] / EXPORT_TABLE_PAGE_SIZE));
    const page = Math.min(tablePages[key], totalPages - 1);
    const from = page * EXPORT_TABLE_PAGE_SIZE + 1;
    const to = Math.min(tableTotals[key], from + EXPORT_TABLE_PAGE_SIZE - 1);
    return (
      <div className="ms-pagination" style={{ marginTop: 8 }}>
        <span style={{ color: "var(--muted)", fontSize: 12 }}>
          {from}–{to} из {tableTotals[key].toLocaleString("ru-RU")}
        </span>
        <div style={{ display: "flex", gap: 8 }}>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={page === 0}
            onClick={() => setTablePages((current) => ({ ...current, [key]: Math.max(0, current[key] - 1) }))}
          >
            Назад
          </button>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={page >= totalPages - 1}
            onClick={() => setTablePages((current) => ({ ...current, [key]: Math.min(totalPages - 1, current[key] + 1) }))}
          >
            Вперёд
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
{/* ── MODES ── */}
<div style={{ fontSize: 13 }}>
  Режимы для выгрузки
  <span
    style={{
      fontWeight: 400,
      color: "var(--muted)",
      marginLeft: 6,
    }}
  >
    (пусто = все режимы)
  </span>
</div>

<input
  placeholder="поиск по режимам"
  value={exportFilters.modeSearch}
  onChange={(e) =>
    setExportFilters((v) => ({ ...v, modeSearch: e.target.value }))
  }
/>

<div className="ms-admin-mini-actions">
  <button
    type="button"
    onClick={() => setExportFilters((v) => ({ ...v, modeIds: [] }))}
  >
    Охватить все режимы
  </button>
  <button
    type="button"
    onClick={() => setExportFilters((v) => ({ ...v, modeIds: [] }))}
  >
    Очистить фильтр
  </button>
</div>

{promoModeSet && (
  <div
    className="ms-info-box"
    style={{ fontSize: 12, padding: "8px 12px" }}
  >
    Список режимов ограничен выбранными промокодами.
  </div>
)}

{renderClosedTableGate("modes")}
{openTables.modes && (
<div className="ms-table-scroll" style={{ maxHeight: 280 }}>
  <table className="ms-admin-table">
    <thead>
      <tr>
        <th style={{ width: 36 }}></th>
        <th>
          {sortButton(
            "#",
            "id",
            exportSort.modes || null,
            () => toggleExportSort("modes", "id")
          )}
        </th>
        <th>
          {sortButton(
            "Название",
            "name",
            exportSort.modes || null,
            () => toggleExportSort("modes", "name")
          )}
        </th>
        <th>
          {sortButton(
            "Статус",
            "hidden",
            exportSort.modes || null,
            () => toggleExportSort("modes", "hidden")
          )}
        </th>
      </tr>
    </thead>
    <tbody>
      {pagedRows("modes", filteredExportModes).map((m) => (
        <tr
          key={m.id}
          className={
            exportFilters.modeIds.includes(m.id) ? "is-active" : ""
          }
          onClick={() =>
            setExportFilters((v) => ({
              ...v,
              modeIds: toggleNumberSelection(v.modeIds, m.id),
            }))
          }
        >
          <td>
            <input
              type="checkbox"
              checked={exportFilters.modeIds.includes(m.id)}
              readOnly
            />
          </td>
          <td
            style={{
              fontFamily: "monospace",
              color: "var(--muted)",
              fontSize: 12,
            }}
          >
            #{m.id}
          </td>
          <td>{m.name}</td>
          <td
            style={{ fontSize: 13, color: "var(--muted)" }}
          >
            {m.hidden ? "скрыт" : "видим"}
          </td>
        </tr>
      ))}
      {renderTableGate("modes", 4, "Режимов не найдено")}
    </tbody>
  </table>
</div>
)}
{renderPagination("modes")}

{/* ── USERS ── */}
<div style={{ fontSize: 13, marginTop: 4 }}>
  Пользователи для выгрузки
  <span
    style={{
      fontWeight: 400,
      color: "var(--muted)",
      marginLeft: 6,
    }}
  >
    (пусто = все пользователи)
  </span>
</div>

<input
  placeholder="поиск пользователя по email/имени"
  value={exportFilters.userSearch}
  onChange={(e) =>
    setExportFilters((v) => ({ ...v, userSearch: e.target.value }))
  }
/>

<div className="ms-admin-mini-actions">
  <button
    type="button"
    onClick={() =>
      setExportFilters((v) => ({ ...v, userIds: [] }))
    }
  >
    Охватить всех
  </button>
  <button
    type="button"
    onClick={() =>
      setExportFilters((v) => ({ ...v, userIds: [] }))
    }
  >
    Очистить фильтр
  </button>
</div>

{renderClosedTableGate("users")}
{openTables.users && (
<div className="ms-table-scroll" style={{ maxHeight: 280 }}>
  <table className="ms-admin-table">
    <thead>
      <tr>
        <th style={{ width: 36 }}></th>
        <th>
          {sortButton(
            "#",
            "id",
            exportSort.users || null,
            () => toggleExportSort("users", "id")
          )}
        </th>
        <th>
          {sortButton(
            "Пользователь",
            "email",
            exportSort.users || null,
            () => toggleExportSort("users", "email")
          )}
        </th>
        <th>
          {sortButton(
            "Сообщений",
            "messageCount",
            exportSort.users || null,
            () => toggleExportSort("users", "messageCount")
          )}
        </th>
        <th>
          {sortButton(
            "Создан",
            "createdAt",
            exportSort.users || null,
            () => toggleExportSort("users", "createdAt")
          )}
        </th>
      </tr>
    </thead>
    <tbody>
      {pagedRows("users", filteredUsers).map((u) => (
        <tr
          key={u.id}
          onClick={() =>
            setExportFilters((v) => ({
              ...v,
              userIds: toggleNumberSelection(v.userIds, u.id),
            }))
          }
        >
          <td>
            <input
              type="checkbox"
              checked={exportFilters.userIds.includes(u.id)}
              readOnly
            />
          </td>
          <td
            style={{
              fontFamily: "monospace",
              color: "var(--muted)",
              fontSize: 12,
            }}
          >
            #{u.id}
          </td>
          <td
            style={{
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              maxWidth: 200,
            }}
          >
            {u.email || u.telegramUsername || "—"}
          </td>
          <td style={{ fontVariantNumeric: "tabular-nums" }}>
            {u.messageCount ?? 0}
          </td>
          <td
            style={{ color: "var(--muted)", fontSize: 12 }}
          >
            {formatDate(u.createdAt)}
          </td>
        </tr>
      ))}
      {renderTableGate("users", 5, "Пользователей не найдено")}
    </tbody>
  </table>
</div>
)}
{renderPagination("users")}

{/* ── PROMO CODES ── */}
<div style={{ fontSize: 13, marginTop: 4 }}>
  Промокоды для выгрузки
  <span
    style={{
      fontWeight: 400,
      color: "var(--muted)",
      marginLeft: 6,
    }}
  >
    (пусто = все промокоды)
  </span>
</div>

<input
  placeholder="поиск промокода"
  value={exportFilters.promocodeSearch}
  onChange={(e) =>
    setExportFilters((v) => ({
      ...v,
      promocodeSearch: e.target.value,
    }))
  }
/>

<div className="ms-admin-mini-actions">
  <button
    type="button"
    onClick={() =>
      setExportFilters((v) => ({ ...v, promocodeIds: [] }))
    }
  >
    Охватить все
  </button>
  <button
    type="button"
    onClick={() =>
      setExportFilters((v) => ({ ...v, promocodeIds: [] }))
    }
  >
    Очистить фильтр
  </button>
</div>

{renderClosedTableGate("promos")}
{openTables.promos && (
<div className="ms-table-scroll" style={{ maxHeight: 320 }}>
  <table className="ms-admin-table">
    <thead>
      <tr>
        <th style={{ width: 36 }}></th>
        <th>
          {sortButton(
            "Код",
            "code",
            exportSort.promos || null,
            () => toggleExportSort("promos", "code")
          )}
        </th>
        <th>Комментарий</th>
        <th>
          {sortButton(
            "Статус",
            "active",
            exportSort.promos || null,
            () => toggleExportSort("promos", "active")
          )}
        </th>
        <th>
          {sortButton(
            "Исп.",
            "usedCount",
            exportSort.promos || null,
            () => toggleExportSort("promos", "usedCount")
          )}
        </th>
        <th>
          {sortButton(
            "Окно до",
            "activeTo",
            exportSort.promos || null,
            () => toggleExportSort("promos", "activeTo")
          )}
        </th>
      </tr>
    </thead>
    <tbody>
      {pagedRows("promos", filteredPromos).map((p) => (
        <tr
          key={p.id}
          onClick={() =>
            setExportFilters((v) => ({
              ...v,
              promocodeIds: toggleNumberSelection(
                v.promocodeIds,
                p.id
              ),
            }))
          }
        >
          <td>
            <input
              type="checkbox"
              checked={exportFilters.promocodeIds.includes(p.id)}
              readOnly
            />
          </td>
          <td
            style={{
              fontFamily: "monospace",
              fontSize: 12,
              fontWeight: 700,
            }}
          >
            {p.code}
          </td>
          <td
            style={{
              color: "var(--muted)",
              fontSize: 13,
              maxWidth: 160,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {p.comment || "—"}
          </td>
          <td style={{ fontSize: 13 }}>
            {promoStateLabel(p)}
          </td>
          <td style={{ fontVariantNumeric: "tabular-nums" }}>
            {p.usedCount}/{p.maxUses}
          </td>
          <td
            style={{ color: "var(--muted)", fontSize: 12 }}
          >
            {formatDate(p.activeTo)}
          </td>
        </tr>
      ))}
      {renderTableGate("promos", 6, "Промокодов не найдено")}
    </tbody>
  </table>
</div>
)}
{renderPagination("promos")}

    </>
  );
}
