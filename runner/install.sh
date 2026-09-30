#!/usr/bin/env bash
# Installs the test runner for YOUR FORK on Linux, WSL or macOS.
#
#   bash runner/install.sh
#
# Asks for your fork name and a fine-grained personal token, writes them to
# runner/.env (never committed, readable only by you) and starts the runner.
# The runner comes back after a reboot on its own.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
cd "$here"

say() { printf '\n%s\n' "$*"; }

if ! command -v docker >/dev/null 2>&1; then
  case "$(uname -s)" in
    Linux)
      say "Docker is not installed. Install it now with the official script (https://get.docker.com)? [y/N]"
      read -r answer
      if [ "${answer:-}" != "y" ] && [ "${answer:-}" != "Y" ]; then
        echo "Install Docker and run this script again." >&2
        exit 1
      fi
      curl -fsSL https://get.docker.com | sh
      sudo usermod -aG docker "$USER" || true
      say "Docker is installed. Log out and back in (or run 'newgrp docker'), then run this script again."
      exit 0
      ;;
    Darwin)
      echo "Install Docker Desktop (https://www.docker.com/products/docker-desktop/) and run this script again." >&2
      exit 1
      ;;
  esac
fi
docker compose version >/dev/null

say "Your fork on GitHub, for example: your-name/mindstrata"
printf 'Fork: '
read -r repo
case "$repo" in
  */*) ;;
  *) echo "expected owner/repo" >&2; exit 1 ;;
esac

say "Create a fine-grained token: GitHub -> Settings -> Developer settings -> Fine-grained tokens.
Repository access: ONLY your fork. Permission: Administration -> Read and write.
The token stays in runner/.env on this computer and is never committed."
printf 'Token (input hidden): '
read -rs pat
echo
if [ -z "$pat" ]; then
  echo "empty token" >&2
  exit 1
fi

umask 077
cat > .env <<EOF
GITHUB_REPO=$repo
GITHUB_PAT=$pat
RUNNER_CPUS=${RUNNER_CPUS:-2}
RUNNER_MEMORY=${RUNNER_MEMORY:-6g}
EOF
unset pat

docker compose up -d --build

say "The runner is starting. One last step: in your fork open
Settings -> Secrets and variables -> Actions -> Variables and add
MINDSTRATA_SELF_HOSTED = true
Without it CI in your fork keeps running on GitHub's machines."
if command -v gh >/dev/null 2>&1; then
  printf 'Set it now with the gh CLI? [y/N] '
  read -r answer
  if [ "${answer:-}" = "y" ] || [ "${answer:-}" = "Y" ]; then
    gh variable set MINDSTRATA_SELF_HOSTED --body true --repo "$repo"
  fi
fi
say "Check: docker compose -f runner/docker-compose.yml logs -f runner"
