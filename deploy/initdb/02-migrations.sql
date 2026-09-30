-- Migrations already contained in 01-schema.sql; tools/migrate.go skips them.
create table if not exists schema_migrations (
  filename text primary key,
  applied_at timestamptz not null default now()
);
