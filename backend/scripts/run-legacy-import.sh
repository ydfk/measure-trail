#!/usr/bin/env sh
set -eu

source_path="${MEASURETRAIL_LEGACY_SOURCE:-/import/slimtrack.db}"
staged_directory=/tmp/measuretrail-legacy-import
staged_path="$staged_directory/slimtrack.db"

if [ ! -f "$source_path" ]; then
  echo "找不到旧数据库：$source_path" >&2
  exit 1
fi

rm -rf "$staged_directory"
mkdir -p "$staged_directory"
cp "$source_path" "$staged_path"
chown -R measuretrail:measuretrail "$staged_directory"
chmod 0400 "$staged_path"

exec su-exec measuretrail:measuretrail /app/measuretrail-legacy-import --source "$staged_path" "$@"
