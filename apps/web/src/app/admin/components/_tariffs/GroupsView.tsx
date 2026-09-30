"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function GroupsView() {
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
  <h2>Группы тарифов</h2>
  <form className="ms-admin-form" onSubmit={createGroup}>
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "1fr 1fr",
        gap: 12,
      }}
      className="ms-groups-grid"
    >
      <label>
        Название
        <input
          placeholder="Название группы"
          value={groupForm.name}
          onChange={(e) =>
            setGroupForm((v) => ({ ...v, name: e.target.value }))
          }
        />
      </label>
      <label>
        Описание
        <input
          placeholder="Краткое описание"
          value={groupForm.description}
          onChange={(e) =>
            setGroupForm((v) => ({
              ...v,
              description: e.target.value,
            }))
          }
        />
      </label>
    </div>
    <label className="ms-inline-check">
      <input
        type="checkbox"
        checked={groupForm.availableForSubscription}
        onChange={(e) =>
          setGroupForm((v) => ({
            ...v,
            availableForSubscription: e.target.checked,
          }))
        }
      />{" "}
      Доступна для подписки
    </label>
    <button className="ms-button ms-button-primary ms-button-xs">
      Создать группу
    </button>
  </form>

  <div className="ms-admin-list" style={{ marginTop: 16 }}>
    {groups.map((g) => (
      <button
        key={g.id}
        className={
          selectedGroup?.id === g.id ? "is-active" : ""
        }
        onClick={() => setSelectedGroup(g)}
      >
        <strong>{g.name}</strong>
        <span>{g.description || "—"}</span>
      </button>
    ))}
    {groups.length === 0 && (
      <p className="muted">Групп ещё нет</p>
    )}
  </div>

  {selectedGroup && (
    <form
      className="ms-admin-form ms-editor-card"
      onSubmit={saveGroup}
    >
      <h3 style={{ margin: "0 0 12px" }}>
        Редактировать группу #{selectedGroup.id}
      </h3>
      <label>
        Название
        <input
          value={selectedGroup.name}
          onChange={(e) =>
            setSelectedGroup(
              (v: any) => v && { ...v, name: e.target.value }
            )
          }
        />
      </label>
      <label>
        Описание
        <textarea
          rows={2}
          value={selectedGroup.description || ""}
          onChange={(e) =>
            setSelectedGroup(
              (v: any) =>
                v && { ...v, description: e.target.value }
            )
          }
        />
      </label>
      <label className="ms-inline-check">
        <input
          type="checkbox"
          checked={selectedGroup.availableForSubscription}
          onChange={(e) =>
            setSelectedGroup(
              (v: any) =>
                v && {
                  ...v,
                  availableForSubscription: e.target.checked,
                }
            )
          }
        />{" "}
        Доступна для подписки
      </label>
      <div className="ms-admin-mini-actions">
        <button className="ms-button ms-button-primary ms-button-xs">
          Сохранить группу
        </button>
        <button
          type="button"
          className="ms-button ms-button-ghost ms-button-xs"
          onClick={() =>
            selectedGroup.id !== undefined && deleteGroup(selectedGroup.id, selectedGroup.name || "")
          }
        >
          Удалить группу
        </button>
      </div>
    </form>
  )}
</div>
  );
}
