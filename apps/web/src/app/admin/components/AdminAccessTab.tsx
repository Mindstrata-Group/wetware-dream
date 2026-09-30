"use client";

import { useAdminPageContext } from "./AdminPageContext";

export function AdminAccessTab() {
  const {
    status,
    users,
    grantUserDetail,
    setGrantUserDetail,
    userMeta,
    userQ,
    setUserQ,
    grant,
    setGrant,
    grantLimitManual,
    setGrantLimitManual,
    userPage,
    totalPages,
    activeTariffs,
    loadUsers,
    modeSelectionToolbar,
    modeCheckboxes,
    selectGrantUser,
    resetSelectedUserModeLimits,
    grantAccess,
    formatDate,
    pageSizes,
    grantLimitSliderMax,
    grantLimitFromSlider,
    grantSliderFromLimit,
    normalizeGrantLimit,
  } = useAdminPageContext();

  return (
    <div className="card ms-admin-card">
      <h2>Выдать доступы</h2>

      <form className="ms-admin-form" onSubmit={grantAccess}>

        {/* ── User search ── */}
        <div
          className="ms-info-box ms-editor-card"
          style={{ padding: 16 }}
        >
          <strong style={{ fontSize: 14 }}>
            Выберите пользователя для выдачи доступа
          </strong>

          <div className="ms-admin-toolbar" style={{ marginTop: 10 }}>
            <input
              value={userQ}
              onChange={(e) => setUserQ(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void loadUsers(0);
                }
              }}
              placeholder="поиск: email, username или id"
            />
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => loadUsers(0)}
            >
              Найти
            </button>
            <select
              value={userMeta.limit}
              onChange={(e) => loadUsers(0, Number(e.target.value))}
            >
              {pageSizes.map((s) => (
                <option key={s}>{s}</option>
              ))}
            </select>
          </div>

          <div className="ms-table-scroll" style={{ maxHeight: 300 }}>
            <table className="ms-admin-table">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Пользователь</th>
                  <th>Добавлен</th>
                  <th>Сообщ.</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr
                    key={u.id}
                    style={{
                      background:
                        grantUserDetail?.id === u.id
                          ? "var(--soft)"
                          : "transparent",
                    }}
                  >
                    <td
                      style={{
                        fontFamily: "monospace",
                        color: "var(--muted)",
                      }}
                    >
                      #{u.id}
                    </td>
                    <td
                      style={{
                        fontWeight: grantUserDetail?.id === u.id ? 600 : 400,
                        color:
                          grantUserDetail?.id === u.id
                            ? "var(--accent-strong)"
                            : "inherit",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                        whiteSpace: "nowrap",
                        maxWidth: 220,
                      }}
                    >
                      {u.email || u.telegramUsername || "без email"}
                    </td>
                    <td
                      style={{
                        color: "var(--muted)",
                        fontSize: 13,
                        fontVariantNumeric: "tabular-nums",
                      }}
                    >
                      {formatDate(u.createdAt)}
                    </td>
                    <td style={{ fontVariantNumeric: "tabular-nums" }}>
                      {u.messageCount ?? 0}
                    </td>
                    <td>
                      <button
                        type="button"
                        className={
                          grantUserDetail?.id === u.id
                            ? "ms-button ms-button-primary ms-button-xs"
                            : "ms-button ms-button-ghost ms-button-xs"
                        }
                        onClick={() => void selectGrantUser(u)}
                      >
                        {grantUserDetail?.id === u.id
                          ? "Выбран"
                          : "Выбрать"}
                      </button>
                    </td>
                  </tr>
                ))}
                {users.length === 0 && (
                  <tr>
                    <td
                      colSpan={5}
                      style={{
                        textAlign: "center",
                        color: "var(--muted)",
                        padding: 20,
                      }}
                    >
                      Введите запрос и нажмите Найти
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <div className="ms-pagination">
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              disabled={userMeta.offset === 0}
              onClick={() =>
                loadUsers(Math.max(0, userMeta.offset - userMeta.limit))
              }
            >
              ← Назад
            </button>
            <span style={{ fontSize: 13, color: "var(--muted)" }}>
              {userPage} / {totalPages}
            </span>
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              disabled={userPage >= totalPages}
              onClick={() => loadUsers(userMeta.offset + userMeta.limit)}
            >
              Вперёд →
            </button>
          </div>
        </div>

        {/* ── Current access of the selected user ── */}
        {grantUserDetail && (
          <div
            style={{
              padding: 16,
              borderRadius: 14,
              border: "1px solid var(--line)",
              background: "var(--soft)",
            }}
          >
            <strong
              style={{
                fontSize: 14,
                color: "var(--accent-strong)",
                display: "block",
                marginBottom: 6,
              }}
            >
              Текущие доступы пользователя #{grantUserDetail.id}
            </strong>
            <div
              style={{
                fontSize: 13,
                color: "var(--accent-strong)",
                marginBottom: 10,
              }}
            >
              Галочки ниже уже проставлены по активным режимам. Снимите
              галочку и нажмите «Обновить доступы», чтобы убрать режим;
              поставьте новую — чтобы добавить.
            </div>
            <div
              className="ms-admin-list"
              style={{ maxHeight: 200, marginTop: 0 }}
            >
              {(grantUserDetail.activeModes || []).length === 0 ? (
                <span className="muted">
                  Активных доступов нет — можно отметить режимы ниже и
                  добавить.
                </span>
              ) : (
                (grantUserDetail.activeModes || []).map((access) => (
                  <div
                    key={access.id}
                    style={{
                      padding: "8px 12px",
                      border: "1px solid #bee1d7",
                      borderRadius: 12,
                      background: "color-mix(in oklab, var(--card) 88%, transparent)",
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      gap: 8,
                      flexWrap: "wrap",
                    }}
                  >
                    <span style={{ fontWeight: 700, fontSize: 13 }}>
                      {access.modeName}
                    </span>
                    <span
                      style={{ fontSize: 12, color: "var(--muted)" }}
                    >
                      {access.status} · до {formatDate(access.activeTo)} ·
                      лимит/день: {access.dailyMessageLimit ?? 50}
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        {/* ── Access period ── */}
        <label>
          Срок доступа: {grant.days} дней
          <input
            type="range"
            min="1"
            max="365"
            value={grant.days}
            onChange={(e) =>
              setGrant((v) => ({ ...v, days: e.target.value }))
            }
          />
        </label>

        {/* ── Daily message limit ── */}
        <label>
          Лимит сообщений в день:{" "}
          {grantLimitFromSlider(grantSliderFromLimit(grant.dailyMessageLimit))}
          <input
            type="range"
            min="0"
            max={grantLimitSliderMax}
            step="1"
            value={grantSliderFromLimit(grant.dailyMessageLimit)}
            onChange={(e) =>
              setGrant((v) => ({
                ...v,
                dailyMessageLimit: String(
                  grantLimitFromSlider(e.target.value)
                ),
              }))
            }
          />
          <small>
            Минимум 2. До 100 настраивается по 1 сообщению, дальше шаги
            крупнее: 125, 150, 200, 250, 300, 400, 500, 750, 1000+.
          </small>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            style={{ marginTop: 6 }}
            onClick={() => setGrantLimitManual((v) => !v)}
          >
            {grantLimitManual
              ? "Скрыть ручной ввод"
              : "Ввести точно вручную"}
          </button>
          {grantLimitManual && (
            <input
              type="number"
              min="2"
              step="1"
              value={grant.dailyMessageLimit || "50"}
              style={{ marginTop: 6 }}
              onChange={(e) =>
                setGrant((v) => ({
                  ...v,
                  dailyMessageLimit: normalizeGrantLimit(e.target.value),
                }))
              }
            />
          )}
        </label>

        {/* ── Tariff or modes ── */}
        <label>
          Способ выдачи
          <select
            value={grant.tariffId}
            onChange={(e) =>
              setGrant((v) => ({
                ...v,
                tariffId: e.target.value,
                modeIds: [],
              }))
            }
          >
            <option value="">Выдать отдельные режимы</option>
            {activeTariffs.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} · {t.monthlyPrice} ·{" "}
                {t.groupName || "без группы"}
              </option>
            ))}
          </select>
        </label>

        {grant.tariffId ? (
          <div className="ms-info-box" style={{ fontSize: 13 }}>
            Будут выданы режимы и параметры выбранного тарифа.
          </div>
        ) : (
          <>
            {modeSelectionToolbar("grant")}
            {modeCheckboxes("grant", grant.modeIds)}
          </>
        )}

        {/* ── Action buttons ── */}
        <div className="ms-admin-mini-actions">
          {grantUserDetail && !grant.tariffId && (
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={resetSelectedUserModeLimits}
            >
              Обнулить лимиты выбранных режимов
            </button>
          )}
          <button className="ms-button ms-button-primary ms-button-xs">
            {grantUserDetail && !grant.tariffId
              ? "Обновить доступы"
              : "Выдать доступ"}
          </button>
        </div>
      </form>
    </div>
  );
}