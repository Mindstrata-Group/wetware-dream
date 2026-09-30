"use client";

import { useAdminPageContext } from "./AdminPageContext";
import { CreateForm } from "./_tariffs/CreateForm";
import { GroupsView } from "./_tariffs/GroupsView";
import { ListView } from "./_tariffs/ListView";

export function AdminTariffsTab() {
  const { tariffSection, setTariffSection } = useAdminPageContext();

  return (
    <div className="ms-admin-stack">
      <div className="ms-admin-mini-actions">
        <button
          className={tariffSection === "list" ? "is-active" : ""}
          onClick={() => setTariffSection("list")}
        >
          Управление тарифами
        </button>
        <button
          className={tariffSection === "groups" ? "is-active" : ""}
          onClick={() => setTariffSection("groups")}
        >
          Группы тарифов
        </button>
        <button
          className={tariffSection === "create" ? "is-active" : ""}
          onClick={() => setTariffSection("create")}
        >
          Создать тариф
        </button>
      </div>

      {tariffSection === "list" && <ListView />}
      {tariffSection === "groups" && <GroupsView />}
      {tariffSection === "create" && <CreateForm />}

      <style>{`
        @media (max-width: 720px) {
          .ms-tariffs-desktop { display: none !important; }
          .ms-tariffs-mobile { display: block !important; }
          .ms-tariff-edit-grid,
          .ms-tariff-create-grid,
          .ms-groups-grid {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
