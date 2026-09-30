#!/usr/bin/env bash
# Secret scan with gitleaks over a commit range (a thin wrapper: downloads a
# pinned, checksum-verified gitleaks and runs it).
#
#   bash scripts/scan-secrets.sh                  # HEAD~1..HEAD
#   bash scripts/scan-secrets.sh origin/main..HEAD
#   bash scripts/scan-secrets.sh --staged         # pre-commit hook
set -euo pipefail

GITLEAKS_VERSION="${GITLEAKS_VERSION:-8.28.0}"
GITLEAKS_SHA256="${GITLEAKS_SHA256:-a65b5253807a68ac0cafa4414031fd740aeb55f54fb7e55f386acb52e6a840eb}"
CACHE_DIR="${GITLEAKS_CACHE_DIR:-$HOME/.cache/gitleaks}"
BIN="$CACHE_DIR/gitleaks-$GITLEAKS_VERSION"
ROOT="$(git rev-parse --show-toplevel)"
CONFIG="$ROOT/.gitleaks.toml"

if ! { [ -x "$BIN" ] && "$BIN" version 2>/dev/null | grep -Fq "$GITLEAKS_VERSION"; }; then
  mkdir -p "$CACHE_DIR"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  curl -sSL --max-time 180 -o "$tmp/gitleaks.tgz" \
    "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
  # A security tool downloaded without verification is a way to run someone else's code.
  got="$(sha256sum "$tmp/gitleaks.tgz" | cut -d' ' -f1)"
  if [ "$got" != "$GITLEAKS_SHA256" ]; then
    echo "gitleaks checksum mismatch: $got" >&2
    exit 2
  fi
  tar -xzf "$tmp/gitleaks.tgz" -C "$tmp" gitleaks
  mv "$tmp/gitleaks" "$BIN"
  chmod 755 "$BIN"
fi

if [ "${1:-}" = "--staged" ]; then
  exec "$BIN" git --no-banner --redact --staged --config "$CONFIG" "$ROOT"
fi

RANGE="${1:-}"
if [ -z "$RANGE" ]; then
  if git rev-parse --verify -q HEAD~1 >/dev/null; then RANGE="HEAD~1..HEAD"; else RANGE="HEAD"; fi
fi
# A push that creates a branch has no "before" commit (all zeros): scan HEAD.
case "$RANGE" in 0000000000000000000000000000000000000000..*) RANGE="HEAD" ;; esac

echo "[scan-secrets] range: $RANGE"
if ! "$BIN" git --no-banner --redact --config "$CONFIG" --log-opts="$RANGE" "$ROOT"; then
  echo "[scan-secrets] SECRET FOUND. Removing it from the commit is not enough: rotate it." >&2
  exit 1
fi
echo "[scan-secrets] clean"
