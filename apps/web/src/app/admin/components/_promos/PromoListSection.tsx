"use client";

import { useAdminPageContext } from "../AdminPageContext";
import { promoEditInitialState } from "../../_hooks/adminBillingActions";
import { toLocalDateInput } from "../../utils";

export function PromoListSection() {
  const { modes, grantModeOptions, tariffs, lastCreatedPromos, promoQ, setPromoQ,
    promoDateFrom, setPromoDateFrom, promoDateTo, setPromoDateTo, promoMeta,
    promoSection, setPromoSection,
    promoActivation, setPromoActivation, promoEdit, setPromoEdit, bulkPromoDeactivate, setBulkPromoDeactivate,
    promoCreating, promoForm, setPromoForm, activeTariffs, activePromos, exhaustedPromos, expiredPromos,
    loadPromocodes, modeSelectionToolbar, modeCheckboxes, createPromocodes,
    deactivatePromocode, activatePromocode, savePromocodeEdit, bulkDeactivatePromocodes,
    createdPromosTable, copyCreatedPromos, downloadCreatedPromos,
    copy, PromoColumn, modeNamesByIds, monthAheadLocalDate, formatDate, QRImage } = useAdminPageContext();

  const promoPage = Math.floor(promoMeta.offset / promoMeta.limit) + 1;
  const promoTotalPages = Math.max(1, Math.ceil(promoMeta.total / promoMeta.limit));
  const listTitle = promoSection === "active"
    ? "Действующие"
    : promoSection === "exhausted"
      ? "Исчерпанные многоразовые"
      : "Неактивные / истёкшие";
  const listItems = promoSection === "active"
    ? activePromos
    : promoSection === "exhausted"
      ? exhaustedPromos
      : expiredPromos;
  const editActiveTo = (value?: string | null) => {
    if (!value) return "";
    const date = new Date(value);
    if (!Number.isFinite(date.getTime()) || date.getTime() < Date.now()) return monthAheadLocalDate();
    return toLocalDateInput(date);
  };
  return (
    <>
      {/* ── Search + toggle + bulk deactivation ── */}
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
        <input
          style={{
            flex: 1,
            minWidth: 220,
            minHeight: 42,
            border: "1px solid var(--line)",
            borderRadius: 13,
            background: "color-mix(in oklab, var(--card) 88%, transparent)",
            padding: "0 12px",
          }}
          placeholder="поиск по коду, комментарию, цели"
          value={promoQ}
          onChange={(e) => setPromoQ(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") void loadPromocodes(0); }}
        />
        <label style={{ display: "flex", flexDirection: "column", gap: 2, fontSize: 12, color: "var(--muted)" }}>
          Активен с
          <input
            type="date"
            value={promoDateFrom}
            style={{ minHeight: 38, border: "1px solid var(--line)", borderRadius: 10, padding: "0 8px", fontSize: 13 }}
            onChange={(e) => setPromoDateFrom(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void loadPromocodes(0); }}
          />
        </label>
        <label style={{ display: "flex", flexDirection: "column", gap: 2, fontSize: 12, color: "var(--muted)" }}>
          до
          <input
            type="date"
            value={promoDateTo}
            style={{ minHeight: 38, border: "1px solid var(--line)", borderRadius: 10, padding: "0 8px", fontSize: 13 }}
            onChange={(e) => setPromoDateTo(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") void loadPromocodes(0); }}
          />
        </label>
        <button
          type="button"
          className="ms-button ms-button-ghost ms-button-xs"
          style={{ alignSelf: "flex-end", height: 38 }}
          onClick={() => void loadPromocodes(0)}
        >
          Найти
        </button>
      </div>

      {promoMeta.total > promoMeta.limit && (
        <div style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 13 }}>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={promoMeta.offset === 0}
            onClick={() => void loadPromocodes(Math.max(0, promoMeta.offset - promoMeta.limit))}
          >
            ← Пред.
          </button>
          <span className="muted">стр. {promoPage} / {promoTotalPages} ({promoMeta.total} всего)</span>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            disabled={promoPage >= promoTotalPages}
            onClick={() => void loadPromocodes(promoMeta.offset + promoMeta.limit)}
          >
            След. →
          </button>
        </div>
      )}

      <div className="ms-admin-mini-actions">
        <button
          className={promoSection === "active" ? "is-active" : ""}
          onClick={() => setPromoSection("active")}
        >
          Действующие ({activePromos.length})
        </button>
        <button
          className={promoSection === "exhausted" ? "is-active" : ""}
          onClick={() => setPromoSection("exhausted")}
        >
          Исчерпанные ({exhaustedPromos.length})
        </button>
        <button
          className={promoSection === "expired" ? "is-active" : ""}
          onClick={() => setPromoSection("expired")}
        >
          Неактивные / истёкшие ({expiredPromos.length})
        </button>
      </div>

      {/* ── Bulk deactivation ── */}
      <div className="ms-info-box ms-editor-card">
        <strong>Массовая деактивация промокодов</strong>
        <small className="muted">
          Если даты не выбрать — деактивируются все активные промокоды.
        </small>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "1fr 1fr",
            gap: 12,
            marginTop: 10,
          }}
          className="ms-promo-bulk-grid"
        >
          <label>
            Дата создания от
            <input
              type="date"
              value={bulkPromoDeactivate.dateFrom}
              onChange={(e) =>
                setBulkPromoDeactivate((v) => ({
                  ...v,
                  dateFrom: e.target.value,
                }))
              }
            />
          </label>
          <label>
            Дата создания до
            <input
              type="date"
              value={bulkPromoDeactivate.dateTo}
              onChange={(e) =>
                setBulkPromoDeactivate((v) => ({
                  ...v,
                  dateTo: e.target.value,
                }))
              }
            />
          </label>
        </div>
        <button
          type="button"
          className="ms-button ms-button-ghost ms-button-xs"
          style={{ marginTop: 8 }}
          onClick={bulkDeactivatePromocodes}
        >
          Деактивировать промокоды по фильтру
        </button>
      </div>

      {/* ── Promo code list ── */}
      <PromoColumn
        title={listTitle}
        items={listItems}
        modes={grantModeOptions}
        tariffs={activeTariffs}
        copy={copy}
        deactivate={deactivatePromocode}
        startActivate={(p) =>
          setPromoActivation({
            id: p.id,
            activeTo: monthAheadLocalDate(),
            maxUses: String(
              Math.max(
                (p.maxUses || 0) + 1,
                (p.usedCount || 0) + 1
              )
            ),
          })
        }
        activation={promoActivation}
        setActivation={setPromoActivation}
        activate={activatePromocode}
        edit={promoEdit}
        setEdit={setPromoEdit}
        startEdit={(p) => setPromoEdit(promoEditInitialState(p, editActiveTo(p.activeTo)))}
        saveEdit={savePromocodeEdit}
        describeModes={(p) => {
          const targetIds = p.targetIds?.length ? p.targetIds : [p.targetId].filter(Boolean);
          if (p.grantsType === "mode")
            return modeNamesByIds(modes, targetIds);
          if (p.grantsType === "admin_role") return "роль admin";
          const names = targetIds.map((id) => {
            const tariff = tariffs.find((t) => t.id === id);
            return tariff
              ? `${tariff.name}: ${modeNamesByIds(modes, tariff.modeIds)}`
              : `тариф #${id} не найден`;
          });
          return names.length ? names.join("; ") : "тариф не найден";
        }}
      />

      <style>{`
        @media (max-width: 640px) {
          .ms-promo-create-grid,
          .ms-promo-sliders-grid,
          .ms-promo-bulk-grid {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </>
  );
}
