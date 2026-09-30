# Test runner on your computer

Optional. Without it CI in your fork runs on GitHub's machines, which is fine.
With it your fork's CI runs on your computer: faster and without queues.

## Three steps

1. Fork the repository on GitHub.
2. Run the installer and answer two questions (fork name and a token):
   - Linux, WSL, macOS: `bash runner/install.sh`
   - Windows: `powershell -ExecutionPolicy Bypass -File runner\install.ps1`
3. In your fork add the Actions variable `MINDSTRATA_SELF_HOSTED = true`
   (the Linux installer can do it for you with the `gh` CLI).

## Safety

- Register the runner on **your fork only**, never on the main repository.
  CI in the main repository never uses self-hosted runners: pull requests
  from strangers arrive there. `tools/prcheck runner-safety` fails CI if a
  workflow tries.
- In your fork the runner takes only `push` events and only while the
  variable is on. Pull requests opened against your fork do not run on your
  machine.
- The runner is ephemeral: every job starts in a fresh container.
- The host Docker socket is not mounted; jobs cannot control your Docker.
- No secrets are given to jobs. The token in `runner/.env` is only used to
  register the runner and is removed from the environment before a job starts.
- CPU and memory are limited (`RUNNER_CPUS`, `RUNNER_MEMORY` in `runner/.env`).

Stop: `docker compose -f runner/docker-compose.yml down`.

---

# Раннер тестов на своём компьютере

Необязателен: без него CI вашего форка идёт на машинах GitHub. С ним — на
вашем компьютере, быстрее и без очереди.

1. Сделайте форк на GitHub.
2. Запустите установщик и ответьте на два вопроса (имя форка и токен):
   `bash runner/install.sh` или на Windows
   `powershell -ExecutionPolicy Bypass -File runner\install.ps1`.
3. В форке добавьте переменную Actions `MINDSTRATA_SELF_HOSTED = true`.

Раннер ставится только на **свой форк**. В основном репозитории self-hosted
не используется никогда — туда приходят PR от незнакомых людей. Раннер
эфемерный, без доступа к Docker хоста, без секретов, с лимитами CPU и памяти.
