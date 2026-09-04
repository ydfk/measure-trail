#!/bin/sh
set -eu

source_path="${1:?必须提供备份文件路径}"
database_path="${MEASURETRAIL_SQLITE_PATH:-/app/data/measuretrail.sqlite}"

test "${MEASURETRAIL_RESTORE_CONFIRMED:-}" = "YES"
test -f "$source_path"
if test -f "$source_path.sha256"; then
	expected_checksum="$(awk 'NR == 1 { print $1 }' "$source_path.sha256")"
	actual_checksum="$(sha256sum "$source_path" | awk '{ print $1 }')"
	test -n "$expected_checksum"
	test "$expected_checksum" = "$actual_checksum"
fi
sqlite3 "$source_path" "PRAGMA integrity_check" | grep -qx 'ok'
temporary_path="$database_path.restore-$$"
sqlite3 "$source_path" ".backup '$temporary_path'"
sqlite3 "$temporary_path" "PRAGMA integrity_check" | grep -qx 'ok'
rm -f "$database_path-wal" "$database_path-shm"
mv "$temporary_path" "$database_path"
