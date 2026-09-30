#!/usr/bin/env node
/* Mindstrata management MCP server: gives Claude agents commands to the service.
 *
 * No dependencies on purpose. MCP is JSON-RPC 2.0 over stdio, and the protocol,
 * to the extent needed, fits in this file. The server must start with one
 * command on any machine, including where npm is unavailable: installing an SDK
 * for three methods would add one more point of failure.
 *
 * All commands go to POST /api/mcp/call with the key in the X-MCP-Key header.
 * The key lives in system_settings.mcp_admin_key on the server; here it is read
 * from an environment variable and never passed as a command-line argument,
 * otherwise it would show up in ps.
 *
 * Run:
 *   MINDSTRATA_MCP_KEY=… node apps/mcp-admin/server.mjs
 * Connecting to Claude Code: see README.md next to this file.
 */

const API = (process.env.MINDSTRATA_API_URL || "https://mindstrata.ru").replace(/\/+$/, "");
const KEY = process.env.MINDSTRATA_MCP_KEY || "";
const NAME = "mindstrata-admin";
const VERSION = "1.0.0";

/* ── tool descriptions ──────────────────────────────────────────────────
   Descriptions are written for the agent, not for a human: it picks a tool
   by this text alone. So each one says what the command does and what it
   does NOT do, e.g. that closing access is reversible. */
