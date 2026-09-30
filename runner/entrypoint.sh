#!/usr/bin/env bash
# Registers an ephemeral runner on the contributor's FORK and takes one job.
#
# After the job the runner exits, Docker starts the container again
# (restart: unless-stopped), and the next job begins from a clean state.
# A registration token lives one hour, so a fresh one is requested every
# time with the contributor's personal token (GITHUB_PAT from runner/.env).
set -euo pipefail

: "${GITHUB_REPO:?GITHUB_REPO (your fork, owner/repo) is not set in runner/.env}"
: "${GITHUB_PAT:?GITHUB_PAT is not set in runner/.env}"

if [ "${EPHEMERAL:-true}" != "true" ]; then
  echo "this runner only works in ephemeral mode (EPHEMERAL=true)" >&2
  exit 1
fi

token="$(curl -fsS -X POST \
  -H "Authorization: Bearer ${GITHUB_PAT}" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/repos/${GITHUB_REPO}/actions/runners/registration-token" | jq -r .token)"
if [ -z "$token" ] || [ "$token" = "null" ]; then
  echo "GitHub did not issue a registration token: check GITHUB_REPO and the token permission (Administration: write)" >&2
  sleep 60 # do not hammer the API in a restart loop
  exit 1
fi

./config.sh --unattended --ephemeral --replace --disableupdate \
  --url "https://github.com/${GITHUB_REPO}" \
  --token "$token" \
  --name "${RUNNER_NAME:-mindstrata-$(hostname)}" \
  --labels mindstrata-runner

# Jobs neither need nor may see the personal token: code in a fork can come
# from anyone.
unset GITHUB_PAT token
exec ./run.sh
