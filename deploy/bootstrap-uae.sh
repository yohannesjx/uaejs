#!/usr/bin/env bash
# Run on your server (as root or with sudo) after creating /uae.
# Usage:
#   curl -fsSL ... | bash
# or copy this file to the server and: bash bootstrap-uae.sh
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/yohannesjx/uaejs.git}"
TARGET="${TARGET:-/uae}"

echo ">>> Deploy target: $TARGET"

if ! command -v git >/dev/null 2>&1; then
  echo "Install git first (e.g. apt install -y git / yum install -y git)."
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "Install Docker + Docker Compose plugin first."
  exit 1
fi

mkdir -p "$TARGET"
cd "$TARGET"

if [ ! -d .git ]; then
  git clone "$REPO_URL" .
else
  git pull origin main
fi

if [ ! -f .env ]; then
  if [ -f .env.example ]; then
    cp .env.example .env
    echo ">>> Created .env from .env.example — edit secrets (POSTGRES_PASSWORD, REDIS_PASSWORD, JWT_SECRET) before production."
  fi
fi

echo ">>> Building and starting stack (docker compose)..."
docker compose up -d --build

echo ">>> If this Postgres volume was created before newer ./migrations were added, apply them once:"
echo ">>>   bash deploy/apply-migrations-existing-db.sh"

echo ">>> Done. API: http://<server-ip>:8080  Admin (prod): http://<server-ip>:3000  Storefront: http://<server-ip>:5173  Grafana: :3001"
echo ">>> Add to CORS_ALLOWED_ORIGINS: http://<server-ip>:3000 and http://<server-ip>:5173 (comma-separated)."
echo ">>> Ensure firewall allows required ports or put nginx/Caddy in front with TLS."
