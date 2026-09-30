"use client";

import { useAdminPageContext } from "../AdminPageContext";

export function PromoCreateResult() {
  const { modes, tariffs, lastCreatedPromos, promoQ, setPromoQ, promoSection, setPromoSection,
    promoActivation, setPromoActivation, bulkPromoDeactivate, setBulkPromoDeactivate,
    promoCreating, promoForm, setPromoForm, activeTariffs, activePromos, expiredPromos,
    loadPromocodes, modeSelectionToolbar, modeCheckboxes, createPromocodes,
    deactivatePromocode, activatePromocode, bulkDeactivatePromocodes,
    createdPromosTable, copyCreatedPromos, downloadCreatedPromos,
    copy, PromoColumn, modeNamesByIds, monthAheadLocalDate, formatDate, QRImage } = useAdminPageContext();
  return (
    <>
      {/* ── Creation result ── */}
      {lastCreatedPromos.length > 1 ? (
        <div className="ms-info-box ms-editor-card">
          <strong>
            Пакет созданных промокодов ({lastCreatedPromos.length} шт.)
          </strong>
          <p
            className="muted"
            style={{ fontSize: 13, margin: "4px 0 0" }}
          >
            Файл скачивается с разделителем «;», чтобы Excel открывал
            колонки сразу.
          </p>
          <div className="ms-admin-mini-actions">
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={copyCreatedPromos}
            >
              Скопировать таблицу
            </button>
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={downloadCreatedPromos}
            >
              Скачать для Excel
            </button>
          </div>
          <textarea
            rows={Math.min(8, lastCreatedPromos.length + 2)}
            readOnly
            value={createdPromosTable()}
            style={{ fontFamily: "monospace", fontSize: 12 }}
          />
        </div>
      ) : lastCreatedPromos.length === 1 ? (
        <div className="ms-info-box ms-editor-card">
          <strong>
            Промокод создан: {lastCreatedPromos[0].code}
          </strong>
          <div className="ms-admin-mini-actions">
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={() =>
                copy(
                  lastCreatedPromos[0].applyUrl ||
                    lastCreatedPromos[0].code
                )
              }
            >
              Скопировать ссылку
            </button>
          </div>
          {lastCreatedPromos[0].applyUrl && (
            <div style={{ marginTop: 8 }}>
              <QRImage
                code={lastCreatedPromos[0].code}
                url={lastCreatedPromos[0].applyUrl}
              />
            </div>
          )}
        </div>
      ) : null}

    </>
  );
}
