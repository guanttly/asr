#!/bin/sh
set -eu

check_mysql() {
  if ! mysql \
    -h 127.0.0.1 \
    -P "${ASR_DATABASE_PORT:-3306}" \
    -u"${ASR_DATABASE_USER:-root}" \
    -p"${ASR_DATABASE_PASSWORD:-${ASR_MYSQL_ROOT_PASSWORD}}" \
    -e 'SELECT 1' >/dev/null 2>&1; then
    echo "healthcheck failed: mysql is not ready" >&2
    return 1
  fi
}

check_http_endpoint() {
  NAME="$1"
  URL="$2"
  EXTRA_ARGS="${3:-}"

  if ! sh -c "curl $EXTRA_ARGS -fsS --max-time 3 '$URL' >/dev/null"; then
    echo "healthcheck failed: $NAME ($URL)" >&2
    return 1
  fi
}

check_mysql
check_http_endpoint "gateway ready" "http://127.0.0.1:10010/readyz"
check_http_endpoint "asr-api ready" "http://127.0.0.1:10011/readyz"
check_http_endpoint "admin-api ready" "http://127.0.0.1:10012/readyz"
check_http_endpoint "nlp-api ready" "http://127.0.0.1:10013/readyz"

if [ "${ASR_ENABLE_HTTPS:-1}" = "1" ]; then
  check_http_endpoint "nginx https health" "https://127.0.0.1/healthz" "-k"
else
  check_http_endpoint "nginx http health" "http://127.0.0.1/healthz"
fi