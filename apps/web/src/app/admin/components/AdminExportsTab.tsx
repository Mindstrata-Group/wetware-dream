"use client";

import { ExportSettings } from "./_exports/ExportSettings";
import { FilterColumns } from "./_exports/FilterColumns";
import { ResultsView } from "./_exports/ResultsView";

export function AdminExportsTab() {
  return (
    <div className="card ms-admin-card">
      <h2>Экспорт сообщений</h2>

      <div className="ms-admin-form">
        <FilterColumns />
        <ExportSettings />
        <ResultsView />
      </div>

      <style>{`
        @media (max-width: 720px) {
          .ms-export-settings-grid,
          .ms-export-actions-grid {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
