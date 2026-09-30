"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function UserListSection() {
  const ctx = useAdminPageContext();
  const {
    users, userDetail, userQ, setUserQ, userMeta, modes, setTab,
    statusHelp, roleDescriptions, roles, statuses, pageSizes, formatDate,
    patchUser, openUser, openMode, loadUsers, userPage, totalPages,
  } = ctx;
  // Compatibility with other fields: accessed via ctx.* in JSX where needed
  return (
    <>
      {/* ── User list ── */}
      <div className="card ms-admin-card">
        <div className="ms-admin-card-head">
          <h2>Пользователи</h2>
          <button
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={() => loadUsers(0)}
          >
            Найти
          </button>
        </div>

        <div className="ms-admin-toolbar">
          <input
            value={userQ}
            onChange={(e) => setUserQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                void loadUsers(0);
              }
            }}
            placeholder="email, username или id"
          />
          <select
            value={userMeta.limit}
            onChange={(e) => loadUsers(0, Number(e.target.value))}
          >
            {pageSizes.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
        </div>

        {/* Desktop table */}
        <div className="ms-table-scroll ms-users-desktop">
          <table className="ms-admin-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Пользователь</th>
                <th>Роль</th>
                <th>Статус</th>
                <th>Сообщений</th>
                <th>Последний вход</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td
                    style={{
                      fontFamily: "monospace",
                      color: "var(--muted)",
                      fontVariantNumeric: "tabular-nums",
                    }}
                  >
                    #{u.id}
                  </td>
                  <td>
                    <button
                      className="ms-link-button"
                      type="button"
                      onClick={() => openUser(u)}
                    >
                      {u.email || u.telegramUsername || "—"}
                    </button>
                  </td>
                  <td>
                    <select
                      className="ms-button ms-button-ghost ms-button-xs"
                      value={u.role}
                      onChange={(e) => patchUser(u, { role: e.target.value })}
                      style={{ minWidth: 130 }}
                    >
                      {roles.map((r) => (
                        <option key={r} value={r}>
                          {r}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <select
                      title={statusHelp(u.status)}
                      value={u.status}
                      onChange={(e) =>
                        patchUser(u, { status: e.target.value })
                      }
                      style={{
                        minWidth: 100,
                        color:
                          u.status === "blocked" ? "#ef4444" : "inherit",
                        fontWeight: u.status === "blocked" ? 700 : 400,
                      }}
                    >
                      {statuses.map((s) => (
                        <option key={s}>{s}</option>
                      ))}
                    </select>
                  </td>
                  <td style={{ fontVariantNumeric: "tabular-nums" }}>
                    {u.messageCount ?? 0}
                  </td>
                  <td
                    style={{
                      color: "var(--muted)",
                      fontSize: 13,
                      fontVariantNumeric: "tabular-nums",
                    }}
                  >
                    {formatDate(u.lastLoginAt)}
                  </td>
                </tr>
              ))}
              {users.length === 0 && (
                <tr>
                  <td
                    colSpan={6}
                    style={{
                      textAlign: "center",
                      color: "var(--muted)",
                      padding: 20,
                    }}
                  >
                    Пользователей не найдено
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Mobile cards */}
        <div className="ms-users-mobile" style={{ display: "none" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {users.map((u) => (
              <div
                key={u.id}
                style={{
                  border: "1px solid var(--line)",
                  borderRadius: 14,
                  padding: 14,
                  background: "color-mix(in oklab, var(--card) 88%, transparent)",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    gap: 8,
                    marginBottom: 6,
                  }}
                >
                  <span
                    style={{
                      fontFamily: "monospace",
                      fontSize: 12,
                      color: "var(--muted)",
                    }}
                  >
                    #{u.id}
                  </span>
                  <span
                    style={{
                      fontSize: 12,
                      color: "var(--muted)",
                      fontVariantNumeric: "tabular-nums",
                    }}
                  >
                    {formatDate(u.lastLoginAt)}
                  </span>
                </div>
                <button
                  className="ms-link-button"
                  type="button"
                  onClick={() => openUser(u)}
                  style={{
                    fontSize: 14,
                    marginBottom: 10,
                    display: "block",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    maxWidth: "100%",
                  }}
                >
                  {u.email || u.telegramUsername || "—"}
                </button>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "1fr 1fr",
                    gap: 8,
                  }}
                >
                  <select
                    className="ms-button ms-button-ghost ms-button-xs"
                    value={u.role}
                    onChange={(e) => patchUser(u, { role: e.target.value })}
                  >
                    {roles.map((r) => (
                      <option key={r} value={r}>
                        {r}
                      </option>
                    ))}
                  </select>
                  <select
                    className="ms-button ms-button-ghost ms-button-xs"
                    value={u.status}
                    onChange={(e) =>
                      patchUser(u, { status: e.target.value })
                    }
                    style={{
                      color:
                        u.status === "blocked" ? "#ef4444" : "inherit",
                    }}
                  >
                    {statuses.map((s) => (
                      <option key={s}>{s}</option>
                    ))}
                  </select>
                </div>
                <div
                  style={{
                    marginTop: 8,
                    fontSize: 12,
                    color: "var(--muted)",
                  }}
                >
                  сообщений: {u.messageCount ?? 0}
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Pagination */}
        <div className="ms-pagination">
          <button
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={userMeta.offset === 0}
            onClick={() =>
              loadUsers(Math.max(0, userMeta.offset - userMeta.limit))
            }
          >
            ← Назад
          </button>
          <span style={{ fontSize: 13, color: "var(--muted)" }}>
            Страница {userPage} из {totalPages} · всего {userMeta.total}
          </span>
          <button
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={userPage >= totalPages}
            onClick={() => loadUsers(userMeta.offset + userMeta.limit)}
          >
            Вперёд →
          </button>
        </div>

        {/* ── User card ── */}
        {userDetail && (
          <div className="ms-info-box ms-editor-card">
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "flex-start",
                gap: 12,
                flexWrap: "wrap",
                marginBottom: 12,
              }}
            >
              <div>
                <strong style={{ fontSize: 15 }}>
                  #{userDetail.id}{" "}
                  {userDetail.email ||
                    userDetail.telegramUsername ||
                    "без email"}
                </strong>
                <div
                  style={{
                    fontSize: 13,
                    color: "var(--muted)",
                    marginTop: 4,
                  }}
                >
                  {userDetail.role} · {userDetail.status}
                </div>
              </div>
              <div
                style={{
                  display: "flex",
                  gap: 8,
                  flexWrap: "wrap",
                }}
              >
                <button
                  type="button"
                  className="ms-button ms-button-primary ms-button-xs"
                  onClick={() => {
                    setTab("access");
                  }}
                >
                  Выдать доступ
                </button>
              </div>
            </div>

            {/* Active access */}
            <div style={{ fontSize: 13, marginBottom: 8 }}>
              Активные режимы
            </div>
            <div className="ms-admin-list" style={{ maxHeight: 280 }}>
              {(userDetail.activeModes || []).length === 0 ? (
                <span className="muted">Активных доступов нет.</span>
              ) : (
                (userDetail.activeModes || []).map((access) => (
                  <button
                    key={access.id}
                    type="button"
                    onClick={() => {
                      setTab("modes");
                      const mode = modes.find(
                        (m) => m.id === access.modeId
                      );
                      if (mode) void openMode(mode);
                    }}
                  >
                    <div
                      style={{
                        display: "flex",
                        justifyContent: "space-between",
                        alignItems: "center",
                        gap: 8,
                        flexWrap: "wrap",
                      }}
                    >
                      <strong>{access.modeName}</strong>
                      <span
                        className="ms-chip"
                        style={{
                          fontSize: 11,
                          background:
                            access.status === "active"
                              ? "var(--soft)"
                              : access.status === "expiring"
                              ? "#fff8d6"
                              : "var(--danger-bg)",
                          color:
                            access.status === "active"
                              ? "var(--accent-strong)"
                              : access.status === "expiring"
                              ? "#92620a"
                              : "#8a3333",
                          borderColor:
                            access.status === "active"
                              ? "#cfe5e1"
                              : access.status === "expiring"
                              ? "#f0d080"
                              : "var(--danger-line)",
                        }}
                      >
                        {access.status}
                      </span>
                    </div>
                    <span style={{ fontSize: 13, color: "var(--muted)" }}>
                      {formatDate(access.activeFrom)} →{" "}
                      {formatDate(access.activeTo)} · {access.accessType} ·{" "}
                      {access.sourceLabel}
                    </span>
                    <small>
                      Лимит/день: {access.dailyMessageLimit ?? 50} ·
                      приоритет: {access.priority} · source #
                      {access.sourceId ?? "—"}
                    </small>
                    {access.quota && (
                      <small>
                        Квота: {access.quota.used} /{" "}
                        {access.quota.limit ?? "∞"} · осталось:{" "}
                        {access.quota.remaining ?? "∞"}
                      </small>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>
        )}
      </div>

      <style>{`
        @media (max-width: 760px) {
          .ms-users-create-grid {
            grid-template-columns: 1fr !important;
          }
          .ms-users-desktop {
            display: none !important;
          }
          .ms-users-mobile {
            display: block !important;
          }
        }
      `}</style>
    </>
  );
}