const TOOLS = [
  {
    name: "stratum_stats",
    description: "Сводка по сервису Mindstrata: сколько пользователей, у скольких живой доступ, сколько активных подписок, сколько сдано отчётов по практике «Отдел Н». Только чтение.",
    action: "stats.summary",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
  {
    name: "stratum_find_user",
    description: "Найти пользователя по имени, telegram-нику, почте или id. Возвращает id, роль, дату регистрации, последний вход и число живых доступов. Только чтение.",
    action: "users.find",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "часть имени, ника или почты" },
        userId: { type: "number", description: "точный id, если известен" },
        limit: { type: "number", description: "сколько вернуть, по умолчанию 50" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "stratum_list_access",
    description: "Кто сейчас имеет доступ к режимам: человек, тип доступа (промокод/подписка/вручную), число режимов и срок. Без userId — весь список. Только чтение.",
    action: "access.list",
    inputSchema: {
      type: "object",
      properties: {
        userId: { type: "number", description: "ограничить одним пользователем" },
        limit: { type: "number" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "stratum_grant_access",
    description: "Выдать пользователю доступ к режимам на N дней (1–366). Режимы задаются списком modeIds либо берутся из тарифа tariffId. Пишется в журнал аудита. Не продлевает существующие записи, а добавляет новые.",
    action: "access.grant",
    inputSchema: {
      type: "object",
      properties: {
        userId: { type: "number" },
        modeIds: { type: "array", items: { type: "number" }, description: "id режимов" },
        tariffId: { type: "number", description: "взять режимы из тарифа" },
        days: { type: "number", description: "срок в днях, 1–366" },
        limit: { type: "number", description: "лимит сообщений в сутки, по умолчанию 50" },
        reason: { type: "string", description: "зачем выдаём — попадёт в журнал" },
      },
      required: ["userId", "days"],
      additionalProperties: false,
    },
  },
  {
    name: "stratum_revoke_access",
    description: "Закрыть пользователю весь живой доступ. Обратимо: записи не удаляются, у них проставляется срок окончания «сейчас», поэтому видно, что и когда было выдано. Пишется в журнал аудита.",
    action: "access.revoke",
    inputSchema: {
      type: "object",
      properties: {
        userId: { type: "number" },
        reason: { type: "string", description: "причина — попадёт в журнал" },
      },
      required: ["userId"],
      additionalProperties: false,
    },
  },
  {
    name: "stratum_list_tariffs",
    description: "Тарифы: название, тип, месячная и годовая цена, продаётся ли, в архиве ли, суточный лимит сообщений. Только чтение.",
    action: "tariffs.list",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
  {
    name: "stratum_list_promocodes",
    description: "Промокоды: код, срок действия выдаваемого доступа, тариф, число активаций, окно действия, комментарий и признак временной админки. Только чтение.",
    action: "promo.list",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "часть кода или комментария" },
        limit: { type: "number" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "stratum_create_promocode",
    description: "Завести промокод на тариф: срок выдаваемого доступа в днях, число активаций, комментарий. Окно действия самого кода — 30 дней с сегодняшнего дня. Временная админка не включается. Пишется в журнал аудита.",
    action: "promo.create",
    inputSchema: {
      type: "object",
      properties: {
        tariffId: { type: "number" },
        days: { type: "number", description: "на сколько дней даёт доступ, 1–366" },
        maxUses: { type: "number", description: "сколько раз можно активировать, по умолчанию 1" },
        code: { type: "string", description: "свой код; без него сгенерируется" },
        limit: { type: "number", description: "лимит сообщений в сутки" },
        comment: { type: "string" },
      },
      required: ["tariffId", "days"],
      additionalProperties: false,
    },
  },
  {
    name: "stratum_update_promocode",
    description: "Поменять у НЕактивированного промокода срок выдаваемого доступа, число активаций или комментарий. Активированный не меняется: у человека доступ уже выдан на прежних условиях. Пишется в журнал аудита.",
    action: "promo.update",
    inputSchema: {
      type: "object",
      properties: {
        code: { type: "string" },
        days: { type: "number", description: "новый срок в днях" },
        maxUses: { type: "number" },
        comment: { type: "string" },
      },
      required: ["code"],
      additionalProperties: false,
    },
  },
  {
    name: "security_events",
    description: "Журнал попыток взлома практикума: подбор ключа, чужие сессии, невозможные тайминги ответов, подмена счёта в отчёте. Можно отфильтровать по виду: bad_key, alien_session, impossible_timing, forged_score, rate_limited. Только чтение.",
    action: "security.events",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "вид события, например forged_score" },
        limit: { type: "number" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "tir_results",
    description: "Результаты практического занятия 1 «Отдел Н»: кто сдал, сколько верных ответов, очки. Можно ограничить группой. Только чтение.",
    action: "tir.results",
    inputSchema: {
      type: "object",
      properties: { group: { type: "string", description: "номер группы, например РИ-410001" } },
      additionalProperties: false,
    },
  },
];

/* ── service call ────────────────────────────────────────────────────── */
async function call(action, args) {
  if (!KEY) {
    return { ok: false, error: "не задан MINDSTRATA_MCP_KEY — сервер управления не примет команду" };
  }
  const body = JSON.stringify({ action, ...args });
  try {
    const res = await fetch(`${API}/api/mcp/call`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-MCP-Key": KEY },
      body,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || !data.ok) {
      // Pass the status code through as is: 401 means "wrong key", 503 means "no key
      // configured on the server", and they need different fixes.
      return { ok: false, error: `HTTP ${res.status}: ${data.error || "без текста"}` };
    }
    return data;
  } catch (e) {
    return { ok: false, error: `сеть недоступна: ${e.message}` };
  }
}

/* ── JSON-RPC over stdio ───────────────────────────────────────────────── */
function send(msg) {
  process.stdout.write(JSON.stringify(msg) + "\n");
}
function result(id, payload) {
  send({ jsonrpc: "2.0", id, result: payload });
}
function failure(id, code, message) {
  send({ jsonrpc: "2.0", id, error: { code, message } });
}

async function handle(msg) {
  const { id, method, params } = msg;
  switch (method) {
    case "initialize":
      return result(id, {
        protocolVersion: params?.protocolVersion || "2024-11-05",
        capabilities: { tools: {} },
        serverInfo: { name: NAME, version: VERSION },
      });
    case "notifications/initialized":
      return; // notification, no reply expected
    case "ping":
      return result(id, {});
    case "tools/list":
      return result(id, {
        tools: TOOLS.map(({ name, description, inputSchema }) => ({ name, description, inputSchema })),
      });
    case "tools/call": {
      const tool = TOOLS.find((t) => t.name === params?.name);
      if (!tool) return failure(id, -32602, `нет такого инструмента: ${params?.name}`);
      const data = await call(tool.action, params.arguments || {});
      // Report errors via isError, not a JSON-RPC error: the agent must
      // see the text and decide what to do, not get a broken call.
      return result(id, {
        content: [{ type: "text", text: JSON.stringify(data, null, 2) }],
        isError: data.ok === false,
      });
    }
    default:
      if (id !== undefined) failure(id, -32601, `метод не поддержан: ${method}`);
  }
}

let buffer = "";
// Counter of unfinished calls. Claude Code keeps stdin open, but when
// tested through a pipe it closes immediately, and exiting on "end" cut off
// replies to already accepted requests that were waiting on the network. Exit
// only once everything accepted is done.
let inflight = 0;
let stdinClosed = false;
let draining = false;
// Exit only when stdin is closed, the queue is drained and no call is
// waiting on the network. The first version counted only inflight and managed to
// exit while unparsed lines were still in the buffer: over a pipe every reply
// except the first was lost.
const maybeExit = () => { if (stdinClosed && !draining && inflight === 0 && buffer.trim() === "") process.exit(0); };

process.stdin.setEncoding("utf8");
process.stdin.on("data", (chunk) => { buffer += chunk; drain(); });

async function drain() {
  if (draining) return;
  draining = true;
  let idx;
  while ((idx = buffer.indexOf("\n")) >= 0) {
    const line = buffer.slice(0, idx).trim();
    buffer = buffer.slice(idx + 1);
    if (!line) continue;
    let msg;
    try {
      msg = JSON.parse(line);
    } catch {
      continue; // a garbage line must not crash the server
    }
    // Calls run in parallel: one slow network request must not
    // delay parsing and running the next ones.
    inflight++;
    handle(msg)
      .catch((e) => { if (msg.id !== undefined) failure(msg.id, -32603, e.message); })
      .finally(() => { inflight--; maybeExit(); });
  }
  draining = false;
  maybeExit();
}
process.stdin.on("end", () => { stdinClosed = true; maybeExit(); });
