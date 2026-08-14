#!/usr/bin/env bash
set -euo pipefail

package_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="$package_root/.env"

if [[ ! -f "$env_file" ]]; then
  cp "$package_root/.env.example" "$env_file"
fi

set -a
# shellcheck disable=SC1090
. "$env_file"
set +a

export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:8080}"
export DATABASE_PATH="$package_root/data/app.db"
export DATA_DIR="$package_root/data"
export JWT_SIGNING_KEY_FILE="$package_root/secrets/jwt.key"

mkdir -p "$DATA_DIR" "$(dirname "$JWT_SIGNING_KEY_FILE")"
if [[ ! -f "$JWT_SIGNING_KEY_FILE" ]]; then
  head -c 32 /dev/urandom > "$JWT_SIGNING_KEY_FILE"
  chmod 600 "$JWT_SIGNING_KEY_FILE"
fi

if [[ ! -f "$DATABASE_PATH" ]]; then
  echo "Run ./initialize-admin.sh before starting the service." >&2
  exit 1
fi

exec "$package_root/douyin-capture-server"
