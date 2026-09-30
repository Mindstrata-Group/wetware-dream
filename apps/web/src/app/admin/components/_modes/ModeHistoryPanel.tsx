"use client";

// The "History" tab of the mode editor. No git terminology in the UI:
// only "saved" / "version of {date}" / "restore". The backend keeps
// the history in a separate git repository (details hidden behind /api/admin/modes/{id}/history*),
// but the user does not know about it and should not have to.

import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import { diffLines } from "./lineDiff";

type HistoryVersion = {
  sha: string;
  authorName: string;
  authorEmail: string;
  createdAt: string;
  message: string;
};

type ModeSnapshot = {
  name: string;
  prompt: string;
  welcomeMessage: string;
  criteria: string;
};

function formatDateTime(iso: string) {
  try {
    return new Date(iso).toLocaleString("ru-RU", { dateStyle: "medium", timeStyle: "short" });
  } catch {
    return iso;
  }
}

function errorMessage(e: unknown, fallback: string) {
  return e instanceof Error && e.message ? e.message : fallback;
}

function DiffBlock({ label, before, after }: { label: string; before: string; after: string }) {
  if ((before || "") === (after || "")) return null;
  const lines = diffLines(before || "", after || "");
  return (
    <div style={{ marginBottom: 12 }}>
      <div style={{ fontWeight: 500, fontSize: 12, marginBottom: 4 }}>{label}</div>
      <pre
        style={{
          fontFamily: '"JetBrains Mono", "SF Mono", Menlo, ui-monospace, monospace',
          fontSize: 12,
          whiteSpace: "pre-wrap",
          border: "1px solid var(--line)",
          borderRadius: 6,
          padding: "8px 10px",
          background: "var(--card)",
          maxHeight: 260,
          overflow: "auto",
          margin: 0,
        }}
      >
        {lines.map((line, idx) => (
          <div
            key={idx}
            style={{
              color: line.type === "add" ? "#1D9E75" : line.type === "remove" ? "#b00020" : "inherit",
              background:
                line.type === "add"
                  ? "rgba(29,158,117,0.08)"
                  : line.type === "remove"
                    ? "rgba(176,0,32,0.08)"
                    : "transparent",
            }}
          >
            {line.type === "add" ? "+ " : line.type === "remove" ? "− " : "  "}
            {line.text}
          </div>
        ))}
      </pre>
    </div>
  );
}

export function ModeHistoryPanel({
  modeId,
  current,
  onRestored,
}: {
  modeId: number;
  current: { prompt: string; welcomeMessage?: string; criteria?: string };
  onRestored: () => void;
}) {
  const [versions, setVersions] = useState<HistoryVersion[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState("");
  const [selectedSha, setSelectedSha] = useState<string | null>(null);
  const [selectedSnapshot, setSelectedSnapshot] = useState<ModeSnapshot | null>(null);
  const [snapshotLoading, setSnapshotLoading] = useState(false);
  const [snapshotError, setSnapshotError] = useState("");
  const [restoring, setRestoring] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setVersions(null);
    setSelectedSha(null);
    setSelectedSnapshot(null);
    setListError("");
    setLoading(true);
    apiFetch<{ versions: HistoryVersion[] }>(`/api/admin/modes/${modeId}/history`)
      .then((json) => {
        if (!cancelled) setVersions(json.versions || []);
      })
      .catch((e) => {
        if (!cancelled) setListError(errorMessage(e, "Не удалось загрузить историю версий"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [modeId]);

  async function selectVersion(sha: string) {
    setSelectedSha(sha);
    setSelectedSnapshot(null);
    setSnapshotError("");
    setSnapshotLoading(true);
    try {
      const json = await apiFetch<{ version: ModeSnapshot }>(`/api/admin/modes/${modeId}/history/${sha}`);
      setSelectedSnapshot(json.version);
    } catch (e) {
      setSnapshotError(errorMessage(e, "Не удалось загрузить версию"));
    } finally {
      setSnapshotLoading(false);
    }
  }

  async function restore(sha: string) {
    if (!window.confirm("Восстановить эту версию? Текущее содержимое сохранится в истории как предыдущая версия.")) {
      return;
    }
    setRestoring(true);
    setSnapshotError("");
    try {
      await apiFetch(`/api/admin/modes/${modeId}/restore/${sha}`, { method: "POST" });
      onRestored();
    } catch (e) {
      setSnapshotError(errorMessage(e, "Не удалось восстановить версию"));
    } finally {
      setRestoring(false);
    }
  }

  if (loading) return <p className="muted">Загрузка истории…</p>;
  if (listError) return <p className="muted">{listError}</p>;
  if (!versions || versions.length === 0) {
    return <p className="muted">Пока нет сохранённых версий этого режима — они появятся после следующего сохранения.</p>;
  }

  const sameAsCurrent =
    !!selectedSnapshot &&
    selectedSnapshot.prompt === (current.prompt || "") &&
    (selectedSnapshot.welcomeMessage || "") === (current.welcomeMessage || "") &&
    (selectedSnapshot.criteria || "") === (current.criteria || "");

  return (
    <div style={{ display: "grid", gridTemplateColumns: "220px minmax(0,1fr)", gap: 16 }} data-testid="mode-history-panel">
      <div className="ms-admin-list">
        {versions.map((v) => (
          <button
            key={v.sha}
            type="button"
            className={selectedSha === v.sha ? "is-active" : ""}
            onClick={() => selectVersion(v.sha)}
          >
            <strong>Сохранено {formatDateTime(v.createdAt)}</strong>
            <span>{v.authorName || v.authorEmail}</span>
          </button>
        ))}
      </div>
      <div>
        {!selectedSha && <p className="muted">Выберите версию слева, чтобы увидеть отличия от текущего содержимого.</p>}
        {selectedSha && snapshotLoading && <p className="muted">Загрузка версии…</p>}
        {selectedSha && selectedSnapshot && (
          <div>
            <DiffBlock label="Системный промпт" before={selectedSnapshot.prompt} after={current.prompt || ""} />
            <DiffBlock
              label="Приветственное сообщение"
              before={selectedSnapshot.welcomeMessage}
              after={current.welcomeMessage || ""}
            />
            <DiffBlock label="Критерии оркестратора" before={selectedSnapshot.criteria} after={current.criteria || ""} />
            {sameAsCurrent && <p className="muted">Эта версия совпадает с текущим содержимым.</p>}
            <button
              type="button"
              className="ms-button ms-button-primary ms-button-xs"
              disabled={restoring}
              onClick={() => restore(selectedSha)}
            >
              Восстановить эту версию
            </button>
          </div>
        )}
        {snapshotError && (
          <p className="muted" style={{ color: "#b00020" }}>
            {snapshotError}
          </p>
        )}
      </div>
    </div>
  );
}
