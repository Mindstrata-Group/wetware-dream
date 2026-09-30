"use client";

import { ReactNode, useMemo } from "react";
import { useAdminPageContext } from "./AdminPageContext";

const AUDIENCE_SEARCH_PLACEHOLDER = {
  promocode: "🔍 поиск промокода",
  users: "🔍 поиск пользователя",
};

function SectionTitle({ children }: { children: ReactNode }) {
  return <div className="ms-field-label">{children}</div>;
}

export function AdminBroadcastTab() {
  const {
    broadcast,
    setBroadcast,
    filteredUsers,
    promocodes,
    exportPromocodes,
    exportModes,
    tariffs,
    sendBroadcast,
    exportFilters,
    setExportFilters,
    modes,
    users,
  } = useAdminPageContext();

  const promoSource = exportPromocodes?.length > 0 ? exportPromocodes : promocodes || [];

  const uiAudience = broadcast.uiAudience || "promocode";
  const unsupportedAudience = uiAudience === "tariff" || uiAudience === "mode";
  const tariffList = tariffs || [];

  function togglePromocode(id: number) {
    const ids = broadcast.promocodeIds || [];
    const next = ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
    setBroadcast((v) => ({ ...v, promocodeIds: next }));
  }

  const filteredPromoSearch = useMemo(
    () =>
      promoSource.filter((p) =>
        `${p.id} ${p.code} ${p.comment || ""}`
          .toLowerCase()
          .includes((exportFilters.promocodeSearch || "").toLowerCase())
      ),
    [promoSource, exportFilters.promocodeSearch]
  );

  const filteredUserSearch = useMemo(
    () =>
      (filteredUsers || []).filter((u) =>
        `${u.email || ""} ${u.telegramUsername || ""} ${u.id}`
          .toLowerCase()
          .includes((exportFilters.userSearch || "").toLowerCase())
      ),
    [filteredUsers, exportFilters.userSearch]
  );

  const items = useMemo(() => {
    if (uiAudience === "promocode") {
      return promoSource.map((p) => ({
        id: p.id,
        label: `#${p.id} ${p.code}${p.comment ? ` · ${p.comment}` : ""}`,
        search: `${p.id} ${p.code} ${p.comment || ""}`.toLowerCase(),
      }));
    }
    if (uiAudience === "tariff") {
      return tariffList.map((t) => ({
        id: t.id,
        label: `${t.name}${t.archivedAt ? " · архив" : ""}`,
        search: `${t.id} ${t.name} ${t.description || ""}`.toLowerCase(),
      }));
    }
    if (uiAudience === "mode") {
      return modes.map((m) => ({
        id: m.id,
        label: `#${m.id} ${m.name}`,
        search: `${m.id} ${m.name}`.toLowerCase(),
      }));
    }
    return users.map((u) => ({
      id: u.id,
      label: `#${u.id} ${u.email || u.telegramUsername || "—"}`,
      search: `${u.id} ${u.email || ""} ${u.telegramUsername || ""}`.toLowerCase(),
    }));
  }, [uiAudience, promoSource, tariffList, modes, users]);

  const selectedUiIds = useMemo(() => {
    if (uiAudience === "promocode") return broadcast.promocodeIds || [];
    return broadcast.userIds || [];
  }, [uiAudience, broadcast.promocodeIds, broadcast.userIds]);

  const filteredItems = useMemo(() => {
    const q = (broadcast.uiSearch || "").trim().toLowerCase();
    if (!q) return items;
    return items.filter((it) => it.search.includes(q));
  }, [broadcast.uiSearch, items]);

  function setUiAudience(next: string) {
    setBroadcast((v) => ({
      ...v,
      uiAudience: next,
      audience: next === "promocode" ? "promocode" : "users",
      promocodeIds: next === "promocode" ? v.promocodeIds || [] : [],
      userIds: next === "promocode" ? [] : v.userIds || [],
      uiSearch: "",
    }));
  }

  function toggleUser(id: number) {
    const ids = broadcast.userIds || [];
    const next = ids.includes(id) ? ids.filter((x) => x !== id) : [...ids, id];
    setBroadcast((v) => ({ ...v, userIds: next }));
  }

  const isPromocodeAudience = broadcast.audience === "promocode";
  const isUsersAudience = broadcast.audience === "users";

  return (
    <div className="card ms-admin-card">
      <h2>Рассылка сообщений</h2>

      <form onSubmit={sendBroadcast} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <div>
          <SectionTitle>Аудитория</SectionTitle>
          <select
            value={broadcast.uiAudience || uiAudience}
            onChange={(e) => setUiAudience(e.target.value)}
          >
            <option value="promocode">По промокодам</option>
            <option value="tariff">По тарифам</option>
            <option value="mode">По режимам</option>
            <option value="user">По пользователям</option>
          </select>
        </div>

        {isPromocodeAudience && (
          <div>
            <input
              placeholder={AUDIENCE_SEARCH_PLACEHOLDER.promocode}
              value={exportFilters.promocodeSearch || ""}
              onChange={(e) => setExportFilters((v) => ({ ...v, promocodeSearch: e.target.value }))}
              style={{ marginBottom: 8, width: "100%" }}
            />
            <div className="ms-checkbox-list">
              {filteredPromoSearch.length === 0 ? (
                <div
                  style={{
                    padding: "12px",
                    fontSize: 12,
                    color: "var(--muted)",
                    textAlign: "center",
                  }}
                >
                  Промокодов не найдено
                </div>
              ) : (
                filteredPromoSearch.map((p) => (
                  <label key={p.id}>
                    <input
                      type="checkbox"
                      checked={(broadcast.promocodeIds || []).includes(p.id)}
                      onChange={() => togglePromocode(p.id)}
                    />
                    <span>
                      #{p.id} {p.code}
                      {p.comment ? ` · ${p.comment}` : ""}
                    </span>
                  </label>
                ))
              )}
            </div>
            <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 6 }}>
              Выбрано {(broadcast.promocodeIds || []).length}. Можно отметить сразу
              несколько.
            </div>
          </div>
        )}

        {isUsersAudience && (
          <div>
            <input
              placeholder={AUDIENCE_SEARCH_PLACEHOLDER.users}
              value={exportFilters.userSearch || ""}
              onChange={(e) => setExportFilters((v) => ({ ...v, userSearch: e.target.value }))}
              style={{ marginBottom: 8, width: "100%" }}
            />
            <div className="ms-checkbox-list">
              {filteredUserSearch.length === 0 ? (
                <div
                  style={{
                    padding: "12px",
                    fontSize: 12,
                    color: "var(--muted)",
                    textAlign: "center",
                  }}
                >
                  Пользователей не найдено
                </div>
              ) : (
                filteredUserSearch.slice(0, 200).map((u) => (
                  <label key={u.id}>
                    <input
                      type="checkbox"
                      checked={(broadcast.userIds || []).includes(u.id)}
                      onChange={() => toggleUser(u.id)}
                    />
                    <span>#{u.id} {u.email || u.telegramUsername || "—"}</span>
                  </label>
                ))
              )}
            </div>
            <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 6 }}>
              Для выбора по тарифам и режимам API пока не готово: используйте «По пользователям» или «По промокодам».
            </div>
          </div>
        )}

        <div>
          <SectionTitle>Канал</SectionTitle>
          <select
            value={broadcast.channel}
            onChange={(e) => setBroadcast((v) => ({ ...v, channel: e.target.value }))}
          >
            <option value="email">Email</option>
            <option value="phone">Телефон</option>
          </select>
        </div>

        <div>
          <SectionTitle>Регулярность</SectionTitle>
          <select
            value={broadcast.every}
            onChange={(e) => setBroadcast((v) => ({ ...v, every: e.target.value }))}
          >
            <option value="once">один раз</option>
            <option value="daily">ежедневно</option>
            <option value="weekly">еженедельно</option>
          </select>
        </div>

        <div>
          <SectionTitle>Текст сообщения</SectionTitle>
          <textarea
            rows={7}
            placeholder="Текст рассылки"
            value={broadcast.message}
            onChange={(e) => setBroadcast((v) => ({ ...v, message: e.target.value }))}
          />
        </div>

        <button
          type="submit"
          className="ms-button ms-button-primary ms-button-xs"
          style={{ width: "100%" }}
          disabled={unsupportedAudience}
          title={unsupportedAudience ? "Выбор по тарифам/режимам пока недоступен для отправки" : ""}
        >
          Сохранить рассылку
        </button>
      </form>
    </div>
  );
}
