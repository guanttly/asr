#!/bin/sh
set -eu

for _ in $(seq 1 120); do
  if mysql \
    -h "${ASR_DATABASE_HOST}" \
    -P "${ASR_DATABASE_PORT}" \
    -u"${ASR_DATABASE_USER}" \
    -p"${ASR_DATABASE_PASSWORD}" \
    -e 'SELECT 1' >/dev/null 2>&1; then
    exit 0
  fi
  sleep 1
done

echo "mysql is not ready after waiting" >&2
exit 1