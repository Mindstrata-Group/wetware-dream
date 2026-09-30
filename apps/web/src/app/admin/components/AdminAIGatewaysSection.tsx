"use client";

import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";

// AI gateway registry: a list in fallback order, editing, archive and a form for
// a new gateway.
//
// There used to be exactly three providers hard-coded in the code, so
// three cards were enough in the admin panel. Now a gateway is added by hand, and the screen
// must show what really determines behaviour: the order, the state of the
// key, and that a gateway can be removed without losing history.

type Gateway = {
  id: string;
  title: string;
  protocol: string;
  baseUrl: string;
  relayUrl: string;
  useRelay: boolean;
  effectiveUrl: string;
  apiKeyMasked: string;
  apiKeySource: "gateway" | "legacy" | "none";
  defaultModel: string;
  priority: number;
  enabled: boolean;
  archived: boolean;
  configured: boolean;
};

const PROTOCOL_LABELS: Record<string, string> = {
  openai: "OpenAI-совместимый",
  anthropic: "Anthropic /v1/messages",
  gemini: "Gemini (OpenAI-слой)",
};

const KEY_SOURCE_LABELS: Record<Gateway["apiKeySource"], string> = {
  gateway: "ключ задан здесь",
  legacy: "ключ унаследован из env сервера",
  none: "ключа нет — шлюз выпадает из цепочки",
};

