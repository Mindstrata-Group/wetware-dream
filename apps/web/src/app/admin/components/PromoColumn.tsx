"use client";

import type { Dispatch, SetStateAction } from "react";
import type { ModeRow, Promo, Tariff } from "../types";
import type { PromoEditForm } from "./AdminPageContext";
import { formatDate, promoStateLabel } from "../utils";
import { QRImage } from "./QRImage";

export function PromoColumn({
  title, items, modes, tariffs, copy, deactivate, startActivate, activation, setActivation, activate, edit, setEdit, startEdit, saveEdit, describeModes,
}: {
  title: string;
  items: Promo[];
  modes: ModeRow[];
  tariffs: Tariff[];
  copy: (text: string) => void;
  deactivate: (id: number) => void;
  startActivate: (promo: Promo) => void;
  activation: { id: number; activeTo: string; maxUses: string } | null;
  setActivation: Dispatch<SetStateAction<{ id: number; activeTo: string; maxUses: string } | null>>;
  activate: () => void;
  edit: PromoEditForm | null;
  setEdit: Dispatch<SetStateAction<PromoEditForm | null>>;
  startEdit: (promo: Promo) => void;
  saveEdit: () => void;
  describeModes: (promo: Promo) => string;
}) {
  function toggleEditTarget(id: number) {
    setEdit((value) => {
      if (!value) return value;
      const exists = value.targetIds.includes(id);
      const targetIds = exists ? value.targetIds.filter((item) => item !== id) : [...value.targetIds, id];
      return {
        ...value,
        targetIds,
        firstModeId: exists && value.firstModeId === id ? 0 : value.firstModeId,
      };
    });
  }

  return (
    <div className="card ms-admin-card">
      <h2>{title}</h2>
      <div className="ms-admin-list">
        {items.map((p) => {
          const isEditing = edit?.id === p.id;
          return (
            <div key={p.id} className="ms-info-box">
              <strong>{p.code}</strong>
              <span>{p.comment || "без комментария"} · использовано {p.usedCount}/{p.maxUses} · последнее: {formatDate(p.lastUsedAt)}</span>
              <span>{p.purpose}</span>
              <small>Статус промокода: {promoStateLabel(p)}</small>
              <small>Доступные режимы: {describeModes(p)}</small>
              <small>Окно активации до: {formatDate(p.activeTo)}.</small>
              <div className="ms-admin-mini-actions">
                <button className="ms-button ms-button-ghost ms-button-xs" onClick={() => copy(p.applyUrl)}>Скопировать ссылку</button>
                <a className="ms-button ms-button-ghost ms-button-xs" href={p.applyUrl}>Перейти</a>
                {p.temporaryAdminUrl && <a className="ms-button ms-button-ghost ms-button-xs" href={p.temporaryAdminUrl}>Временная админка</a>}
                <button className="ms-button ms-button-ghost ms-button-xs" onClick={() => startEdit(p)}>Редактировать состав</button>
              </div>
              <QRImage code={p.code} url={p.applyUrl} />
              {isEditing && edit && (
                <div className="ms-admin-form ms-editor-card">
                  <strong>Состав промокода</strong>
                  <label>Что выдаёт промокод
                    <select
                      value={edit.grantsType}
                      onChange={(event) => setEdit((value) => value ? { ...value, grantsType: event.target.value, targetIds: [], firstModeId: 0 } : value)}
                    >
                      <option value="tariff">Тариф или несколько</option>
                      <option value="mode">Режимы</option>
                      <option value="admin_role">Админ-доступ</option>
                    </select>
                  </label>
                  {edit.grantsType === "mode" ? (
                    <div className="ms-mode-checkboxes">
                      {modes.map((mode) => (
                        <label key={mode.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                          <input
                            type="checkbox"
                            checked={edit.targetIds.includes(mode.id)}
                            onChange={() => toggleEditTarget(mode.id)}
                          />
                          <span style={{ flex: 1 }}>{mode.name}</span>
                          {edit.targetIds.includes(mode.id) && (
                            <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", alignItems: "center", gap: 3, cursor: "pointer" }}>
                              <input
                                type="radio"
                                name={`promo_edit_first_${p.id}`}
                                checked={edit.firstModeId === mode.id}
                                onChange={() => setEdit((value) => value ? { ...value, firstModeId: mode.id } : value)}
                              />
                              первый
                            </label>
                          )}
                        </label>
                      ))}
                    </div>
                  ) : edit.grantsType === "tariff" ? (
                    <div className="ms-mode-checkboxes">
                      {tariffs.map((tariff) => (
                        <label key={tariff.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                          <input
                            type="checkbox"
                            checked={edit.targetIds.includes(tariff.id)}
                            onChange={() => toggleEditTarget(tariff.id)}
                          />
                          <span className="ms-ellipsis" style={{ flex: 1 }} title={tariff.name}>
                            {tariff.name} · {tariff.monthlyPrice} ₽{tariff.groupName ? ` · ${tariff.groupName}` : ""}
                          </span>
                        </label>
                      ))}
                    </div>
                  ) : (
                    <div className="ms-info-box" style={{ fontSize: 13 }}>
                      Промокод выдаёт роль admin тем, кто уже активировал ссылку или активирует её позже.
                    </div>
                  )}
                  <div className="ms-promo-bulk-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
                    <label>Можно активировать до
                      <input type="date" value={edit.activeTo} onChange={(event) => setEdit((value) => value ? { ...value, activeTo: event.target.value } : value)} />
                    </label>
                    <label>Лимит активаций
                      <input type="number" min={Math.max((p.usedCount || 0) + 1, 1)} value={edit.maxUses} onChange={(event) => setEdit((value) => value ? { ...value, maxUses: event.target.value } : value)} />
                    </label>
                  </div>
                  <label>Как применить состав
                    <select
                      value={edit.updateScope}
                      onChange={(event) => setEdit((value) => value ? { ...value, updateScope: event.target.value as PromoEditForm["updateScope"] } : value)}
                    >
                      <option value="new_only">Только новые активации</option>
                      <option value="all_activations">Обновить все активации</option>
                    </select>
                  </label>
                  <div className="ms-admin-mini-actions">
                    <button className="ms-button ms-button-primary ms-button-xs" onClick={saveEdit}>Сохранить</button>
                    <button className="ms-button ms-button-ghost ms-button-xs" onClick={() => setEdit(null)}>Отмена</button>
                  </div>
                </div>
              )}
              {!p.active && <strong className="ms-reactivate-title">Активировать повторно</strong>}
              {p.active ? (
                <button className="ms-button ms-button-ghost ms-button-xs" onClick={() => deactivate(p.id)}>Деактивировать</button>
              ) : activation?.id === p.id ? (
                <div className="ms-admin-form ms-editor-card">
                  <strong>Активировать промокод</strong>
                  <label>Новая дата, до которой промокод можно активировать
                    <input type="date" autoFocus value={activation.activeTo} onChange={(e) => setActivation((v) => v ? { ...v, activeTo: e.target.value } : v)} />
                  </label>
                  <label>Новый лимит активаций
                    <input type="number" min={Math.max((p.usedCount || 0) + 1, 1)} value={activation.maxUses} onChange={(e) => setActivation((v) => v ? { ...v, maxUses: e.target.value } : v)} />
                  </label>
                  <div className="ms-admin-mini-actions">
                    <button className="ms-button ms-button-primary ms-button-xs" onClick={activate}>Активировать</button>
                    <button className="ms-button ms-button-ghost ms-button-xs" onClick={() => setActivation(null)}>Отмена</button>
                  </div>
                </div>
              ) : (
                <button className="ms-button ms-button-primary ms-button-xs" onClick={() => startActivate(p)}>Активировать</button>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
