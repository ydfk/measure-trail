#!/usr/bin/env sh
set -eu

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root_dir"
MEASURETRAIL_SQLITE_PATH="${MEASURETRAIL_SQLITE_PATH:-/tmp/measuretrail-openapi.sqlite}" \
	MEASURETRAIL_JWT_ACCESS_SECRET="${MEASURETRAIL_JWT_ACCESS_SECRET:-01234567890123456789012345678901}" \
	MEASURETRAIL_JWT_REFRESH_SECRET="${MEASURETRAIL_JWT_REFRESH_SECRET:-abcdefghijklmnopqrstuvwxyzABCDEF}" \
  go run ./cmd/openapi ./openapi/openapi-3.1.json ./openapi/openapi-3.0.json
