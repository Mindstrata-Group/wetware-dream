"use client";

import { useAdminPageContext } from "./AdminPageContext";

export function AdminProfileTab() {
  const { profile, status, statusHelp } = useAdminPageContext();

  const user = profile?.user;
  const stats = profile?.stats || {};
  const letter = user?.email ? user.email[0].toUpperCase() : "?";

  return (
    <div className="ms-admin-stack">
      <div className="card ms-admin-card ms-profile-card">
        <div className="ms-profile-top">
          <div className="ms-profile-top-main">
            <div className="ms-profile-avatar">{letter}</div>
            <div>
              <div className="ms-profile-title">{user?.email || `User #${user?.id}`}</div>
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                <span className="ms-chip">{user?.role || "—"}</span>
                <span
                  className="ms-chip"
                  style={{
                    fontSize: 12,
                    background:
                      user?.status === "active"
                        ? "var(--soft)"
                        : "var(--danger-bg)",
                    color: user?.status === "active" ? "var(--accent-strong)" : "#8a3333",
                    borderColor:
                      user?.status === "active" ? "#cfe5e1" : "var(--danger-line)",
                  }}
                >
                  {user?.status === "active" ? "активен" : "заблокирован"}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "minmax(0,1fr) minmax(0,1fr)",
            gap: 16,
          }}
          className="ms-profile-admin-grid"
        >
          <div>
            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 12 }}>Учётные данные</div>

            <div style={{ display: "grid", gap: 10 }}>
              <label style={{ display: "grid", gap: 6 }}>
                Email
                <input
                  className="ms-admin-form"
                  defaultValue={user?.email || ""}
                  style={{
                    minHeight: 42,
                    border: "1px solid var(--line)",
                    borderRadius: 13,
                    background: "color-mix(in oklab, var(--card) 88%, transparent)",
                    padding: "0 12px",
                    width: "100%",
                  }}
                  readOnly
                />
              </label>
            </div>
          </div>

          <div>
            <div style={{ fontSize: 13, fontWeight: 500, marginBottom: 12 }}>Роль и доступы</div>

            <div
              style={{
                padding: 14,
                borderRadius: 14,
                border: "1px solid var(--line)",
                background: "var(--soft)",
                marginBottom: 16,
              }}
            >
              <div style={{ fontSize: 13, lineHeight: 1.55, color: "var(--accent-strong)" }}>
                {user?.role === "owner"
                  ? "Полный доступ. Может удалять промокоды, режимы, рассылки, передавать роль другим администраторам. Все разделы открыты без ограничений."
                  : user?.role === "support"
                  ? "Только чтение: пользователи, доступы, диалоги, общая статистика."
                  : user?.role === "content_admin"
                  ? "Чтение режимов, оркестрации и экспортов. Изменение промптов и demo chat."
                  : user?.role === "billing_admin"
                  ? "Тарифы и финансовая статистика. Промокоды только для чтения."
                  : "Ограниченный доступ к панели."}
              </div>
            </div>

            <div className="ms-stat-grid" style={{ marginBottom: 16 }}>
              <div className="ms-stat-card">
                <span>Сообщений</span>
                <strong>
                  {user?.role === "admin" || user?.role === "owner"
                    ? (status?.counts?.messages ?? stats.messages ?? 0).toLocaleString("ru-RU")
                    : (stats.messages ?? 0).toLocaleString("ru-RU")}
                </strong>
                <small>
                  {user?.role === "admin" || user?.role === "owner" ? "Всего в системе" : "Ваши диалоги"}
                </small>
              </div>
              <div className="ms-stat-card">
                <span>Диалогов</span>
                <strong>{(stats.dialogs ?? 0).toLocaleString("ru-RU")}</strong>
              </div>
            </div>
          </div>
        </div>
      </div>

    </div>
  );
}
