"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function UserCreateForm() {
  const ctx = useAdminPageContext();
  const {
    created, setCreated, statusHelp, roleDescriptions, roles, statuses,
    createUser, modeSelectionToolbar, modeCheckboxes,
  } = ctx;
  // Compatibility with other fields: accessed via ctx.* in JSX where needed
  return (
    <>

      {/* ── Create user ── */}
      <div className="card ms-admin-card">
        <h2>Создать пользователя</h2>
        <form className="ms-admin-form" onSubmit={createUser}>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr",
              gap: 12,
            }}
            className="ms-users-create-grid"
          >
            <label>
              Email
              <input
                placeholder="you@example.com"
                value={created.email}
                onChange={(e) =>
                  setCreated((v) => ({ ...v, email: e.target.value }))
                }
              />
            </label>
          </div>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr 1fr",
              gap: 12,
            }}
            className="ms-users-create-grid"
          >
            <label>
              Роль
              <select
                value={created.role}
                onChange={(e) =>
                  setCreated((v) => ({ ...v, role: e.target.value }))
                }
              >
                {roles.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </label>

            <label>
              Статус
              <select
                value={created.status}
                onChange={(e) =>
                  setCreated((v) => ({ ...v, status: e.target.value }))
                }
              >
                {statuses.map((s) => (
                  <option key={s}>{s}</option>
                ))}
              </select>
            </label>

            <label>
              Дней доступа
              <input
                placeholder="30"
                value={created.days}
                onChange={(e) =>
                  setCreated((v) => ({ ...v, days: e.target.value }))
                }
              />
            </label>
          </div>

          {/* Role description */}
          {created.role && (
            <div
              className="ms-info-box"
              style={{ fontSize: 13, padding: "10px 14px" }}
            >
              {roleDescriptions[created.role]}
            </div>
          )}

          {/* Status description */}
          <div style={{ fontSize: 12, color: "var(--muted)" }}>
            {statusHelp(created.status)}
          </div>

          {/* Modes */}
          <div style={{ fontSize: 13 }}>
            Режимы для выдачи доступа
          </div>
          {modeSelectionToolbar("created")}
          {modeCheckboxes("created", created.modeIds)}

          <button className="ms-button ms-button-primary ms-button-xs">
            Создать и выдать
          </button>
        </form>
      </div>
    </>
  );
}
