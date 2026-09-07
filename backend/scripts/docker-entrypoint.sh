#!/usr/bin/env sh
set -eu

for directory in /app/data /app/log /app/backups; do
  mkdir -p "$directory"
  chown -R measuretrail:measuretrail "$directory"
done

exec su-exec measuretrail:measuretrail /app/measuretrail-api "$@"
