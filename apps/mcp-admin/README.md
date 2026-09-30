# MCP-управление Mindstrata

Сервер даёт Claude-агентам команды к сервису: найти человека, посмотреть и
выдать доступ, завести и поправить промокод, снять сводку, забрать результаты
практикума «Отдел Н».

Устроен так: `server.mjs` (JSON-RPC по stdio, без зависимостей) → `POST
/api/mcp/call` на сервере → база. Проверка ключа, белый список команд и запись
в журнал — на стороне API (`apps/api/internal/httpapi/mcp_admin.go`), а не
здесь: клиент можно подменить, сервер — нет.

## Ключ

Живёт в `system_settings.mcp_admin_key`. Завести или сменить:

```sql
insert into system_settings (key, value)
values ('mcp_admin_key', '<32 байта из openssl rand -hex 32>')
on conflict (key) do update set value = excluded.value, updated_at = now();
```

Пока ключа нет, сервис отвечает `503` на любую команду — это не поломка, а
состояние «управление выключено».

## Подключение к Claude Code

```bash
claude mcp add mindstrata \
  --env MINDSTRATA_API_URL=https://mindstrata.ru \
  --env MINDSTRATA_MCP_KEY=<ключ> \
  -- node /path/to/repo/apps/mcp-admin/server.mjs
```

Либо в `.mcp.json` проекта:

```json
{
  "mcpServers": {
    "mindstrata": {
      "command": "node",
      "args": ["apps/mcp-admin/server.mjs"],
      "env": {
        "MINDSTRATA_API_URL": "https://mindstrata.ru",
        "MINDSTRATA_MCP_KEY": "…"
      }
    }
  }
}
```

⚠️ Ключ передаётся только через окружение. В аргументах командной строки он был
бы виден в `ps` любому процессу на машине.

Проверка без Claude:

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | MINDSTRATA_MCP_KEY=… node apps/mcp-admin/server.mjs
```

## Команды

| Инструмент | Что делает | Пишется в журнал |
|---|---|---|
| `stratum_stats` | сводка: пользователи, доступы, подписки, отчёты практики | — |
| `stratum_find_user` | поиск по имени, нику, почте, id | — |
| `stratum_list_access` | у кого живой доступ, какого типа и до какого числа | — |
| `stratum_grant_access` | выдать доступ к режимам на 1–366 дней | да |
| `stratum_revoke_access` | закрыть весь живой доступ человеку | да |
| `stratum_list_tariffs` | тарифы, цены, что продаётся | — |
| `stratum_list_promocodes` | промокоды, сроки, активации | — |
| `stratum_create_promocode` | завести промокод на тариф | да |
| `stratum_update_promocode` | поправить **неактивированный** промокод | да |
| `tir_results` | результаты практики 1, можно по группе | — |

## Что сервер намеренно не умеет

- **Удалять** — ни пользователей, ни доступы, ни промокоды. Закрытие доступа
  проставляет срок окончания, запись остаётся: видно, что было выдано и когда.
- **Менять активированные промокоды** — у человека доступ уже выдан на прежних
  условиях, и правка задним числом дала бы спор без следов.
- **Включать временную админку** промокоду. Этот флаг открывает срез аудитории
  и экспорт; такое включается руками и осознанно.
- **Трогать тарифы и цены.** Продажи — не то место, где агент должен
  действовать без человека.

Каждое изменение пишется в `admin_audit_log` с `actor_user_id = NULL`,
`section = access`, `sensitive = true` и пометкой `source: mcp` в `meta` —
действия агента всегда отличимы от ручных.
