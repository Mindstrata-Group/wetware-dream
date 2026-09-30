"use client";

import { useAdminPageContext } from "../AdminPageContext";
import { DemoChatJSONField } from "./DemoChatJSONField";

export function GuardrailPanel() {
  const {
    modes, modeMeta, modeDetail, setModeDetail, modeQ, setModeQ, newMode, setNewMode,
    guardrail, setGuardrail, modelStats, modePage, modeTotalPages, loadModes, loadModelStats,
    openMode, saveMode, createMode, testModeModel, detachModeFromPaidTariffs,
    cancelDeleteModeHold, startDeleteModeHold, applyGuardrail, testGuardrailModel, pageSizes,
  } = useAdminPageContext();

  return (
<div className="card ms-admin-card">
  <h2>Защитный блок и стиль ответа</h2>
  <p className="muted" style={{ marginTop: 0 }}>
    Работает для всех режимов автоматически: при отправке сообщения
    этот блок добавляется в конец системного промпта, не
    переписывая сами режимы. Здесь можно держать общий тон, формат
    ответа и защитные правила.
  </p>

  <div className="ms-admin-form">
    <textarea
      rows={6}
      value={guardrail.text}
      onChange={(e) =>
        setGuardrail((v) => ({ ...v, text: e.target.value }))
      }
    />

    <label style={{ fontWeight: 500 }}>
      Модель для теста (оставьте пустым — возьмёт текущий режим)
      <input
        value={guardrail.testModel}
        onChange={(e) =>
          setGuardrail((v) => ({
            ...v,
            testModel: e.target.value,
          }))
        }
        placeholder="openai/gpt-4o-mini"
        style={{ fontFamily: "monospace", fontSize: 13 }}
      />
    </label>

    {/* Bulk model replacement */}
    <label className="ms-inline-check">
      <input
        type="checkbox"
        checked={guardrail.replaceAll}
        onChange={(e) =>
          setGuardrail((v) => ({
            ...v,
            replaceAll: e.target.checked,
          }))
        }
      />{" "}
      Массово заменить модель во всех режимах
    </label>

    {guardrail.replaceAll && (
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          gap: 12,
        }}
        className="ms-guardrail-replace-grid"
      >
        <label style={{ fontWeight: 500 }}>
          Что заменить
          <select
            value={guardrail.find}
            onChange={(e) =>
              setGuardrail((v) => ({ ...v, find: e.target.value }))
            }
          >
            <option value="">
              Выберите существующую модель
            </option>
            {modelStats.map((stat) => (
              <option key={stat.model} value={stat.model}>
                {stat.model} · {stat.total} режимов
              </option>
            ))}
          </select>
        </label>
        <label style={{ fontWeight: 500 }}>
          На что заменить
          <input
            value={guardrail.replacement}
            onChange={(e) =>
              setGuardrail((v) => ({
                ...v,
                replacement: e.target.value,
              }))
            }
            placeholder="openai/gpt-4o-mini"
            style={{ fontFamily: "monospace", fontSize: 13 }}
          />
        </label>
      </div>
    )}

    <div className="ms-admin-mini-actions">
      <button
        type="button"
        className="ms-button ms-button-primary ms-button-xs"
        onClick={applyGuardrail}
      >
        Сохранить / применить массовую замену
      </button>
      <button
        type="button"
        className="ms-button ms-button-ghost ms-button-xs"
        onClick={testGuardrailModel}
      >
        Тест: Привет
      </button>
      <button
        type="button"
        className="ms-button ms-button-ghost ms-button-xs"
        onClick={loadModelStats}
      >
        Обновить статистику моделей
      </button>
    </div>

    {/* Models table */}
    {modelStats.length > 0 && (
      <div className="ms-table-scroll" style={{ marginTop: 8 }}>
        <table className="ms-admin-table">
          <thead>
            <tr>
              <th>Модель</th>
              <th>Всего</th>
              <th>Видимых</th>
              <th>Скрытых</th>
            </tr>
          </thead>
          <tbody>
            {modelStats.map((stat) => (
              <tr key={stat.model}>
                <td
                  style={{
                    fontFamily: "monospace",
                    fontSize: 13,
                  }}
                >
                  {stat.model}
                </td>
                <td style={{ fontVariantNumeric: "tabular-nums" }}>
                  {stat.total}
                </td>
                <td style={{ fontVariantNumeric: "tabular-nums" }}>
                  {stat.visible}
                </td>
                <td style={{ fontVariantNumeric: "tabular-nums" }}>
                  {stat.hidden}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )}
  </div>
</div>
  );
}
