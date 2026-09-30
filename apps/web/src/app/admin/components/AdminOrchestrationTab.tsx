"use client";

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { useAdminPageContext } from "./AdminPageContext";
import { ModelPicker } from "./ModelPicker";
import { AdminAIGatewaysSection } from "./AdminAIGatewaysSection";

export function AdminOrchestrationTab() {
  const {
    summaryPrompts,
    orchestrationPanel,
    setOrchestrationPanel,
    orchestrationPrompt,
    setOrchestrationPrompt,
    dialogSummaryPrompt,
    setDialogSummaryPrompt,
    leadSummaryPrompt,
    setLeadSummaryPrompt,
    aiSettings,
    setAISettings,
    newPrompt,
    setNewPrompt,
    saveOrchestrationPrompt,
    saveDialogSummaryPrompt,
    saveLeadSummaryPrompt,
    saveAISettings,
    createSummaryPrompt,
    deleteSummaryPrompt,
    editSummaryPrompt,
  } = useAdminPageContext();

  // Live list of the provider's models (the same /api/admin/ai-models as in the
  // mode editor): a <select> dropdown via ModelPicker for
  // orchestration/summarisation/attachment annotation. Manual input stays
  // always available. The cache is shared per provider (not per mechanic); three fields
  // may point at the same provider.
  const [fetchedAIModels, setFetchedAIModels] = useState<Record<string, string[]>>({});
  const orchestrationProvider = aiSettings.orchestrationProvider;
  const leadSummaryProvider = aiSettings.leadSummaryProvider;
  const summaryProvider = aiSettings.summaryProvider;
  const attachmentAnnotationProvider = aiSettings.attachmentAnnotationProvider;
  useEffect(() => {
    let cancelled = false;
    const providers = new Set([orchestrationProvider, leadSummaryProvider, summaryProvider, attachmentAnnotationProvider]);
    for (const provider of providers) {
      if (provider === "vsegpt" || fetchedAIModels[provider]) continue;
      apiFetch<{ ok: boolean; models: { id: string; displayName: string }[] }>(
        `/api/admin/ai-models?provider=${provider}`
      )
        .then((json) => {
          if (cancelled || !json.models?.length) return;
          setFetchedAIModels((v) => ({ ...v, [provider]: json.models.map((m) => m.id) }));
        })
        .catch(() => {
          /* an auto-load failure is not critical: manual input without hints remains */
        });
    }
    return () => {
      cancelled = true;
    };
  }, [orchestrationProvider, leadSummaryProvider, summaryProvider, attachmentAnnotationProvider, fetchedAIModels]);

  return (
    <div className="card ms-admin-card">
      <h2>Оркестрация и резюмирование</h2>

      {/* Sub-tabs */}
      <div className="ms-admin-mini-actions ms-mode-subtabs" style={{ marginBottom: 16 }}>
        {[
          { id: "orchestration", label: "Оркестрация" },
          { id: "summary",       label: "Резюмирование" },
          { id: "dialogSummary", label: "Итог промпт" },
          { id: "leadSummary",   label: "Выжимка лида" },
          { id: "ai",            label: "AI" },
        ].map((t) => (
          <button
            key={t.id}
            className={orchestrationPanel === t.id ? "is-active" : ""}
            onClick={() => setOrchestrationPanel(t.id as any)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* ══ ORCHESTRATION ══ */}
      {orchestrationPanel === "orchestration" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <p className="muted" style={{ margin: 0, fontSize: 13 }}>
            Единый промпт оркестрации для всех режимов.
          </p>

          <div style={{ display: "grid", gridTemplateColumns: "140px 1fr 120px 180px", gap: 10 }} className="ms-orch-model-grid">
            <div>
              <div className="ms-field-label">Провайдер</div>
              <select
                value={aiSettings.orchestrationProvider}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, orchestrationProvider: e.target.value }))
                }
              >
                <option value="vsegpt">vsegpt</option>
                <option value="gemini">Gemini</option>
                <option value="anthropic">Claude</option>
              </select>
            </div>
            <div>
              <div className="ms-field-label">Модель оркестрации</div>
              <ModelPicker
                ariaLabel="Модель оркестрации"
                value={aiSettings.orchestrationModel}
                onChange={(next) =>
                  setAISettings((v) => ({ ...v, orchestrationModel: next }))
                }
                placeholder="openai/gpt-4o-mini"
                options={fetchedAIModels[orchestrationProvider] ?? []}
              />
            </div>
            <div>
              <div className="ms-field-label">Температура</div>
              <input
                type="number"
                min="0"
                max="2"
                step="0.1"
                value={aiSettings.orchestrationTemperature}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, orchestrationTemperature: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Окно контекста</div>
              <input
                type="number"
                min="1"
                max="50"
                step="1"
                value={aiSettings.orchestrationHistoryLimit}
                aria-label="Окно сообщений оркестратора"
                onChange={(e) => {
                  const value = e.target.value;
                  setAISettings((v) => ({ ...v, orchestrationHistoryLimit: value }));
                }}
              />
            </div>
          </div>

          <div>
            <div className="ms-field-label">Промпт</div>
            <textarea
              rows={7}
              value={orchestrationPrompt}
              onChange={(e) => setOrchestrationPrompt(e.target.value)}
            />
          </div>

          <div className="ms-admin-mini-actions">
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={saveOrchestrationPrompt}
            >
              Сохранить промпт оркестрации
            </button>
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => { void saveAISettings(); }}
            >
              Сохранить модель
            </button>
          </div>
        </div>
      )}

      {/* ══ SUMMARISATION ══ */}
      {orchestrationPanel === "summary" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>

          {/* Summarisation model */}
          <div className="ms-editor-card" style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{ fontSize: 13, fontWeight: 600 }}>Модель AI для сводок</div>
            <p className="muted" style={{ margin: 0, fontSize: 12 }}>
              Используется для сводок выгрузок, промо-админ сводок и итогов диалога в чате.
            </p>
            <div style={{ display: "grid", gridTemplateColumns: "140px 1fr 120px", gap: 10 }} className="ms-orch-model-grid">
              <div>
                <div className="ms-field-label">Провайдер</div>
                <select
                  value={aiSettings.summaryProvider}
                  onChange={(e) =>
                    setAISettings((v) => ({ ...v, summaryProvider: e.target.value }))
                  }
                >
                  <option value="vsegpt">vsegpt</option>
                  <option value="gemini">Gemini</option>
                  <option value="anthropic">Claude</option>
                </select>
              </div>
              <div>
                <div className="ms-field-label">Модель</div>
                <ModelPicker
                  ariaLabel="Модель резюмирования"
                  value={aiSettings.summaryModel}
                  onChange={(next) =>
                    setAISettings((v) => ({ ...v, summaryModel: next }))
                  }
                  placeholder="openai/gpt-4o-mini"
                  options={fetchedAIModels[summaryProvider] ?? []}
                />
              </div>
              <div>
                <div className="ms-field-label">Температура</div>
                <input
                  type="number"
                  min="0"
                  max="2"
                  step="0.1"
                  value={aiSettings.summaryTemperature}
                  onChange={(e) =>
                    setAISettings((v) => ({ ...v, summaryTemperature: e.target.value }))
                  }
                />
              </div>
            </div>
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => { void saveAISettings(); }}
            >
              Сохранить модель резюмирования
            </button>
          </div>

          {/* New / edited prompt */}
          <form
            onSubmit={createSummaryPrompt}
            className="ms-editor-card"
            style={{ display: "flex", flexDirection: "column", gap: 10 }}
          >
            <div style={{ fontSize: 13, fontWeight: 600 }}>
              {newPrompt.id ? "Редактировать промпт выгрузки" : "Новый промпт сводки выгрузки"}
            </div>
            <p className="muted" style={{ margin: 0, fontSize: 12 }}>
              Эти промпты доступны в экспортах и промо-админе; кнопка завершения чата использует отдельный итоговый промпт.
            </p>
            <div>
              <div className="ms-field-label">Название</div>
              <input
                placeholder="Название промпта"
                value={newPrompt.name}
                onChange={(e) => setNewPrompt((v) => ({ ...v, name: e.target.value }))}
              />
            </div>
            <div>
              <div className="ms-field-label">Текст промпта</div>
              <textarea
                rows={6}
                value={newPrompt.prompt}
                onChange={(e) => setNewPrompt((v) => ({ ...v, prompt: e.target.value }))}
              />
            </div>
            <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={newPrompt.isDefault}
                onChange={(e) => setNewPrompt((v) => ({ ...v, isDefault: e.target.checked }))}
              />
              Сделать промптом по умолчанию
            </label>
            <button type="submit" className="ms-button ms-button-ghost ms-button-xs">
              {newPrompt.id ? "Сохранить изменения" : "Сохранить промпт"}
            </button>
          </form>

          {/* Prompt list */}
          <div className="ms-admin-list">
            {summaryPrompts.map((p) => (
              <div key={p.id} className="ms-info-box">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 8 }}>
                  <strong style={{ fontSize: 13, fontWeight: 600 }}>
                    {p.name}
                    {p.isDefault ? (
                      <span style={{ marginLeft: 8, fontSize: 11, color: "var(--accent-strong)", fontWeight: 500 }}>
                        · по умолчанию
                      </span>
                    ) : null}
                  </strong>
                </div>
                <span style={{ fontSize: 12, color: "var(--muted)" }}>
                  {p.prompt.slice(0, 200)}{p.prompt.length > 200 ? "…" : ""}
                </span>
                <div className="ms-admin-mini-actions">
                  <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => editSummaryPrompt(p)}>
                    Редактировать
                  </button>
                  {!p.isDefault && (
                    <button type="button" className="ms-button ms-button-ghost ms-button-xs" onClick={() => deleteSummaryPrompt(p.id)}>
                      Удалить
                    </button>
                  )}
                </div>
              </div>
            ))}
            {summaryPrompts.length === 0 && (
              <p className="muted" style={{ fontSize: 13 }}>Промптов ещё нет</p>
            )}
          </div>
        </div>
      )}

      {/* ══ SUMMARY PROMPT ══ */}
      {orchestrationPanel === "dialogSummary" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <p className="muted" style={{ margin: 0, fontSize: 13 }}>
            Промпт для кнопки «Завершить и получить итог». Нажатие тратит одно
            сообщение из дневного лимита. Модель и температура берутся из модели AI для сводок.
          </p>
          <div>
            <div className="ms-field-label">Промпт</div>
            <textarea
              rows={7}
              value={dialogSummaryPrompt}
              onChange={(e) => setDialogSummaryPrompt(e.target.value)}
            />
          </div>
          <div>
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={saveDialogSummaryPrompt}
            >
              Сохранить итоговый промпт
            </button>
          </div>
        </div>
      )}

      {/* ══ LEAD DIGEST ══ */}
      {orchestrationPanel === "leadSummary" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <p className="muted" style={{ margin: 0, fontSize: 13 }}>
            Промпт AI-выжимки в lead-уведомлениях менеджеру (см. настройки режима —
            получатели Max/Telegram и порог сообщений). Не хардкод в коде — правится здесь.
          </p>

          <div style={{ display: "grid", gridTemplateColumns: "140px 1fr 120px", gap: 10 }} className="ms-orch-model-grid">
            <div>
              <div className="ms-field-label">Провайдер</div>
              <select
                value={aiSettings.leadSummaryProvider}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, leadSummaryProvider: e.target.value }))
                }
              >
                <option value="vsegpt">vsegpt</option>
                <option value="gemini">Gemini</option>
                <option value="anthropic">Claude</option>
              </select>
            </div>
            <div>
              <div className="ms-field-label">Модель выжимки лида</div>
              <ModelPicker
                ariaLabel="Модель выжимки лида"
                value={aiSettings.leadSummaryModel}
                onChange={(next) =>
                  setAISettings((v) => ({ ...v, leadSummaryModel: next }))
                }
                placeholder="openai/gpt-4o-mini"
                options={fetchedAIModels[leadSummaryProvider] ?? []}
              />
            </div>
            <div>
              <div className="ms-field-label">Температура</div>
              <input
                type="number"
                min="0"
                max="2"
                step="0.1"
                value={aiSettings.leadSummaryTemperature}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, leadSummaryTemperature: e.target.value }))
                }
              />
            </div>
          </div>

          <div>
            <div className="ms-field-label">Промпт</div>
            <textarea
              rows={7}
              value={leadSummaryPrompt}
              onChange={(e) => setLeadSummaryPrompt(e.target.value)}
            />
          </div>

          <div className="ms-admin-mini-actions">
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={saveLeadSummaryPrompt}
            >
              Сохранить промпт выжимки лида
            </button>
            <button
              type="button"
              className="ms-button ms-button-ghost ms-button-xs"
              onClick={() => { void saveAISettings(); }}
            >
              Сохранить модель
            </button>
          </div>
        </div>
      )}

      {/* ══ AI ══ */}
      {orchestrationPanel === "ai" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <p className="muted" style={{ margin: 0, fontSize: 13 }}>
            Live AI использует OpenAI-совместимый провайдер
            (по умолчанию <code style={{ fontFamily: "monospace", fontSize: 12 }}>api.vsegpt.ru</code>).
            Указывайте явные модели формата
            <code style={{ fontFamily: "monospace", fontSize: 12 }}>openai/gpt-4o-mini</code>.
          </p>

          <div>
            <div className="ms-field-label">
              История сообщений для live-запроса: {aiSettings.chatHistoryLimit}
            </div>
            <input
              type="range"
              min="0"
              max="100"
              step="1"
              value={aiSettings.chatHistoryLimit}
              onChange={(e) =>
                setAISettings((v) => ({ ...v, chatHistoryLimit: e.target.value }))
              }
            />
          </div>
          <div>
            <div className="ms-field-label">
              Макс. символов в одном сообщении: {aiSettings.chatMessageMaxChars}
            </div>
            <input
              type="number"
              min="1"
              max="1048576"
              step="1"
              value={aiSettings.chatMessageMaxChars}
              aria-label="Лимит символов сообщения"
              onChange={(e) =>
                setAISettings((v) => ({ ...v, chatMessageMaxChars: e.target.value }))
              }
            />
          </div>

          <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "20px 0" }} />
          <AdminAIGatewaysSection />

          <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "20px 0" }} />
          <div className="ms-field-label" style={{ fontWeight: 600, marginBottom: 8 }}>
            Защита от сбоев AI-провайдера
          </div>
          <p className="muted" style={{ fontSize: 12, margin: "0 0 12px" }}>
            При 404, 429, 5xx или сетевой ошибке система сначала делает retry, затем пробует резервную модель
            того же провайдера — для vsegpt она задаётся полем «дефолтная модель для фолбека» в списке
            AI-провайдеров выше, для Gemini/Claude — своими полями там же.
          </p>
          <div className="ms-admin-form-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
            <div>
              <div className="ms-field-label">Текста вложений за раз, символов</div>
              <input
                type="number"
                min="1000"
                max="200000"
                step="1000"
                value={aiSettings.attachmentContextMaxChars}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, attachmentContextMaxChars: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Попыток retry (429/5xx)</div>
              <input
                type="number"
                min="1"
                max="10"
                value={aiSettings.retryAttempts}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, retryAttempts: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Начальная пауза, мс</div>
              <input
                type="number"
                min="100"
                max="10000"
                step="100"
                value={aiSettings.retryInitialDelayMs}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, retryInitialDelayMs: e.target.value }))
                }
              />
            </div>
          </div>

          <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "20px 0" }} />
          <div className="ms-field-label" style={{ fontWeight: 600, marginBottom: 8 }}>
            Управляемая очередь AI-запросов
          </div>
          <p className="muted" style={{ fontSize: 12, margin: "0 0 12px" }}>
            Пользователи могут нажимать отправку свободно, но реальные запросы к провайдеру
            уходят через очередь с заданным интервалом. Для VseGPT обычно нужен интервал не меньше 1000 мс.
          </p>

          <label className="ms-inline-check">
            <input
              type="checkbox"
              checked={aiSettings.queueEnabled !== "0"}
              onChange={(e) =>
                setAISettings((v) => ({ ...v, queueEnabled: e.target.checked ? "1" : "0" }))
              }
            />{" "}
            Очередь включена
          </label>

          <div className="ms-admin-form-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
            <div>
              <div className="ms-field-label">Интервал между запросами, мс</div>
              <input
                type="number"
                min="100"
                max="60000"
                step="100"
                value={aiSettings.queueIntervalMs}
                onChange={(e) => {
                  const val = e.target.value;
                  setAISettings((v) => ({ ...v, queueIntervalMs: val }));
                }}
              />
            </div>
            <div>
              <div className="ms-field-label">Таймаут очереди, мс</div>
              <input
                type="number"
                min="1000"
                max="600000"
                step="1000"
                value={aiSettings.queueTimeoutMs}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, queueTimeoutMs: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Параллельных запросов</div>
              <input
                type="number"
                min="1"
                max="10"
                value={aiSettings.queueConcurrency}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, queueConcurrency: e.target.value }))
                }
              />
            </div>
          </div>

          <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "20px 0" }} />
          <div className="ms-field-label" style={{ fontWeight: 600, marginBottom: 8 }}>
            Вложения в чат
          </div>
          <p className="muted" style={{ fontSize: 12, margin: "0 0 12px" }}>
            Небольшие txt, md, doc и docx идут в ответ напрямую. Большие файлы один раз сжимаются моделью,
            сохраняются в базе и дальше переиспользуются по хэшу.
          </p>

          <div className="ms-admin-form-grid" style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
            <div>
              <div className="ms-field-label">Без аннотации, байт</div>
              <input
                type="number"
                min="1024"
                max="1048576"
                step="1024"
                value={aiSettings.attachmentDirectMaxBytes}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, attachmentDirectMaxBytes: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Максимум файла, байт</div>
              <input
                type="number"
                min="1024"
                max="10485760"
                step="1024"
                value={aiSettings.attachmentMaxUploadBytes}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, attachmentMaxUploadBytes: e.target.value }))
                }
              />
            </div>
            <div>
              <div className="ms-field-label">Провайдер аннотации</div>
              <select
                value={aiSettings.attachmentAnnotationProvider}
                onChange={(e) =>
                  setAISettings((v) => ({ ...v, attachmentAnnotationProvider: e.target.value }))
                }
              >
                <option value="vsegpt">vsegpt</option>
                <option value="gemini">Gemini</option>
                <option value="anthropic">Claude</option>
              </select>
            </div>
            <div>
              <div className="ms-field-label">Модель аннотации</div>
              <ModelPicker
                ariaLabel="Модель аннотации"
                value={aiSettings.attachmentAnnotationModel}
                onChange={(next) =>
                  setAISettings((v) => ({ ...v, attachmentAnnotationModel: next }))
                }
                placeholder="openai/gpt-4o-mini"
                options={fetchedAIModels[attachmentAnnotationProvider] ?? []}
              />
            </div>
          </div>

          <div>
            <div className="ms-field-label">Промпт аннотации файлов</div>
            <textarea
              rows={5}
              value={aiSettings.attachmentAnnotationPrompt}
              onChange={(e) => {
                const value = e.target.value;
                setAISettings((v) => ({ ...v, attachmentAnnotationPrompt: value }));
              }}
            />
          </div>

          <div>
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              onClick={() => { void saveAISettings(); }}
            >
              Сохранить настройки AI
            </button>
          </div>
        </div>
      )}

      <style>{`
        .ms-field-label {
          font-size: 11px;
          font-weight: 600;
          color: var(--muted);
          text-transform: uppercase;
          letter-spacing: 0.4px;
          margin-bottom: 5px;
        }
        @media (max-width: 600px) {
          .ms-orch-model-grid {
            grid-template-columns: 1fr !important;
          }
        }
      `}</style>
    </div>
  );
}
