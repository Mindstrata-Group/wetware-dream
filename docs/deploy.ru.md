# Развернуть на сервере

[English](deploy.md) — основная версия.

Один сервер, Docker, HTTPS автоматически. Файлы: `deploy/compose.prod.yml`,
`deploy/Caddyfile.example`, `deploy/.env.prod.example`.

## Требования к серверу

Оценка по настройкам ресурсов в `deploy/compose.prod.yml` (Postgres
`shared_buffers=256MB` и до 120 соединений, пул API 30, воркер 0,5 CPU /
128 МБ) плюс сервер Next.js и Caddy:

| | Минимум | Рекомендуется |
|---|---|---|
| CPU | 2 vCPU | 2–4 vCPU |
| Память | 2 ГБ + 2 ГБ swap | 4 ГБ |
| Диск | 20 ГБ SSD | 40 ГБ SSD + бэкапы в другом месте |

Это для **готовых образов** из реестра. Сборка веб-образа требует ещё около
3 ГБ памяти — собирайте на своём компьютере или в CI, не на маленьком сервере.

## Первый запуск за 7 шагов

1. Направьте A (и AAAA) запись домена на сервер, откройте порты 80 и 443.
2. Поставьте Docker с плагином Compose.
3. Возьмите файлы: `git clone https://github.com/Mindstrata-Group/wetware-dream && cd mindstrata`,
   `git checkout vX.Y.Z`.
4. `cp deploy/.env.prod.example deploy/.env.prod`, заполните все REQUIRED,
   секреты — `openssl rand -hex 32`. `chmod 600 deploy/.env.prod`.
5. `cp deploy/Caddyfile.example deploy/Caddyfile`.
6. Запуск: `docker compose -f deploy/compose.prod.yml --env-file deploy/.env.prod up -d`.
   Схема базы создаётся при первом старте, сертификат — за минуту.
7. Создайте первого администратора (README, раздел про администратора, с
   `https://ваш-домен`), войдите и добавьте шлюз ИИ в **Оркестрация → AI**.
   Ключи ИИ хранятся в базе, не в env.

Сокращение ниже: `dc() { docker compose -f deploy/compose.prod.yml --env-file deploy/.env.prod "$@"; }`

## Миграции

Новые версии могут добавить файлы в `apps/api/sql/`. Они применяются
**вручную**, а не автоматически при старте: миграция, упавшая на полпути на
проде или запущенная до бэкапа, откатывается куда тяжелее минуты ручной
работы. Порядок: бэкап → смотрим CHANGELOG → `dc --profile tools run --rm migrate`
(применяет только новые файлы, каждый в транзакции) → запускаем новую версию.

## Бэкап и восстановление

Каждый день, копии храните **не на этом сервере**:

```bash
dc exec -T postgres pg_dump -U mindstrata -Fc mindstrata > mindstrata-$(date +%F).dump
```

Восстановление (сначала остановить api и web):

```bash
dc stop api web
dc exec -T postgres dropdb -U mindstrata --if-exists mindstrata
dc exec -T postgres createdb -U mindstrata mindstrata
dc exec -T postgres pg_restore -U mindstrata -d mindstrata --no-owner < mindstrata-2026-01-01.dump
dc start api web
```

Проверьте восстановление хотя бы раз заранее.

## Обновление

Бэкап → `git fetch --tags && git checkout vNEW`, `MINDSTRATA_VERSION=vNEW` в
`deploy/.env.prod`, сверить новые переменные с примером → `dc pull` →
миграции, если есть → `dc up -d`.

## Откат

Прежний `MINDSTRATA_VERSION`, `git checkout vOLD`, `dc up -d`. Если новая
версия применила миграции, с которыми старая не работает (такие релизы — MAJOR
в CHANGELOG), восстановите бэкап, снятый перед обновлением.

## Фоновый воркер

Регулярные списания и обезличивание сообщений работают на Temporal. Без него
сайт работает, эти задачи — нет. Включение: сервер Temporal,
`TEMPORAL_ADDRESS`, запуск с `--profile worker`.
