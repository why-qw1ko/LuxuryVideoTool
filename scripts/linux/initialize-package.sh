#!/usr/bin/env bash
set -euo pipefail

package_root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="$package_root/.env"
db_path="$package_root/data/app.db"
key_path="$package_root/secrets/jwt.key"

if [[ ! -f "$env_file" ]]; then
  cp "$package_root/.env.example" "$env_file"
fi

mkdir -p "$(dirname "$db_path")" "$(dirname "$key_path")"
if [[ ! -f "$key_path" ]]; then
  head -c 32 /dev/urandom > "$key_path"
  chmod 600 "$key_path"
fi

read -r -s -p "Enter administrator password (at least 12 characters): " password
printf '\n'
if (( ${#password} < 12 )); then
  echo "Password must contain at least 12 characters." >&2
  exit 1
fi

password_file="$(mktemp)"
trap 'rm -f "$password_file"' EXIT
printf '%s' "$password" > "$password_file"
chmod 600 "$password_file"

DATABASE_PATH="$db_path" "$package_root/douyin-capture-admin" create-user \
  --username owner \
  --display-name Owner \
  --role admin \
  --password-file "$password_file"

echo "Initialization completed. Run ./start-web.sh to start the service."
