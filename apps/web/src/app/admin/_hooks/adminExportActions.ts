import type { CreatedPromo, SummaryPrompt } from "../types";
import type { AdminPageContextValue, MetricRow } from "../components/AdminPageContext";
import { apiFetch } from "@/lib/api";
import { defaultSummaryPrompt } from "../constants";
import { formatDate } from "../utils";

type AdminExportActionsContext = Pick<AdminPageContextValue, "exportFilters" | "exportModes" | "setExportResult" | "setExportSummaryPayload" | "setExportMeta" | "setExportSummaryResult" | "setExportSummaryHistory" | "newPrompt" | "setNewPrompt" | "loadSummaryPrompts" | "broadcast" | "lastCreatedPromos" | "exportSummaryPayload" | "exportResult" | "setError" | "setErrorDebug" | "showNotice" | "handleError" | "router">;

export function createAdminExportActions(ctx: AdminExportActionsContext) {
  const { exportFilters, exportModes, setExportResult, setExportSummaryPayload, setExportMeta, setExportSummaryResult, setExportSummaryHistory, newPrompt, setNewPrompt, loadSummaryPrompts, broadcast, lastCreatedPromos, exportSummaryPayload, exportResult, setError, setErrorDebug, showNotice, handleError, router } = ctx;
  async function exportMessages() {
    try {
      const qs = new URLSearchParams();
      exportFilters.modeIds.forEach((id) => qs.append("modeIds", String(id)));
      exportFilters.userIds.forEach((id) => qs.append("userIds", String(id)));
      exportFilters.promocodeIds.forEach((id) => qs.append("promocodeIds", String(id)));
      qs.set("roleFilter", exportFilters.roleFilter);
      qs.set("limit", Number(exportFilters.limit) >= 1000001 ? "0" : String(Number(exportFilters.limit) || 10000));
      qs.set("dateFrom", exportFilters.dateFrom); qs.set("dateTo", exportFilters.dateTo);
      if (exportFilters.withSummary) { qs.set("withSummary", "true"); qs.set("summaryPrompt", exportFilters.customPrompt); }
      const j = await apiFetch<any>(`/api/admin/exports/messages?${qs}`);
      setExportResult(j.messages || []); setExportSummaryPayload(j.summaryPayload || "");
      setExportMeta({ messageCount: j.messageCount || 0, sourceBytes: j.sourceBytes || 0, approxTokens: j.approxTokens || 0 });
      setExportSummaryResult("");
    } catch (e) { handleError(e, "Ошибка экспорта"); }
  }
  async function summarizeExport() {
    try {
      setError(""); setErrorDebug(null); setExportSummaryResult("Резюмирование запущено, ждём ответ нейросети...");
      const j = await apiFetch<any>("/api/admin/exports/messages", { method: "POST", body: JSON.stringify({ modeIds: exportFilters.modeIds, userIds: exportFilters.userIds, promocodeIds: exportFilters.promocodeIds, promptId: Number(exportFilters.promptId) || 0, prompt: exportFilters.customPrompt, roleFilter: exportFilters.roleFilter, limit: Number(exportFilters.limit) >= 1000001 ? 0 : Number(exportFilters.limit) || 10000, dateFrom: exportFilters.dateFrom, dateTo: exportFilters.dateTo }) });
      setExportMeta({ messageCount: j.messageCount || 0, sourceBytes: j.sourceBytes || 0, approxTokens: j.approxTokens || 0 });
      setExportSummaryResult(j.result || ""); setExportResult([]);
      setExportSummaryHistory((prev) => [{ ...j, result: j.result || "" }, ...prev].slice(0, 10));
    } catch (e) { setExportSummaryResult(""); handleError(e, "Ошибка резюмирования экспорта"); }
  }
  async function createSummaryPrompt(e: React.FormEvent) {
    e.preventDefault();
    try {
      const method = newPrompt.id ? "PATCH" : "POST";
      const path = newPrompt.id ? `/api/admin/summary-prompts/${newPrompt.id}` : "/api/admin/summary-prompts";
      await apiFetch(path, { method, body: JSON.stringify({ name: newPrompt.name, prompt: newPrompt.prompt, isDefault: newPrompt.isDefault }) });
      await loadSummaryPrompts();
      setNewPrompt({ id: 0, name: "", prompt: defaultSummaryPrompt, isDefault: false });
    } catch (err) { handleError(err, "Ошибка промпта"); }
  }
  async function deleteSummaryPrompt(id: number) {
    try { await apiFetch(`/api/admin/summary-prompts/${id}`, { method: "DELETE" }); await loadSummaryPrompts(); } catch (err) { handleError(err, "Ошибка удаления промпта"); }
  }
  function editSummaryPrompt(prompt: SummaryPrompt) {
    setNewPrompt({ id: prompt.id, name: prompt.name, prompt: prompt.prompt, isDefault: prompt.isDefault });
  }
  async function sendBroadcast(e: React.FormEvent) {
    e.preventDefault();
    try { const j = await apiFetch<any>("/api/admin/broadcasts", { method: "POST", body: JSON.stringify(broadcast) }); showNotice(`Рассылка сохранена. Получателей: ${j.plannedRecipients}`); } catch (err) { handleError(err, "Ошибка рассылки"); }
  }
  function createdPromosTable(items: CreatedPromo[] = lastCreatedPromos) {
    const delimiter = ";";
    const cell = (value: string | number | undefined) => `"${String(value ?? "").replace(/"/g, '""')}"`;
    const header = ["code", "apply_url", "temporary_admin_url", "target_ids"].join(delimiter);
    const rows = items.map((item) => [item.code, item.applyUrl || "", item.temporaryAdminUrl || "", item.targetIds?.length ? item.targetIds.join(",") : item.targetId || ""].map(cell).join(delimiter));
    return [`sep=${delimiter}`, header, ...rows].join("\r\n");
  }
  function downloadText(filename: string, text: string) {
    const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url; a.download = filename; a.click(); URL.revokeObjectURL(url);
  }
  async function copyCreatedPromos() {
    await navigator.clipboard?.writeText(createdPromosTable()).catch(() => null);
    showNotice(`Пакет промокодов скопирован: ${lastCreatedPromos.length}.`);
  }
  function downloadCreatedPromos() {
    downloadText(`promocodes-${new Date().toISOString().slice(0, 10)}.csv`, `\uFEFF${createdPromosTable()}`);
    showNotice(`Пакет промокодов скачан: ${lastCreatedPromos.length}.`);
  }
  function downloadTxt() {
    const text = exportSummaryPayload || exportResult.map((m: MetricRow) => `[${formatDate(String(m.createdAt))}] user#${m.userId} ${m.modeName} ${m.role}:\n${m.content}`).join("\n\n");
    const today = new Date().toISOString().slice(0, 10);
    const parts: string[] = [];
    if (exportFilters.modeIds.length > 0) {
      const modeMap = new Map(exportModes.map((m) => [m.id, m.name]));
      const names = exportFilters.modeIds.map((id) => modeMap.get(id) ?? String(id)).join("+");
      parts.push(names.slice(0, 40));
    }
    if (exportFilters.dateFrom) parts.push(`от-${exportFilters.dateFrom}`);
    if (exportFilters.dateTo) parts.push(`по-${exportFilters.dateTo}`);
    if (exportFilters.roleFilter && exportFilters.roleFilter !== "all") parts.push(exportFilters.roleFilter);
    const limit = Number(exportFilters.limit);
    if (limit < 1000001) parts.push(`лимит-${limit}`);
    const slug = parts.length ? parts.join("_") : "все";
    const filename = `mindstrata-${slug}_${today}.txt`.replace(/[^\wЀ-ӿ.+\-_]/g, "-");
    downloadText(filename, text);
  }
  async function copy(text: string) {
    await navigator.clipboard?.writeText(text).catch(() => null);
    showNotice("Ссылка скопирована.");
  }
  async function logout() {
    await apiFetch("/api/auth/logout", { method: "POST" }).catch(() => null);
    router.push("/login");
  }


  return { exportMessages, summarizeExport, createSummaryPrompt, deleteSummaryPrompt, editSummaryPrompt, sendBroadcast, createdPromosTable, downloadText, copyCreatedPromos, downloadCreatedPromos, downloadTxt, copy, logout };
}
