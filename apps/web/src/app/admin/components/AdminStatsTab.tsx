"use client";

import { useAdminPageContext } from "./AdminPageContext";

// Simple per-day SVG chart (no external dependencies)
function SparkChart({ rows }: { rows: any[] }) {
  if (!rows || rows.length === 0) return null;

  const pts = rows
    .slice()
    .reverse()
    .slice(0, 14)
    .map((r) => Number(r.requests || 0));

  if (pts.every((v) => v === 0)) return null;

  const W = 600;
  const H = 120;
  const maxV = Math.max(...pts, 1);
  const stepX = W / Math.max(pts.length - 1, 1);

  const path = pts
    .map((v, i) =>
      (i === 0 ? "M" : "L") + i * stepX + " " + (H - (v / maxV) * H)
    )
    .join(" ");

  const area =
    path +
    " L " +
    (pts.length - 1) * stepX +
    " " +
    H +
    " L 0 " +
    H +
    " Z";

  return (
    <svg
      viewBox={"0 0 " + W + " " + H}
      style={{ width: "100%", height: 120 }}
      preserveAspectRatio="none"
    >
      <defs>
        <linearGradient id="sgFill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="var(--accent)" stopOpacity="0.22" />
          <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill="url(#sgFill)" />
      <path
        d={path}
        fill="none"
        stroke="var(--accent)"
        strokeWidth="2"
      />
      {pts.map((v, i) => (
        <circle
          key={i}
          cx={i * stepX}
          cy={H - (v / maxV) * H}
          r="3"
          fill="#fff"
          stroke="var(--accent)"
          strokeWidth="2"
        />
      ))}
    </svg>
  );
}

// Horizontal bar for the top modes
function BarRow({
  label,
  value,
  max,
  color = "var(--accent)",
}: {
  label: string;
  value: number;
  max: number;
  color?: string;
}) {
  const pct = max > 0 ? (value / max) * 100 : 0;
  return (
    <div style={{ padding: "7px 0" }}>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          fontSize: 12,
          marginBottom: 4,
        }}
      >
        <span
          style={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            maxWidth: "70%",
          }}
        >
          {label}
        </span>
        <span
          style={{
            fontVariantNumeric: "tabular-nums",
            color: "var(--muted)",
          }}
        >
          {value.toLocaleString("ru-RU")}
        </span>
      </div>
      <div
        style={{
          height: 5,
          background: "var(--line)",
          borderRadius: 3,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            width: pct + "%",
            height: "100%",
            background: color,
            borderRadius: 3,
            transition: "width 0.4s ease",
          }}
        />
      </div>
    </div>
  );
}

