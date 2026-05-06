#!/usr/bin/env bash
# Apply all migrations/*.sql in order to an *already initialized* Postgres.
# Docker Compose only runs init scripts on first empty volume; use this after pulling new migrations.
#
# Usage (from repo root on the server):
#   bash deploy/apply-migrations-existing-db.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -f .env ]]; then
	set -a
	# shellcheck disable=SC1091
	source .env
	set +a
fi

PG_USER="${POSTGRES_USER:-dubai_admin}"
PG_DB="${POSTGRES_DB:-dubai_retail}"

echo ">>> Applying SQL migrations to database '$PG_DB' as '$PG_USER' (postgres service)"

shopt -s nullglob
files=(migrations/*.sql)
IFS=$'\n' sorted="$(printf '%s\n' "${files[@]}" | sort)"
unset IFS

while IFS= read -r f; do
	[[ -z "${f:-}" ]] && continue
	base="$(basename "$f")"
	echo ">>> $base"
	docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$PG_USER" -d "$PG_DB" <"$f"
done <<< "$sorted"

echo ">>> Done."
