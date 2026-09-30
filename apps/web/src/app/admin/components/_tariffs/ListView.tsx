"use client";

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { useAdminPageContext } from "../AdminPageContext";
import { ModelPicker } from "../ModelPicker";

export function ListView() {
  const {
    groups,
    selectedTariff,
    setSelectedTariff,
    selectedTariffIds,
    setSelectedTariffIds,
    tariffArchiveView,
    setTariffArchiveView,
    tariffForm,
    setTariffForm,
    groupForm,
    setGroupForm,
    selectedGroup,
    setSelectedGroup,
    tariffSort,
    setTariffSort,
    activeTariffs,
    archivedTariffs,
    visibleTariffs,
    loadTariffs,
    modeCheckboxes,
    grantModeOptions,
    createTariff,
    saveTariff,
    applyTariffModesAI,
    createGroup,
    saveGroup,
    restoreTariff,
    deleteTariff,
    deleteSelectedTariffs,
    deleteGroup,
  } = useAdminPageContext();

  // Bulk AI change for all modes of a tariff: provider + model are chosen
  // locally here and applied with a separate button (not "Save tariff").
  const [bulkAI, setBulkAI] = useState({ provider: "anthropic", model: "" });
  // Live list of the provider's models, same pattern as in the mode editor
  // (_modes/EditPanel): while there is no response or the provider is vsegpt, a static
  // fallback list or manual input.
  const [fetchedAIModels, setFetchedAIModels] = useState<Record<string, string[]>>({});
  useEffect(() => {
    if (!selectedTariff || bulkAI.provider === "vsegpt" || fetchedAIModels[bulkAI.provider]) return;
    let cancelled = false;
    apiFetch<{ ok: boolean; models: { id: string; displayName: string }[] }>(
      `/api/admin/ai-models?provider=${bulkAI.provider}`
    )
      .then((json) => {
        if (cancelled || !json.models?.length) return;
        setFetchedAIModels((v) => ({ ...v, [bulkAI.provider]: json.models.map((m) => m.id) }));
      })
      .catch(() => {
        /* an auto-load failure is not critical: the static fallback list remains */
      });
    return () => {
      cancelled = true;
    };
  }, [selectedTariff, bulkAI.provider, fetchedAIModels]);
  const bulkAIModelOptions =
    fetchedAIModels[bulkAI.provider] ??
    (bulkAI.provider === "gemini"
      ? ["gemini-3.5-flash-minimal", "gemini-3.1-flash-lite", "gemini-3.1-pro"]
      : bulkAI.provider === "anthropic"
        ? ["claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"]
        : []);

  return (
<div className="card ms-admin-card">
  <div className="ms-admin-card-head">
    <h2>Тарифы</h2>
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <select
        className="ms-button ms-button-ghost ms-button-xs"
        value={tariffSort}
        onChange={(e) => {
          setTariffSort(e.target.value);
          void loadTariffs(e.target.value);
        }}
      >
        <option value="created">по дате создания</option>
        <option value="price">по стоимости</option>
        <option value="group">по группе</option>
        <option value="type">по типу</option>
      </select>
    </div>
  </div>

  {/* Active/archive toggle + bulk archive */}
  <div className="ms-admin-mini-actions">
    <button
      className={tariffArchiveView === "active" ? "is-active" : ""}
      onClick={() => {
        setTariffArchiveView("active");
        setSelectedTariffIds([]);
      }}
    >
      Активные ({activeTariffs.length})
    </button>
    <button
      className={tariffArchiveView === "archive" ? "is-active" : ""}
      onClick={() => {
        setTariffArchiveView("archive");
        setSelectedTariffIds([]);
      }}
    >
      Архив ({archivedTariffs.length})
    </button>
    {selectedTariffIds.length > 0 &&
      tariffArchiveView === "active" && (
        <button
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={deleteSelectedTariffs}
        >
          В архив выбранные ({selectedTariffIds.length})
        </button>
      )}
  </div>

  {/* Desktop table */}
  <div className="ms-table-scroll ms-tariffs-desktop">
    <table className="ms-admin-table">
      <thead>
        <tr>
          <th style={{ width: 36 }}>
            <input
              type="checkbox"
              disabled={tariffArchiveView !== "active"}
              checked={
                tariffArchiveView === "active" &&
                visibleTariffs.length > 0 &&
                visibleTariffs.every((t) =>
                  selectedTariffIds.includes(t.id)
                )
              }
              onChange={(e) =>
                setSelectedTariffIds(
                  e.target.checked
                    ? visibleTariffs.map((t) => t.id)
                    : []
                )
              }
            />
          </th>
          <th>Название</th>
          <th>Тип</th>
          <th>Цена</th>
          <th>Группа</th>
          <th>Статус</th>
          <th>Действия</th>
        </tr>
      </thead>
      <tbody>
        {visibleTariffs.map((t) => (
          <tr key={t.id}>
            <td>
              <input
                type="checkbox"
                disabled={tariffArchiveView !== "active"}
                checked={selectedTariffIds.includes(t.id)}
                onChange={(e) =>
                  setSelectedTariffIds((ids) =>
                    e.target.checked
                      ? [...ids, t.id]
                      : ids.filter((id) => id !== t.id)
                  )
                }
              />
            </td>
            <td>
              <button
                className="ms-link-button"
                onClick={() => setSelectedTariff(t)}
              >
                {t.name}
              </button>
            </td>
            <td style={{ color: "var(--muted)", fontSize: 13 }}>
              {t.tariffType}
            </td>
            <td style={{ fontVariantNumeric: "tabular-nums" }}>
              {t.monthlyPrice.toLocaleString("ru-RU")} ₽
            </td>
            <td style={{ color: "var(--muted)", fontSize: 13 }}>
              {t.groupName || "—"}
            </td>
            <td>
              <span
                className="ms-chip"
                style={{
                  fontSize: 11,
                  background:
                    t.archivedAt || !t.availableForSubscription
                      ? "var(--soft)"
                      : "#edf8f5",
                  color:
                    t.archivedAt || !t.availableForSubscription
                      ? "var(--muted)"
                      : "var(--accent-strong)",
                  borderColor:
                    t.archivedAt || !t.availableForSubscription
                      ? "var(--line)"
                      : "#bee1d7",
                }}
              >
                {t.archivedAt || !t.availableForSubscription
                  ? "в архиве"
                  : "активен"}
              </span>
            </td>
            <td>
              <div
                style={{ display: "flex", gap: 6, flexWrap: "wrap" }}
              >
                {t.archivedAt || !t.availableForSubscription ? (
                  <>
                    <button
                      type="button"
                      className="ms-button ms-button-primary ms-button-xs"
                      onClick={() => restoreTariff(t)}
                    >
                      Восстановить
                    </button>
                    <button
                      type="button"
                      className="ms-button ms-button-ghost ms-button-xs"
                      onClick={() => deleteTariff(t.id, true)}
                    >
                      Удалить
                    </button>
                  </>
                ) : (
                  <button
                    type="button"
                    className="ms-button ms-button-ghost ms-button-xs"
                    onClick={() => deleteTariff(t.id)}
                  >
                    В архив
                  </button>
                )}
              </div>
            </td>
          </tr>
        ))}
        {visibleTariffs.length === 0 && (
          <tr>
            <td
              colSpan={7}
              style={{
                textAlign: "center",
                color: "var(--muted)",
                padding: 20,
              }}
            >
              Тарифов нет
            </td>
          </tr>
        )}
      </tbody>
    </table>
  </div>

  {/* Mobile cards */}
  <div className="ms-tariffs-mobile" style={{ display: "none" }}>
    {visibleTariffs.map((t) => (
      <div
        key={t.id}
        style={{
          border: "1px solid var(--line)",
          borderRadius: 14,
          padding: 14,
          marginBottom: 8,
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
          <button
            className="ms-link-button"
            onClick={() => setSelectedTariff(t)}
            style={{ fontSize: 14 }}
          >
            {t.name}
          </button>
          <span
            className="ms-chip"
            style={{ fontSize: 11 }}
          >
            {t.archivedAt || !t.availableForSubscription
              ? "архив"
              : "активен"}
          </span>
        </div>
        <div
          style={{
            fontSize: 12,
            color: "var(--muted)",
            marginBottom: 8,
          }}
        >
          {t.tariffType} · {t.monthlyPrice.toLocaleString("ru-RU")} ₽
          · {t.groupName || "без группы"}
        </div>
        <div style={{ display: "flex", gap: 6 }}>
          {t.archivedAt || !t.availableForSubscription ? (
            <>
              <button
                className="ms-button ms-button-primary ms-button-xs"
                onClick={() => restoreTariff(t)}
              >
                Восстановить
              </button>
              <button
                className="ms-button ms-button-ghost ms-button-xs"
                onClick={() => deleteTariff(t.id, true)}
              >
                Удалить
              </button>
            </>
          ) : (
            <button
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => deleteTariff(t.id)}
            >
              В архив
            </button>
          )}
        </div>
      </div>
    ))}
  </div>

  {/* ── Editor of the selected tariff ── */}
  {selectedTariff && (
    <form
      className="ms-admin-form ms-editor-card"
      onSubmit={saveTariff}
    >
      <h3 style={{ margin: "0 0 12px" }}>
        Редактировать тариф #{selectedTariff.id}
      </h3>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          gap: 12,
        }}
        className="ms-tariff-edit-grid"
      >
        <label>
          Название
          <input
            value={selectedTariff.name}
            onChange={(e) =>
              setSelectedTariff(
                (v) => v && { ...v, name: e.target.value }
              )
            }
          />
        </label>

        <label>
          Тип
          <select
            value={selectedTariff.tariffType}
            onChange={(e) =>
              setSelectedTariff(
                (v) => v && { ...v, tariffType: e.target.value }
              )
            }
          >
            <option value="regular">обычный</option>
            <option value="promo">промо one-time</option>
          </select>
        </label>

        <label>
          Цена в месяц, ₽
          <input
            type="number"
            value={selectedTariff.monthlyPrice}
            onChange={(e) =>
              setSelectedTariff(
                (v) =>
                  v && {
                    ...v,
                    monthlyPrice: Number(e.target.value),
                  }
              )
            }
          />
        </label>

        <label>
          Группа
          <select
            value={selectedTariff.groupId || ""}
            onChange={(e) =>
              setSelectedTariff(
                (v) =>
                  v && {
                    ...v,
                    groupId: e.target.value
                      ? Number(e.target.value)
                      : null,
                  }
              )
            }
          >
            <option value="">Без группы</option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label>
        Описание
        <textarea
          rows={2}
          value={selectedTariff.description || ""}
          onChange={(e) =>
            setSelectedTariff(
              (v) => v && { ...v, description: e.target.value }
            )
          }
        />
      </label>

      <label>
        Дневной лимит сообщений:{" "}
        {selectedTariff.dailyMessageLimit ?? 0}
        <input
          type="range"
          min="0"
          max="500"
          step="10"
          value={selectedTariff.dailyMessageLimit ?? 0}
          onChange={(e) =>
            setSelectedTariff(
              (v) =>
                v && {
                  ...v,
                  dailyMessageLimit: Number(e.target.value),
                }
            )
          }
        />
      </label>

      <label className="ms-inline-check">
        <input
          type="checkbox"
          checked={selectedTariff.availableForSubscription}
          onChange={(e) =>
            setSelectedTariff(
              (v) =>
                v && {
                  ...v,
                  availableForSubscription: e.target.checked,
                }
            )
          }
        />{" "}
        Доступен для подписки (снять — переход в архив)
      </label>

      <div style={{ fontSize: 13 }}>
        Режимы тарифа
      </div>
      <div className="ms-mode-checkboxes">
        {grantModeOptions.map((m) => (
          <label key={m.id} style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <input
              type="checkbox"
              checked={(selectedTariff.modeIds || []).includes(m.id)}
              onChange={() =>
                setSelectedTariff(
                  (v) =>
                    v && {
                      ...v,
                      modeIds: (v.modeIds || []).includes(m.id)
                        ? (v.modeIds || []).filter((id) => id !== m.id)
                        : [...(v.modeIds || []), m.id],
                      firstModeId:
                        v.firstModeId === m.id &&
                        (v.modeIds || []).includes(m.id)
                          ? null
                          : v.firstModeId,
                    }
                )
              }
            />
            <span style={{ flex: 1 }}>
              {m.name}
              {m.paidChains ? (
                <small>{m.paidChains}</small>
              ) : null}
            </span>
            {(selectedTariff.modeIds || []).includes(m.id) && (
              <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", alignItems: "center", gap: 3, cursor: "pointer" }}>
                <input
                  type="radio"
                  name="tariff_first_mode"
                  checked={selectedTariff.firstModeId === m.id}
                  onChange={() =>
                    setSelectedTariff((v) => v && { ...v, firstModeId: m.id })
                  }
                />
                ведущий
              </label>
            )}
          </label>
        ))}
      </div>

      {/* ── AI for all modes of the tariff at once ── */}
      <div
        style={{
          border: "1px solid var(--line)",
          borderRadius: 12,
          padding: 12,
          display: "grid",
          gap: 8,
        }}
      >
        <div style={{ fontSize: 13 }}>
          AI всех режимов тарифа
          <span style={{ color: "var(--muted)", fontSize: 12 }}>
            {" "}
            — применяется сразу ко всем режимам из состава ({(selectedTariff.modeIds || []).length}), кнопкой ниже, без «Сохранить тариф»
          </span>
        </div>
        <div
          style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}
        >
          <select
            aria-label="Провайдер для всех режимов"
            value={bulkAI.provider}
            onChange={(e) =>
              setBulkAI({ provider: e.target.value, model: "" })
            }
          >
            <option value="anthropic">anthropic</option>
            <option value="gemini">gemini</option>
            <option value="vsegpt">vsegpt</option>
          </select>
          <div style={{ flex: 1, minWidth: 220 }}>
            <ModelPicker
              ariaLabel="Модель для всех режимов"
              value={bulkAI.model}
              onChange={(model) => setBulkAI((v) => ({ ...v, model }))}
              options={bulkAIModelOptions}
              placeholder="например claude-haiku-4-5"
            />
          </div>
          <button
            type="button"
            className="ms-button ms-button-primary ms-button-xs"
            disabled={!bulkAI.model.trim()}
            onClick={() => applyTariffModesAI(bulkAI.provider, bulkAI.model.trim())}
          >
            Применить ко всем режимам
          </button>
        </div>
      </div>

      <div className="ms-admin-mini-actions">
        <button className="ms-button ms-button-primary ms-button-xs">
          Сохранить тариф
        </button>
        {selectedTariff.archivedAt ||
        !selectedTariff.availableForSubscription ? (
          <button
            type="button"
            className="ms-button ms-button-primary ms-button-xs"
            onClick={() => restoreTariff(selectedTariff)}
          >
            Восстановить из архива
          </button>
        ) : null}
        <button
          type="button"
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={() =>
            deleteTariff(
              selectedTariff.id,
              Boolean(
                selectedTariff.archivedAt ||
                  !selectedTariff.availableForSubscription
              )
            )
          }
        >
          {selectedTariff.archivedAt ||
          !selectedTariff.availableForSubscription
            ? "Удалить окончательно"
            : "В архив"}
        </button>
      </div>
    </form>
  )}
</div>
  );
}
