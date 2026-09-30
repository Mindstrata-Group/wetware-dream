"use client";

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { useAdminPageContext } from "../AdminPageContext";
import { ModelPicker } from "../ModelPicker";
import { DemoChatJSONField } from "./DemoChatJSONField";
import { ModeHistoryPanel } from "./ModeHistoryPanel";

export function EditPanel() {
  const {
    modes, modeMeta, modeDetail, setModeDetail, modeQ, setModeQ, newMode, setNewMode,
    guardrail, setGuardrail, modelStats, modePage, modeTotalPages, loadModes, loadModelStats,
    openMode, saveMode, copyMode, createMode, testModeModel, detachModeFromPaidTariffs,
    cancelDeleteModeHold, startDeleteModeHold, applyGuardrail, testGuardrailModel, pageSizes, copy,
    showNotice, handleError,
  } = useAdminPageContext();

  // Editor tab: "Edit" (form) or "History" (prompt
  // versions). Resets to "Edit" when the mode is switched.
  const [modeEditorTab, setModeEditorTab] = useState<"edit" | "history">("edit");
  useEffect(() => {
    setModeEditorTab("edit");
  }, [modeDetail?.id]);

  // Live list of the provider's models: a <select> dropdown via ModelPicker;
  // manual input stays always available (the "enter manually" item). It used to
  // be <input list>+<datalist>: the native autocomplete is drawn by the OS and looks
  // off outside desktop Chrome (on a phone it is something else entirely, and it is not always
  // clear that it is a dropdown at all).
  // Until the response arrives (or for the vsegpt provider/a failure) the list is empty and the field
  // stays a plain text input.
  const [fetchedAIModels, setFetchedAIModels] = useState<Record<string, string[]>>({});
  const activeProvider = modeDetail?.aiProvider ?? "vsegpt";
  useEffect(() => {
    if (activeProvider === "vsegpt" || fetchedAIModels[activeProvider]) return;
    let cancelled = false;
    apiFetch<{ ok: boolean; models: { id: string; displayName: string }[] }>(
      `/api/admin/ai-models?provider=${activeProvider}`
    )
      .then((json) => {
        if (cancelled || !json.models?.length) return;
        setFetchedAIModels((v) => ({ ...v, [activeProvider]: json.models.map((m) => m.id) }));
      })
      .catch(() => {
        /* an auto-load failure is not critical: the static fallback list below remains */
      });
    return () => {
      cancelled = true;
    };
  }, [activeProvider, fetchedAIModels]);

  async function handleModeRestored() {
    if (!modeDetail) return;
    try {
      const json = await apiFetch<{ mode: typeof modeDetail }>(`/api/admin/modes/${modeDetail.id}`);
      setModeDetail(json.mode);
      showNotice("Версия восстановлена.");
      setModeEditorTab("edit");
      await loadModes();
    } catch (e) {
      handleError(e, "Ошибка восстановления версии");
    }
  }

  return (
<div
  style={{
    display: "grid",
    gridTemplateColumns: "300px minmax(0,1fr)",
    gap: 16,
    alignItems: "start",
  }}
  className="ms-modes-edit-grid"
>
  {/* Mode list */}
  <div className="card ms-admin-card">
    <div className="ms-admin-card-head">
      <h2>Режимы</h2>
      <button
        type="button"
        className="ms-button ms-button-ghost ms-button-xs"
        onClick={() => loadModes(0)}
      >
        Найти
      </button>
    </div>

    <div className="ms-admin-toolbar">
      <input
        value={modeQ}
        onChange={(e) => setModeQ(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            void loadModes(0);
          }
        }}
        placeholder="название или id"
      />
      <select
        value={modeMeta.limit}
        onChange={(e) => loadModes(0, Number(e.target.value))}
      >
        {pageSizes.map((s) => (
          <option key={s}>{s}</option>
        ))}
      </select>
    </div>

    <div className="ms-admin-list">
      {modes.map((m) => (
        <button
          key={m.id}
          className={modeDetail?.id === m.id ? "is-active" : ""}
          onClick={() => openMode(m)}
        >
          <strong>
            #{m.id} {m.name}
          </strong>
          <span>
            {m.hidden
              ? "скрыт из выбора пользователя"
              : "доступен"}
            {m.paidChains ? ` · ${m.paidChains}` : ""}
          </span>
        </button>
      ))}
      {modes.length === 0 && (
        <p className="muted" style={{ padding: "8px 0" }}>
          Режимов не найдено
        </p>
      )}
    </div>

    <div className="ms-pagination">
      <button
        className="ms-button ms-button-ghost ms-button-xs"
        disabled={modeMeta.offset === 0}
        onClick={() =>
          loadModes(Math.max(0, modeMeta.offset - modeMeta.limit))
        }
      >
        ← Назад
      </button>
      <span style={{ fontSize: 13, color: "var(--muted)" }}>
        {modePage} / {modeTotalPages}
      </span>
      <button
        className="ms-button ms-button-ghost ms-button-xs"
        disabled={modePage >= modeTotalPages}
        onClick={() => loadModes(modeMeta.offset + modeMeta.limit)}
      >
        Вперёд →
      </button>
    </div>
  </div>

  {/* Mode editor */}
  <div className="card ms-admin-card">
    <h2>Редактор режима</h2>
    {modeDetail && (
      <div className="ms-admin-toolbar" style={{ marginBottom: 12 }}>
        <button
          type="button"
          className={`ms-button ms-button-xs ${modeEditorTab === "edit" ? "ms-button-primary" : "ms-button-ghost"}`}
          onClick={() => setModeEditorTab("edit")}
        >
          Редактирование
        </button>
        <button
          type="button"
          className={`ms-button ms-button-xs ${modeEditorTab === "history" ? "ms-button-primary" : "ms-button-ghost"}`}
          onClick={() => setModeEditorTab("history")}
        >
          История
        </button>
      </div>
    )}
    {modeDetail && modeEditorTab === "history" ? (
      <ModeHistoryPanel
        modeId={modeDetail.id}
        current={{ prompt: modeDetail.prompt, welcomeMessage: modeDetail.welcomeMessage, criteria: modeDetail.criteria }}
        onRestored={handleModeRestored}
      />
    ) : modeDetail ? (
      <form className="ms-admin-form" onSubmit={saveMode}>

        {/* Name */}
        <label style={{ fontWeight: 500 }}>
          Название
          <input
            value={modeDetail.name}
            onChange={(e) =>
              setModeDetail(
                (v) => v && { ...v, name: e.target.value }
              )
            }
          />
        </label>

        {/* Provider */}
        <label style={{ fontWeight: 500 }}>
          Провайдер
          <select
            value={modeDetail.aiProvider ?? "vsegpt"}
            onChange={(e) =>
              setModeDetail(
                (v) => v && { ...v, aiProvider: e.target.value as typeof v.aiProvider }
              )
            }
          >
            <option value="vsegpt">vsegpt (агрегатор, страховка)</option>
            <option value="gemini">Gemini напрямую</option>
            <option value="anthropic">Claude напрямую</option>
          </select>
        </label>

        {/* Model + test button */}
        <label style={{ fontWeight: 500 }}>
          Модель AI
        </label>
        <div className="ms-admin-toolbar">
          <ModelPicker
            ariaLabel="Модель AI"
            value={modeDetail.aiModel}
            onChange={(next) =>
              setModeDetail(
                (v) => v && { ...v, aiModel: next }
              )
            }
            options={fetchedAIModels[activeProvider] ?? (
              activeProvider === "gemini"
                ? ["gemini-3.5-flash-minimal", "gemini-3.1-flash-lite", "gemini-3.1-pro"]
                : activeProvider === "anthropic"
                  ? ["claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"]
                  : []
            )}
          />
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={testModeModel}
          >
            Тест: Привет
          </button>
        </div>

        {/* Reasoning */}
        <label style={{ fontWeight: 500 }}>
          Размышления
          <select
            value={modeDetail.thinkingMode ?? "default"}
            onChange={(e) =>
              setModeDetail(
                (v) => v && { ...v, thinkingMode: e.target.value as typeof v.thinkingMode }
              )
            }
          >
            <option value="default">По умолчанию</option>
            <option value="off">Выключены</option>
            <option value="low">Эконом</option>
            <option value="high">Максимум</option>
          </select>
          <small>Для vsegpt игнорируется — thinking зашит в имя модели.</small>
        </label>

        {/* Temperature */}
        <label style={{ fontWeight: 500 }}>
          Температура: {modeDetail.modelTemperature ?? 0}
          <input
            type="range"
            min="0"
            max="1"
            step="0.05"
            value={modeDetail.modelTemperature ?? 0}
            onChange={(e) =>
              setModeDetail(
                (v) =>
                  v && {
                    ...v,
                    modelTemperature: Number(e.target.value),
                  }
              )
            }
          />
        </label>

        {/* Response token ceiling (max_tokens) */}
        <label style={{ fontWeight: 500 }}>
          Лимит ответа (токенов):{" "}
          {(modeDetail.aiMaxTokens ?? 0) > 0
            ? modeDetail.aiMaxTokens
            : "глобальный дефолт"}
          <input
            type="range"
            min="0"
            max="32768"
            step="512"
            value={modeDetail.aiMaxTokens ?? 0}
            onChange={(e) =>
              setModeDetail(
                (v) =>
                  v && {
                    ...v,
                    aiMaxTokens: Number(e.target.value),
                  }
              )
            }
          />
          <span style={{ display: "block", fontWeight: 400, fontSize: 12, opacity: 0.7 }}>
            0 = использовать общий лимит из настроек AI. Выше — длиннее ответ,
            меньше обрывов, но дороже.
          </span>
        </label>

        {/* Reminders */}
        <label style={{ fontWeight: 500 }}>
          Количество напоминаний:{" "}
          {modeDetail.reminderCount ?? 0}
          <input
            type="range"
            min="0"
            max="10"
            step="1"
            value={modeDetail.reminderCount ?? 0}
            onChange={(e) =>
              setModeDetail(
                (v) =>
                  v && {
                    ...v,
                    reminderCount: Number(e.target.value),
                  }
              )
            }
          />
        </label>

        {/* Checkboxes */}
        <div
          style={{ display: "flex", gap: 20, flexWrap: "wrap" }}
        >
          <label className="ms-inline-check">
            <input
              type="checkbox"
              checked={Boolean(modeDetail.audioEnabled)}
              onChange={(e) =>
                setModeDetail(
                  (v) =>
                    v && { ...v, audioEnabled: e.target.checked }
                )
              }
            />{" "}
            Поддержка аудио
          </label>
          <label className="ms-inline-check">
            <input
              type="checkbox"
              checked={modeDetail.hidden}
              onChange={(e) =>
                setModeDetail(
                  (v) =>
                    v && { ...v, hidden: e.target.checked }
                )
              }
            />{" "}
            Скрыть из выбора пользователя
          </label>
        </div>

        {/* Welcome message */}
        <label style={{ fontWeight: 500 }}>
          Приветственное сообщение
          <textarea
            rows={3}
            placeholder="Что напишет помощник в начале диалога"
            value={modeDetail.welcomeMessage || ""}
            onChange={(e) =>
              setModeDetail(
                (v) =>
                  v && { ...v, welcomeMessage: e.target.value }
              )
            }
          />
        </label>

        {/* Criteria */}
        <label style={{ fontWeight: 500 }}>
          Критерии оркестратора
          <textarea
            rows={4}
            placeholder="По каким признакам оркестратор должен выбрать этот режим"
            value={modeDetail.criteria || ""}
            onChange={(e) =>
              setModeDetail(
                (v) => v && { ...v, criteria: e.target.value }
              )
            }
          />
        </label>

        {/* Orchestrator interval */}
        <label style={{ fontWeight: 500 }}>
          Проверять оркестратор каждые N сообщений
          <input
            type="number"
            min="1"
            placeholder="5"
            value={modeDetail.orchestratorCheckInterval ?? 5}
            onChange={(e) =>
              setModeDetail(
                (v) =>
                  v && {
                    ...v,
                    orchestratorCheckInterval: Number(e.target.value),
                  }
              )
            }
          />
          <small>5 = проверка каждые 5 сообщений пользователя</small>
        </label>

        {/* Lead notifications: trigger "user replied N times" ->
            a digest to managers in Max. Recipients are chat_ids separated by commas. */}
        <label className="ms-inline-check" style={{ fontWeight: 500 }}>
          <input
            type="checkbox"
            checked={Boolean(modeDetail.leadNotifyEnabled)}
            onChange={(e) =>
              setModeDetail(
                (v) => v && { ...v, leadNotifyEnabled: e.target.checked }
              )
            }
          />{" "}
          Уведомлять менеджера о тёплом лиде
        </label>
        {modeDetail.leadNotifyEnabled && (
          <>
            <label style={{ fontWeight: 500 }}>
              Получатели (Max chat_id, через запятую)
              <input
                placeholder="111222333, 123456789"
                value={modeDetail.leadNotifyChatIds ?? ""}
                onChange={(e) =>
                  setModeDetail(
                    (v) => v && { ...v, leadNotifyChatIds: e.target.value }
                  )
                }
              />
              <small>
                chat_id появляется после привязки Max-бота получателем; пусто —
                уведомления не шлются даже при включённом триггере.
              </small>
            </label>
            <label style={{ fontWeight: 500 }}>
              Получатели Telegram (chat_id или @username, через запятую)
              <input
                placeholder="123456789, @username"
                value={modeDetail.leadNotifyTelegramIds ?? ""}
                onChange={(e) =>
                  setModeDetail(
                    (v) => v && { ...v, leadNotifyTelegramIds: e.target.value }
                  )
                }
              />
              <small>Необязательно — можно указать Max, Telegram или оба канала сразу.</small>
            </label>
            <label style={{ fontWeight: 500 }}>
              Слать после N-го ответа пользователя
              <input
                type="number"
                min="1"
                max="100"
                value={modeDetail.leadNotifyThreshold ?? 3}
                onChange={(e) =>
                  setModeDetail(
                    (v) =>
                      v && {
                        ...v,
                        leadNotifyThreshold: Number(e.target.value),
                      }
                  )
                }
              />
              <small>
                Одно уведомление на диалог: внутренний id, промокод и краткая
                выжимка разговора.
              </small>
            </label>
          </>
        )}

        {/* System prompt */}
        <div style={{ display: "flex", alignItems: "center", gap: 8, fontWeight: 500 }}>
          Системный промпт
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={() => copy(modeDetail.prompt)}
          >
            Скопировать
          </button>
        </div>
        <textarea
          rows={10}
          placeholder="Опишите роль, стиль и правила режима"
          value={modeDetail.prompt}
          onChange={(e) =>
            setModeDetail(
              (v) => v && { ...v, prompt: e.target.value }
            )
          }
        />

        {/* Demo dialog (raw JSON): shown on the home page as a preview.
            We validate with JSON.parse on change; the backend also rejects
            broken JSON via the `::jsonb` cast: double protection. */}
        <DemoChatJSONField
          value={modeDetail.demoChat ?? ""}
          onChange={(v) =>
            setModeDetail((d) => d && { ...d, demoChat: v })
          }
        />

        {/* Buttons */}
        <div className="ms-admin-mini-actions">
          <button className="ms-button ms-button-primary ms-button-xs">
            Сохранить
          </button>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={copyMode}
          >
            Скопировать режим
          </button>
          <button
            type="button"
            className="ms-button ms-button-ghost ms-button-xs"
            onClick={detachModeFromPaidTariffs}
            onPointerDown={startDeleteModeHold}
            onPointerUp={cancelDeleteModeHold}
            onPointerLeave={cancelDeleteModeHold}
            onPointerCancel={cancelDeleteModeHold}
          >
            Убрать из всех платных тарифов
          </button>
        </div>


      </form>
    ) : (
      <p className="muted">
        Выберите режим из списка слева. Скрытый режим не удаляется —
        он пропадает из выбора пользователя.
      </p>
    )}
  </div>
</div>
  );
}
