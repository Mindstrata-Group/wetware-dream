"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function CreateForm() {
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
    createTariff,
    saveTariff,
    createGroup,
    saveGroup,
    restoreTariff,
    deleteTariff,
    deleteSelectedTariffs,
    deleteGroup,
  } = useAdminPageContext();

  return (
<div className="card ms-admin-card">
  <h2>Создать тариф</h2>
  <form className="ms-admin-form" onSubmit={createTariff}>
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "1fr 1fr",
        gap: 12,
      }}
      className="ms-tariff-create-grid"
    >
      <label>
        Название
        <input
          placeholder="Название тарифа"
          value={tariffForm.name}
          onChange={(e) =>
            setTariffForm((v) => ({
              ...v,
              name: e.target.value,
            }))
          }
        />
      </label>

      <label>
        Тип
        <select
          value={tariffForm.tariffType}
          onChange={(e) =>
            setTariffForm((v) => ({
              ...v,
              tariffType: e.target.value,
            }))
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
          placeholder="0"
          value={tariffForm.monthlyPrice}
          onChange={(e) =>
            setTariffForm((v) => ({
              ...v,
              monthlyPrice: e.target.value,
            }))
          }
        />
      </label>

      <label>
        Группа
        <select
          value={tariffForm.groupId}
          onChange={(e) =>
            setTariffForm((v) => ({
              ...v,
              groupId: e.target.value,
            }))
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
        placeholder="Краткое описание тарифа"
        value={tariffForm.description}
        onChange={(e) =>
          setTariffForm((v) => ({
            ...v,
            description: e.target.value,
          }))
        }
      />
    </label>

    <label>
      Дневной лимит сообщений: {tariffForm.dailyMessageLimit}
      <input
        type="range"
        min="0"
        max="500"
        step="10"
        value={tariffForm.dailyMessageLimit}
        onChange={(e) =>
          setTariffForm((v) => ({
            ...v,
            dailyMessageLimit: e.target.value,
          }))
        }
      />
    </label>

    <label className="ms-inline-check">
      <input
        type="checkbox"
        checked={tariffForm.availableForSubscription}
        onChange={(e) =>
          setTariffForm((v) => ({
            ...v,
            availableForSubscription: e.target.checked,
          }))
        }
      />{" "}
      Доступен для подписки
    </label>

    <div style={{ fontSize: 13 }}>
      Режимы тарифа
    </div>
    {modeCheckboxes("tariff", tariffForm.modeIds)}

    <button
      className="ms-button ms-button-primary ms-button-xs"
    >
      Создать тариф
    </button>
  </form>
</div>
  );
}
