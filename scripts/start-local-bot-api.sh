#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required but not found in PATH"
  exit 1
fi

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${TELEGRAM_API_ID:-}" || -z "${TELEGRAM_API_HASH:-}" ]]; then
  echo "Set TELEGRAM_API_ID and TELEGRAM_API_HASH in .env first."
  echo "Get them from: https://my.telegram.org"
  exit 1
fi

mkdir -p "$ROOT_DIR/tgdata"

docker rm -f telegram-bot-api-local >/dev/null 2>&1 || true

docker run -d \
  --name telegram-bot-api-local \
  --restart unless-stopped \
  -p 8081:8081 \
  -v "$ROOT_DIR/tgdata:/var/lib/telegram-bot-api" \
  ghcr.io/tdlib/telegram-bot-api:latest \
  --api-id="$TELEGRAM_API_ID" \
  --api-hash="$TELEGRAM_API_HASH" \
  --http-port=8081 \
  --local >/dev/null

echo "telegram-bot-api is running on http://127.0.0.1:8081"
echo "Check logs: docker logs -f telegram-bot-api-local"
