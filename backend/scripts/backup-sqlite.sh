#!/bin/sh
set -eu

database_path="${MEASURETRAIL_SQLITE_PATH:-/app/data/measuretrail.sqlite}"
backup_dir="${MEASURETRAIL_BACKUP_DIR:-/app/backups}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_path="$backup_dir/measuretrail-$timestamp.sqlite"

test -f "$database_path"
mkdir -p "$backup_dir"
sqlite3 "$database_path" ".backup '$backup_path'"
sqlite3 "$backup_path" "PRAGMA integrity_check" | grep -qx 'ok'
sha256sum "$backup_path" > "$backup_path.sha256"
printf '%s\n' "$backup_path"