export function AdminStatsTab() {
  const {
    adminStats,
    statsSort,
    sortButton,
    toggleStatsSort,
    loadAdminStats,
    sortedRows,
  } = useAdminPageContext();

  const totals = adminStats?.totals || {};
  const daily: any[] = adminStats?.daily || [];
  const byMode: any[] = adminStats?.byMode || [];
  const byUser: any[] = adminStats?.byUser || [];
  const byTariff: any[] = adminStats?.byTariff || [];

  // Top 5 modes for the bars
  const topModes = [...byMode]
    .sort((a, b) => (b.requests || 0) - (a.requests || 0))
    .slice(0, 5);
  const maxModeRequests = topModes[0]?.requests || 1;

  return (
    <div className="card ms-admin-card">
      <div className="ms-admin-card-head">
        <h2>Статистика затрат и использования</h2>
        <button
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={loadAdminStats}
        >
          Обновить
        </button>
      </div>

      {!adminStats ? (
        <p className="muted">Загрузка...</p>
      ) : (
        <>
          {/* ── Totals grid ── */}
          <div className="ms-stat-grid" style={{ marginBottom: 20 }}>
            {[
              {
                label: "AI-запросов",
                value: (totals.requests || 0).toLocaleString("ru-RU"),
              },
              {
                label: "Пользователей",
                value: (totals.users || 0).toLocaleString("ru-RU"),
              },
              {
                label: "Токенов всего",
                value: (totals.tokens || 0).toLocaleString("ru-RU"),
              },
              {
                label: "Среднее токенов",
                value: Number(totals.avgTokens || 0).toFixed(1),
              },
              {
                label: "Среднее режимов / польз.",
                value: Number(totals.avgModesPerUser || 0).toFixed(1),
              },
              {
                label: "Оценка затрат $",
                value: Number(totals.cost || 0).toFixed(4),
              },
              {
                label: "$ на 1 токен",
                value: Number(totals.costPerToken || 0).toFixed(8),
              },
            ].map((s) => (
              <div key={s.label} className="ms-stat-card">
                <span>{s.label}</span>
                <strong>{s.value}</strong>
              </div>
            ))}
          </div>

          {/* ── Two-column layout: chart + mode bars ── */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "minmax(0, 1.6fr) minmax(0, 1fr)",
              gap: 16,
              marginBottom: 16,
            }}
            className="ms-stats-two-col"
          >
            {/* Per-day chart */}
            <div
              className="card"
              style={{ padding: 16, border: "1px solid var(--line)" }}
            >
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "baseline",
                  marginBottom: 12,
                  gap: 8,
                  flexWrap: "wrap",
                }}
              >
                <div style={{ fontSize: 14 }}>
                  Запросы по дням (последние 14)
                </div>
                <span style={{ fontSize: 12, color: "var(--muted)" }}>
                  за 30 дней
                </span>
              </div>
              <SparkChart rows={daily} />
              {daily.length > 0 && (
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    marginTop: 8,
                    fontSize: 11,
                    color: "var(--muted)",
                  }}
                >
                  <span>
                    {daily[daily.length - 1]?.label || ""}
                  </span>
                  <span>{daily[0]?.label || ""}</span>
                </div>
              )}
            </div>

            {/* Top modes */}
            <div
              className="card"
              style={{ padding: 16, border: "1px solid var(--line)" }}
            >
              <div
                style={{
                  fontSize: 14,
                  marginBottom: 12,
                }}
              >
                Топ режимов по запросам
              </div>
              {topModes.length === 0 ? (
                <p className="muted" style={{ fontSize: 13 }}>
                  Нет данных
                </p>
              ) : (
                topModes.map((r, i) => (
                  <BarRow
                    key={r.label}
                    label={r.label}
                    value={r.requests}
                    max={maxModeRequests}
                    color={
                      i === 0
                        ? "var(--accent)"
                        : i === 1
                        ? "#4a9eff"
                        : i === 2
                        ? "#f59e0b"
                        : "var(--muted)"
                    }
                  />
                ))
              )}
            </div>
          </div>

          {/* ── Detailed tables ── */}
          {(
            [
              ["По дням", daily],
              ["По тарифам / источникам", byTariff],
              ["По пользователям", byUser],
              ["По режимам", byMode],
            ] as [string, any[]][]
          ).map(([title, rows]) => (
            <div
              key={title}
              className="ms-table-scroll"
              style={{ marginTop: 16, maxHeight: "none", overflowX: "auto", overflowY: "visible" }}
            >
              <div
                style={{
                  fontSize: 14,
                  fontWeight: 600,
                  padding: "10px 14px",
                  borderBottom: "1px solid var(--line)",
                  background: "var(--card)",
                  color: "var(--foreground)",
                  position: "sticky",
                  top: 0,
                }}
              >
                {title}
              </div>
              <table className="ms-admin-table">
                <thead>
                  <tr>
                    <th>
                      {sortButton(
                        "Срез",
                        "label",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "label")
                      )}
                    </th>
                    <th>
                      {sortButton(
                        "Запросов",
                        "requests",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "requests")
                      )}
                    </th>
                    <th>
                      {sortButton(
                        "Польз.",
                        "users",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "users")
                      )}
                    </th>
                    <th>
                      {sortButton(
                        "Токены",
                        "tokens",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "tokens")
                      )}
                    </th>
                    <th>
                      {sortButton(
                        "Среднее",
                        "avgTokens",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "avgTokens")
                      )}
                    </th>
                    <th>
                      {sortButton(
                        "Затраты $",
                        "cost",
                        statsSort[title] || null,
                        () => toggleStatsSort(title, "cost")
                      )}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {sortedRows(
                    (rows || []) as any[],
                    statsSort[title] || null
                  ).map((row: any) => (
                    <tr key={`${title}-${row.label}`}>
                      <td
                        style={{
                          maxWidth: 260,
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {row.label}
                      </td>
                      <td style={{ fontVariantNumeric: "tabular-nums" }}>
                        {(row.requests || 0).toLocaleString("ru-RU")}
                      </td>
                      <td style={{ fontVariantNumeric: "tabular-nums" }}>
                        {(row.users || 0).toLocaleString("ru-RU")}
                      </td>
                      <td style={{ fontVariantNumeric: "tabular-nums" }}>
                        {(row.tokens || 0).toLocaleString("ru-RU")}
                      </td>
                      <td style={{ fontVariantNumeric: "tabular-nums" }}>
                        {Number(row.avgTokens || 0).toFixed(1)}
                      </td>
                      <td style={{ fontVariantNumeric: "tabular-nums" }}>
                        {Number(row.cost || 0).toFixed(4)}
                      </td>
                    </tr>
                  ))}
                  {(!rows || rows.length === 0) && (
                    <tr>
                      <td
                        colSpan={6}
                        style={{
                          textAlign: "center",
                          color: "var(--muted)",
                          padding: 16,
                        }}
                      >
                        Нет данных
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          ))}
        </>
      )}

      <style>{`
        @media (max-width: 860px) {
          .ms-stats-two-col {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
