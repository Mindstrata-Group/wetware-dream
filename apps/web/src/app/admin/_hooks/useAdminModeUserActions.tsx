import type { ModeRow, UserAccessMode, UserRow } from "../types";
import type { AdminPageContextValue } from "../components/AdminPageContext";
import React, { useMemo } from "react";
import { apiFetch } from "@/lib/api";

type AdminModeUserActionsContext = Pick<AdminPageContextValue, "created" | "setCreated" | "grant" | "setGrant" | "tariffForm" | "setTariffForm" | "promoForm" | "setPromoForm" | "grantUserDetail" | "setGrantUserDetail" | "exportModes" | "modes" | "setError" | "showNotice" | "loadUsers" | "handleError" | "setUserDetail">;

export function useAdminModeUserActions(ctx: AdminModeUserActionsContext) {
  const { created, setCreated, grant, setGrant, tariffForm, setTariffForm, promoForm, setPromoForm, grantUserDetail, setGrantUserDetail, exportModes, modes, setError, showNotice, loadUsers, handleError, setUserDetail } = ctx;
  function setModeSelection(scope: "created" | "grant" | "tariff" | "promo", ids: number[]) {
    const uniq = Array.from(new Set(ids));
    if (scope === "created") setCreated((v) => ({ ...v, modeIds: uniq }));
    if (scope === "grant") setGrant((v) => ({ ...v, modeIds: uniq }));
    if (scope === "tariff") setTariffForm((v) => ({ ...v, modeIds: uniq, firstModeId: v.firstModeId && uniq.includes(v.firstModeId) ? v.firstModeId : uniq[0] ?? null }));
    if (scope === "promo") setPromoForm((v) => ({ ...v, targetIds: uniq }));
  }
  function toggleModeId(scope: "created" | "grant" | "tariff" | "promo", id: number) {
    const current = scope === "created" ? created.modeIds : scope === "grant" ? grant.modeIds : scope === "tariff" ? tariffForm.modeIds : promoForm.targetIds;
    setModeSelection(scope, current.includes(id) ? current.filter((x) => x !== id) : [...current, id]);
  }
  const activeGrantModeIds = useMemo(() => Array.from(new Set((grantUserDetail?.activeModes || []).map((item) => item.modeId))), [grantUserDetail]);
  const grantModeOptions: ModeRow[] = exportModes.length ? exportModes : modes;

  function modeSelectionToolbar(scope: "created" | "grant" | "promo") {
    return (
      <div className="ms-admin-mini-actions">
        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => setModeSelection(scope, grantModeOptions.map((m) => m.id))}>Выбрать все режимы</button>
        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => setModeSelection(scope, grantModeOptions.filter((m) => m.paid).map((m) => m.id))}>Выбрать платные</button>
        <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => setModeSelection(scope, [])}>Снять все</button>
      </div>
    );
  }
  function modeCheckboxes(scope: "created" | "grant" | "tariff" | "promo", ids: number[], showChains = false) {
    return (
      <div className="ms-mode-checkboxes">
        {grantModeOptions.map((m) => {
          const selected = ids.includes(m.id);
          if (scope === "tariff") {
            return (
              <div key={m.id} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <label style={{ flex: 1 }}>
                  <input type="checkbox" checked={selected} onChange={() => toggleModeId(scope, m.id)} />
                  <span>{m.name}{m.paid ? " · платный" : ""}{showChains && m.paidChains ? <small>{m.paidChains}</small> : null}</span>
                </label>
                {selected ? (
                  <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", alignItems: "center", gap: 3, cursor: "pointer" }}>
                    <input
                      type="radio"
                      name="tariff_first_mode_create"
                      checked={tariffForm.firstModeId === m.id}
                      onChange={() => setTariffForm((v) => ({ ...v, firstModeId: m.id }))}
                    />
                    ведущий
                  </label>
                ) : null}
              </div>
            );
          }
          return (
            <label key={m.id}>
              <input type="checkbox" checked={selected} onChange={() => toggleModeId(scope, m.id)} />
              <span>{m.name}{m.paid ? " · платный" : ""}{showChains && m.paidChains ? <small>{m.paidChains}</small> : null}</span>
            </label>
          );
        })}
      </div>
    );
  }

  async function createUser(e: React.FormEvent) {
    e.preventDefault(); setError("");
    try {
      await apiFetch("/api/admin/users", { method: "POST", body: JSON.stringify({ ...created, password: "", days: Number(created.days) }) });
      showNotice("Пользователь создан.");
      setCreated({ email: "", password: "", role: "user", status: "active", days: "30", modeIds: [] });
      await loadUsers(0);
    } catch (err) { handleError(err, "Ошибка создания"); }
  }
  async function selectGrantUser(user: UserRow) {
    try {
      const json = await apiFetch<any>(`/api/admin/users/${user.id}`);
      const detail = json.user || null;
      const activeModeIds: number[] = Array.from(new Set<number>((detail?.activeModes || []).map((item: UserAccessMode) => item.modeId)));
      setGrantUserDetail(detail);
      setGrant((v) => ({ ...v, email: user.email || String(user.id), userId: String(user.id), tariffId: "", modeIds: activeModeIds }));
    } catch (e) { handleError(e, "Ошибка загрузки доступов пользователя"); }
  }
  async function syncSelectedUserAccess() {
    if (!grantUserDetail) return false;
    const selected = new Set(grant.modeIds);
    const active = new Set(activeGrantModeIds);
    const toRemove = activeGrantModeIds.filter((id) => !selected.has(id));
    await Promise.all(toRemove.map((modeId) => apiFetch(`/api/admin/users/${grantUserDetail.id}/access?modeId=${modeId}`, { method: "DELETE" })));
    let granted = 0, extended = 0;
    if (grant.modeIds.length) {
      const json = await apiFetch<any>("/api/admin/access", { method: "POST", body: JSON.stringify({ userId: grantUserDetail.id, email: grant.email, tariffId: 0, modeIds: grant.modeIds, days: Number(grant.days), dailyMessageLimit: Number(grant.dailyMessageLimit || 50), replaceActive: true }) });
      granted = Number(json.granted || 0); extended = Number(json.extended || 0);
    }
    const refreshed = await apiFetch<any>(`/api/admin/users/${grantUserDetail.id}`);
    const detail = refreshed.user || null;
    setGrantUserDetail(detail);
    setGrant((v) => ({ ...v, modeIds: Array.from(new Set<number>((detail?.activeModes || []).map((item: UserAccessMode) => item.modeId))) }));
    showNotice(`Доступы обновлены: добавлено ${granted}, продлено ${extended}, снято ${toRemove.length}.`);
    return true;
  }
  async function resetSelectedUserModeLimits() {
    if (!grantUserDetail || !grant.modeIds.length) return;
    try {
      const json = await apiFetch<any>("/api/admin/access", { method: "POST", body: JSON.stringify({ userId: grantUserDetail.id, modeIds: grant.modeIds, resetLimits: true }) });
      const refreshed = await apiFetch<any>(`/api/admin/users/${grantUserDetail.id}`);
      setGrantUserDetail(refreshed.user || null);
      showNotice(`Лимиты сброшены для записей: ${Number(json.updated || 0)}.`);
    } catch (e) { handleError(e, "Ошибка сброса лимитов"); }
  }
  async function grantAccess(e: React.FormEvent) {
    e.preventDefault();
    try {
      if (grantUserDetail && !grant.tariffId) { await syncSelectedUserAccess(); return; }
      const json = await apiFetch<any>("/api/admin/access", { method: "POST", body: JSON.stringify({ userId: Number(grant.userId) || 0, email: grant.email, tariffId: Number(grant.tariffId) || 0, modeIds: grant.modeIds, days: Number(grant.days), dailyMessageLimit: Number(grant.dailyMessageLimit || 50) }) });
      const granted = Number(json.granted || 0); const extended = Number(json.extended || 0);
      showNotice(extended > 0 && granted > 0 ? `Доступы выданы: ${granted}; продлены: ${extended}.` : extended > 0 ? `Доступы продлены: ${extended}.` : `Доступы выданы: ${granted}.`);
      setGrant({ userId: "", email: "", days: "30", tariffId: "", modeIds: [], dailyMessageLimit: "50" });
      setGrantUserDetail(null);
    } catch (err) { handleError(err, "Ошибка доступа"); }
  }
  async function patchUser(user: UserRow, patch: Partial<UserRow>) {
    try { await apiFetch(`/api/admin/users/${user.id}`, { method: "PATCH", body: JSON.stringify(patch) }); await loadUsers(); showNotice("Пользователь обновлён."); } catch (e) { handleError(e, "Ошибка пользователя"); }
  }
  async function openUser(user: UserRow) {
    try { const json = await apiFetch<any>(`/api/admin/users/${user.id}`); setUserDetail(json.user || null); } catch (e) { handleError(e, "Ошибка пользователя"); }
  }

  return { activeGrantModeIds, grantModeOptions, setModeSelection, toggleModeId, modeSelectionToolbar, modeCheckboxes, createUser, selectGrantUser, syncSelectedUserAccess, resetSelectedUserModeLimits, grantAccess, patchUser, openUser };
}
