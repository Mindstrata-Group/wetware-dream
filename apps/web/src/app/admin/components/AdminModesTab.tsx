"use client";

import { useAdminPageContext } from "./AdminPageContext";
import { CreatePanel } from "./_modes/CreatePanel";
import { EditPanel } from "./_modes/EditPanel";
import { GuardrailPanel } from "./_modes/GuardrailPanel";

export function AdminModesTab() {
  const { modePanel, setModePanel } = useAdminPageContext();

  return (
    <div className="ms-admin-stack">
      <div className="ms-admin-mini-actions ms-mode-subtabs">
        <button className={modePanel === "edit" ? "is-active" : ""} onClick={() => setModePanel("edit")}>
          Редактирование
        </button>
        <button className={modePanel === "create" ? "is-active" : ""} onClick={() => setModePanel("create")}>
          Создать режим
        </button>
        <button className={modePanel === "guardrail" ? "is-active" : ""} onClick={() => setModePanel("guardrail")}>
          Защитный блок
        </button>
      </div>

      {modePanel === "edit" && <EditPanel />}
      {modePanel === "create" && <CreatePanel />}
      {modePanel === "guardrail" && <GuardrailPanel />}

      <style>{`
        @media (max-width: 900px) {
          .ms-modes-edit-grid {
            grid-template-columns: 1fr !important;
          }
        }
        @media (max-width: 640px) {
          .ms-modes-create-grid,
          .ms-guardrail-replace-grid {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
