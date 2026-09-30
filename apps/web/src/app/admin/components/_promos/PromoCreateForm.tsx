"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function PromoCreateForm() {
  const { grantModeOptions, tariffs, lastCreatedPromos, promoQ, setPromoQ, promoSection, setPromoSection,
    promoActivation, setPromoActivation, bulkPromoDeactivate, setBulkPromoDeactivate,
    promoCreating, promoForm, setPromoForm, activeTariffs, activePromos, expiredPromos,
    loadPromocodes, modeSelectionToolbar, toggleModeId, createPromocodes,
    deactivatePromocode, activatePromocode, bulkDeactivatePromocodes,
    createdPromosTable, copyCreatedPromos, downloadCreatedPromos,
    copy, PromoColumn, modeNamesByIds, monthAheadLocalDate, formatDate, QRImage } = useAdminPageContext();
  return (
    <>
      {/* ══ CREATE FORM ══ */}
      <div className="card ms-admin-card">
        <h2>Создание промокодов</h2>
        <form className="ms-admin-form" onSubmit={createPromocodes}>

          <p className="muted" style={{ marginTop: 0, fontSize: 13 }}>
            Код генерируется автоматически. 30 — дней доступа, 1 —
            одно использование.
          </p>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: 12,
            }}
            className="ms-promo-create-grid"
          >
            <label>
              Комментарий для админа
              <input
                placeholder="Для кого/где выдан"
                value={promoForm.comment}
                onChange={(e) =>
                  setPromoForm((v) => ({ ...v, comment: e.target.value }))
                }
              />
            </label>

            <label>
              Сколько кодов создать
              <input
                type="number"
                min="1"
                max="200"
                value={promoForm.bulkCount}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    bulkCount: e.target.value,
                  }))
                }
              />
            </label>
          </div>

          <label>
            Назначение промокода
            <textarea
              rows={2}
              placeholder="Для чего / сценарий промокода"
              value={promoForm.purpose}
              onChange={(e) =>
                setPromoForm((v) => ({ ...v, purpose: e.target.value }))
              }
            />
          </label>

          <label>
            Что выдаёт промокод
            <select
              value={promoForm.grantsType}
              onChange={(e) =>
                setPromoForm((v) => ({
                  ...v,
                  grantsType: e.target.value,
                  targetIds: [],
                  firstModeId: 0,
                }))
              }
            >
              <option value="tariff">Тариф или несколько</option>
              <option value="mode">Режим</option>
              <option value="admin_role">Админ-доступ</option>
            </select>
          </label>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr",
              gap: 12,
            }}
            className="ms-promo-create-grid"
          >
            <label>
              Можно активировать с
              <input
                type="date"
                value={promoForm.activeFrom}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    activeFrom: e.target.value,
                  }))
                }
              />
            </label>
            <label>
              Можно активировать до
              <input
                type="date"
                value={promoForm.activeTo}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    activeTo: e.target.value,
                  }))
                }
              />
            </label>
          </div>

          <div
            className="ms-info-box"
            style={{ fontSize: 12, padding: "8px 12px" }}
          >
            Дата выше — окно, когда промокод можно ввести или открыть по
            ссылке. Сам доступ начнётся после активации и будет действовать
            указанное ниже число дней.
          </div>

          <div
            style={{
              display: "grid",
              gridTemplateColumns: "1fr 1fr 1fr",
              gap: 12,
            }}
            className="ms-promo-sliders-grid"
          >
            <label>
              Длительность доступа: {promoForm.durationDays} дней
              <input
                type="range"
                min="1"
                max="365"
                value={promoForm.durationDays}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    durationDays: e.target.value,
                  }))
                }
              />
            </label>

            <label>
              Использований: {promoForm.maxUses}
              <input
                type="range"
                min="1"
                max="500"
                value={promoForm.maxUses}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    maxUses: e.target.value,
                  }))
                }
              />
            </label>

            <label>
              Дневной лимит: {promoForm.dailyMessageLimit}
              <input
                type="range"
                min="1"
                max="500"
                value={promoForm.dailyMessageLimit}
                onChange={(e) =>
                  setPromoForm((v) => ({
                    ...v,
                    dailyMessageLimit: e.target.value,
                  }))
                }
              />
            </label>
          </div>

          <label>
            Лимит резюмирований:{" "}
            {promoForm.summaryLimit === "11" ? "∞" : promoForm.summaryLimit}
            <input
              type="range"
              min="0"
              max="11"
              value={promoForm.summaryLimit}
              onChange={(e) =>
                setPromoForm((v) => ({
                  ...v,
                  summaryLimit: e.target.value,
                }))
              }
            />
            <small>
              Крайнее правое положение = без ограничений (∞)
            </small>
          </label>

          <label className="ms-inline-check">
            <input
              type="checkbox"
              checked={promoForm.temporaryAdmin}
              onChange={(e) =>
                setPromoForm((v) => ({
                  ...v,
                  temporaryAdmin: e.target.checked,
                }))
              }
            />{" "}
            Создать временную админку по ссылке промокода
          </label>

          {/* Choice of tariffs or modes */}
          {promoForm.grantsType === "mode" ? (
            <>
              <div style={{ fontSize: 13 }}>
                Режимы, которые будут доступны по промокоду
              </div>
              {modeSelectionToolbar("promo")}
              <div className="ms-mode-checkboxes">
                {grantModeOptions.map((m) => (
                  <label key={m.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <input
                      type="checkbox"
                      checked={promoForm.targetIds.includes(m.id)}
                      onChange={() => {
                        toggleModeId("promo", m.id);
                        if (promoForm.firstModeId === m.id)
                          setPromoForm((v) => ({ ...v, firstModeId: 0 }));
                      }}
                    />
                    <span style={{ flex: 1 }}>{m.name}</span>
                    {promoForm.targetIds.includes(m.id) && (
                      <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", alignItems: "center", gap: 3, cursor: "pointer" }}>
                        <input
                          type="radio"
                          name="promo_first"
                          checked={promoForm.firstModeId === m.id}
                          onChange={() => setPromoForm((v) => ({ ...v, firstModeId: m.id }))}
                        />
                        первый
                      </label>
                    )}
                  </label>
                ))}
              </div>
            </>
          ) : promoForm.grantsType !== "admin_role" ? (
            <>
              <div style={{ fontSize: 13 }}>
                Тарифы, которые будут доступны по промокоду
              </div>
              <div className="ms-mode-checkboxes">
                {activeTariffs.map((t) => (
                  <label key={t.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
                    <input
                      type="checkbox"
                      checked={promoForm.targetIds.includes(t.id)}
                      onChange={() =>
                        setPromoForm((v) => ({
                          ...v,
                          targetIds: v.targetIds.includes(t.id)
                            ? v.targetIds.filter((id) => id !== t.id)
                            : [...v.targetIds, t.id],
                        }))
                      }
                    />
                    <span className="ms-ellipsis" style={{ flex: 1 }} title={t.name}>
                      {t.name} · {t.monthlyPrice} ₽
                      {t.groupName ? ` · ${t.groupName}` : ""}
                    </span>
                  </label>
                ))}
              </div>
            </>
          ) : (
            <div className="ms-info-box" style={{ fontSize: 13 }}>
              Промокод выдаёт роль admin. Пользователь должен быть
              авторизован через Яндекс.
            </div>
          )}

          <button
            className="ms-button ms-button-primary ms-button-xs"
            disabled={promoCreating}
          >
            {promoCreating
              ? "Создаём промокоды..."
              : "Создать промокоды"}
          </button>
        </form>
      </div>

    </>
  );
}