export function AdminAIGatewaysSection() {
  const [gateways, setGateways] = useState<Gateway[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // Edit drafts per gateway are kept separate from the loaded list so that
  // a reload does not wipe unfinished input and the key mask does not turn into
  // the field value.
  const [drafts, setDrafts] = useState<
    Record<string, { defaultModel?: string; baseUrl?: string; relayUrl?: string; apiKey?: string }>
  >({});
  const [newGateway, setNewGateway] = useState({
    id: "",
    title: "",
    protocol: "openai",
    baseUrl: "",
    relayUrl: "",
    apiKey: "",
    defaultModel: "",
  });

  const load = useCallback(async () => {
    try {
      const json = await apiFetch<{ ok: boolean; gateways: Gateway[] }>("/api/admin/ai-gateways");
      setGateways(json.gateways || []);
      setError("");
    } catch (e: any) {
      setError(e?.payload?.error || e?.message || "не удалось загрузить список шлюзов");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const act = useCallback(
    async (id: string, body: Record<string, unknown>) => {
      setBusy(true);
      // Encode the id in a separate variable, not inside the template: the contract
      // test parses path literals in the source and trips over a function call inside
      // `${}`, dropping the route from the check.
      const encodedID = encodeURIComponent(id);
      try {
        await apiFetch(`/api/admin/ai-gateways/${encodedID}`, {
          method: "POST",
          body: JSON.stringify(body),
        });
        setDrafts((v) => ({ ...v, [id]: {} }));
        await load();
        setError("");
      } catch (e: any) {
        setError(e?.payload?.error || e?.message || "действие не выполнено");
      } finally {
        setBusy(false);
      }
    },
    [load]
  );

  const create = useCallback(async () => {
    setBusy(true);
    try {
      await apiFetch("/api/admin/ai-gateways", { method: "POST", body: JSON.stringify(newGateway) });
      setNewGateway({ id: "", title: "", protocol: "openai", baseUrl: "", relayUrl: "", apiKey: "", defaultModel: "" });
      await load();
      setError("");
    } catch (e: any) {
      setError(e?.payload?.error || e?.message || "шлюз не создан");
    } finally {
      setBusy(false);
    }
  }, [newGateway, load]);

  const active = gateways.filter((g) => !g.archived);
  const archived = gateways.filter((g) => g.archived);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div className="ms-field-label" style={{ fontWeight: 600 }}>
        AI-шлюзы
      </div>
      <p className="muted" style={{ fontSize: 12, margin: 0 }}>
        Порядок в списке — это порядок фолбека: если верхний шлюз не ответил, запрос уходит
        следующему. Шлюз без ключа или выключенный в цепочку не попадает. Новый шлюз встаёт
        в конец — поднимайте его сами, когда проверите.
      </p>

      {error && (
        <div className="muted" style={{ fontSize: 12, color: "var(--danger, #c33)" }}>
          {error}
        </div>
      )}

      <div style={{ display: "grid", gap: 10 }}>
        {active.map((g, index) => {
          const draft = drafts[g.id] || {};
          return (
            <div
              key={g.id}
              style={{
                display: "flex",
                flexDirection: "column",
                gap: 6,
                padding: "8px 12px",
                border: "1px solid var(--line)",
                borderRadius: 10,
                opacity: g.enabled ? 1 : 0.6,
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
                <span className="muted" style={{ fontSize: 12, width: 24, textAlign: "right" }}>
                  {index + 1}.
                </span>
                <label className="ms-inline-check" style={{ minWidth: 220 }}>
                  <input
                    type="checkbox"
                    checked={g.enabled}
                    disabled={busy}
                    onChange={(e) => act(g.id, { action: "update", enabled: e.target.checked })}
                  />{" "}
                  {g.title}
                </label>
                <span className="muted" style={{ fontSize: 12 }}>
                  {PROTOCOL_LABELS[g.protocol] || g.protocol}
                </span>
                <div className="ms-admin-mini-actions" style={{ marginLeft: "auto", display: "flex", gap: 4 }}>
                  <button disabled={busy || index === 0} onClick={() => act(g.id, { action: "move", direction: "up" })}>
                    ↑
                  </button>
                  <button
                    disabled={busy || index === active.length - 1}
                    onClick={() => act(g.id, { action: "move", direction: "down" })}
                  >
                    ↓
                  </button>
                  <button disabled={busy} onClick={() => act(g.id, { action: "archive" })}>
                    В архив
                  </button>
                </div>
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <input
                  value={draft.defaultModel ?? g.defaultModel}
                  disabled={busy}
                  onChange={(e) =>
                    setDrafts((v) => ({ ...v, [g.id]: { ...v[g.id], defaultModel: e.target.value } }))
                  }
                  placeholder="модель при фолбеке"
                  style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 180 }}
                />
                <input
                  value={draft.baseUrl ?? g.baseUrl}
                  disabled={busy}
                  onChange={(e) => setDrafts((v) => ({ ...v, [g.id]: { ...v[g.id], baseUrl: e.target.value } }))}
                  placeholder="прямой адрес (пусто — стандартный)"
                  style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 180 }}
                />
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <label className="ms-inline-check" style={{ minWidth: 220 }}>
                  <input
                    type="checkbox"
                    checked={g.useRelay}
                    disabled={busy}
                    onChange={(e) => act(g.id, { action: "update", useRelay: e.target.checked })}
                  />{" "}
                  Через ВПС
                </label>
                <input
                  value={draft.relayUrl ?? g.relayUrl}
                  disabled={busy}
                  onChange={(e) => setDrafts((v) => ({ ...v, [g.id]: { ...v[g.id], relayUrl: e.target.value } }))}
                  placeholder="адрес через ВПС, вместе с секретным путём"
                  style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 220 }}
                />
              </div>

              <div className="muted" style={{ fontSize: 12 }}>
                Запросы уходят на: <code>{g.effectiveUrl || "стандартный адрес протокола"}</code>
                {g.useRelay && !g.relayUrl && " — галочка включена, но адрес ВПС не задан, поэтому идём напрямую"}
              </div>

              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <span className="muted" style={{ fontSize: 12, minWidth: 220 }}>
                  {g.apiKeyMasked || "—"} · {KEY_SOURCE_LABELS[g.apiKeySource]}
                </span>
                <input
                  type="password"
                  value={draft.apiKey ?? ""}
                  disabled={busy}
                  autoComplete="off"
                  onChange={(e) => setDrafts((v) => ({ ...v, [g.id]: { ...v[g.id], apiKey: e.target.value } }))}
                  placeholder="вставить новый ключ, чтобы заменить"
                  style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 180 }}
                />
                <button
                  disabled={busy}
                  onClick={() =>
                    act(g.id, {
                      action: "update",
                      defaultModel: draft.defaultModel ?? g.defaultModel,
                      baseUrl: draft.baseUrl ?? g.baseUrl,
                      relayUrl: draft.relayUrl ?? g.relayUrl,
                      apiKey: draft.apiKey ?? "",
                    })
                  }
                >
                  Сохранить
                </button>
              </div>
            </div>
          );
        })}
      </div>

      <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "12px 0" }} />
      <div className="ms-field-label" style={{ fontWeight: 600 }}>
        Добавить шлюз
      </div>
      <p className="muted" style={{ fontSize: 12, margin: 0 }}>
        У шлюза задаётся <b>куда</b> слать (адрес — прямой и через ВПС) и <b>в какой форме</b>
        (протокол). Протокол — это формат запроса и ответа, а не название сервиса:
        «OpenAI-совместимый» понимают vsegpt и большинство релеев, у Anthropic свой формат
        (<code>/v1/messages</code>), у Gemini — свой OpenAI-подобный слой. Сервис с
        собственным форматом ответа так не заводится: под него нужен код.
      </p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          value={newGateway.id}
          onChange={(e) => setNewGateway((v) => ({ ...v, id: e.target.value }))}
          placeholder="id (латиница, напр. relay-fi)"
          style={{ fontFamily: "monospace", fontSize: 12, minWidth: 180 }}
        />
        <input
          value={newGateway.title}
          onChange={(e) => setNewGateway((v) => ({ ...v, title: e.target.value }))}
          placeholder="название для админки"
          style={{ fontSize: 12, minWidth: 180 }}
        />
        <select
          value={newGateway.protocol}
          onChange={(e) => setNewGateway((v) => ({ ...v, protocol: e.target.value }))}
          style={{ fontSize: 12 }}
        >
          {Object.entries(PROTOCOL_LABELS).map(([id, label]) => (
            <option key={id} value={id}>
              {label}
            </option>
          ))}
        </select>
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input
          value={newGateway.baseUrl}
          onChange={(e) => setNewGateway((v) => ({ ...v, baseUrl: e.target.value }))}
          placeholder="прямой адрес (пусто — стандартный для протокола)"
          style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 200 }}
        />
        <input
          value={newGateway.relayUrl}
          onChange={(e) => setNewGateway((v) => ({ ...v, relayUrl: e.target.value }))}
          placeholder="адрес через ВПС (можно задать сразу, включить потом)"
          style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 200 }}
        />
        <input
          value={newGateway.defaultModel}
          onChange={(e) => setNewGateway((v) => ({ ...v, defaultModel: e.target.value }))}
          placeholder="модель при фолбеке"
          style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 160 }}
        />
        <input
          type="password"
          value={newGateway.apiKey}
          autoComplete="off"
          onChange={(e) => setNewGateway((v) => ({ ...v, apiKey: e.target.value }))}
          placeholder="ключ"
          style={{ fontFamily: "monospace", fontSize: 12, flex: 1, minWidth: 160 }}
        />
        <button disabled={busy || !newGateway.id.trim()} onClick={create}>
          Добавить
        </button>
      </div>

      {archived.length > 0 && (
        <>
          <hr style={{ border: 0, borderTop: "1px solid var(--line)", margin: "12px 0" }} />
          <div className="ms-field-label" style={{ fontWeight: 600 }}>
            Архив
          </div>
          <p className="muted" style={{ fontSize: 12, margin: 0 }}>
            Архивный шлюз не участвует в цепочке, но остаётся в истории вызовов и в режимах,
            которые на него ссылались. Поэтому шлюзы архивируются, а не удаляются.
          </p>
          <div style={{ display: "grid", gap: 6 }}>
            {archived.map((g) => (
              <div
                key={g.id}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 12,
                  padding: "6px 12px",
                  border: "1px dashed var(--line)",
                  borderRadius: 10,
                }}
              >
                <span style={{ fontSize: 13 }}>{g.title}</span>
                <span className="muted" style={{ fontSize: 12 }}>
                  {PROTOCOL_LABELS[g.protocol] || g.protocol}
                </span>
                <button disabled={busy} style={{ marginLeft: "auto" }} onClick={() => act(g.id, { action: "restore" })}>
                  Вернуть
                </button>
              </div>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
