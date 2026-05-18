#!/bin/sh
set -eu

export ASR_DATABASE_HOST="${ASR_DATABASE_HOST:-127.0.0.1}"
export ASR_DATABASE_PORT="${ASR_DATABASE_PORT:-3306}"
export ASR_DATABASE_USER="${ASR_DATABASE_USER:-root}"
export ASR_DATABASE_PASSWORD="${ASR_DATABASE_PASSWORD:-${ASR_MYSQL_ROOT_PASSWORD}}"
export ASR_DATABASE_DBNAME="${ASR_DATABASE_DBNAME:-${ASR_MYSQL_DATABASE}}"
export ASR_DOWNLOAD_DIR="${ASR_DOWNLOAD_DIR:-/var/lib/asr/downloads}"
export ASR_DOWNLOAD_PUBLIC_BASE_PATH="${ASR_DOWNLOAD_PUBLIC_BASE_PATH:-/downloads/files}"
export ASR_ENABLE_HTTPS="${ASR_ENABLE_HTTPS:-1}"
export ASR_TLS_CERT_DIR="${ASR_TLS_CERT_DIR:-/var/lib/asr/certs}"
export ASR_TLS_CERT_FILE="${ASR_TLS_CERT_FILE:-${ASR_TLS_CERT_DIR}/tls.crt}"
export ASR_TLS_KEY_FILE="${ASR_TLS_KEY_FILE:-${ASR_TLS_CERT_DIR}/tls.key}"
export ASR_TLS_COMMON_NAME="${ASR_TLS_COMMON_NAME:-localhost}"
export ASR_TLS_ALT_NAMES="${ASR_TLS_ALT_NAMES:-DNS:localhost,IP:127.0.0.1}"
export ASR_TMP_DIR="${ASR_TMP_DIR:-/var/lib/asr/tmp}"
export ASR_UPLOAD_DIR="${ASR_UPLOAD_DIR:-/var/lib/asr/uploads}"
export ASR_CATALOG_DIR="${ASR_CATALOG_DIR:-/var/lib/asr/term-catalog}"
export ASR_CATALOG_SEED_DIR="${ASR_CATALOG_SEED_DIR:-/opt/asr/term-catalog-default}"
export ASR_GATEWAY_ASR_API="${ASR_GATEWAY_ASR_API:-http://127.0.0.1:10011}"
export ASR_GATEWAY_ADMIN_API="${ASR_GATEWAY_ADMIN_API:-http://127.0.0.1:10012}"
export ASR_GATEWAY_NLP_API="${ASR_GATEWAY_NLP_API:-http://127.0.0.1:10013}"
export ASR_SERVER_HOST="${ASR_SERVER_HOST:-0.0.0.0}"

mkdir -p "${ASR_MYSQL_DATA_DIR}" "${ASR_DOWNLOAD_DIR}" "${ASR_TLS_CERT_DIR}" "${ASR_TMP_DIR}" "${ASR_UPLOAD_DIR}" "${ASR_CATALOG_DIR}" /run/mysqld /var/log/supervisor /var/log/nginx
chown -R mysql:mysql "${ASR_MYSQL_DATA_DIR}" /run/mysqld
chmod 1777 "${ASR_TMP_DIR}"

# Seed the term catalog mount on first boot. If the operator has already put
# their own md files in the host volume, skip the copy so we never overwrite
# customised content. The image still ships an embedded copy as a fallback,
# so an empty seed dir is non-fatal.
if [ -d "${ASR_CATALOG_SEED_DIR}" ] && [ -z "$(ls -A "${ASR_CATALOG_DIR}" 2>/dev/null)" ]; then
  echo "[entrypoint] seeding term catalog from ${ASR_CATALOG_SEED_DIR} into ${ASR_CATALOG_DIR}"
  cp -a "${ASR_CATALOG_SEED_DIR}/." "${ASR_CATALOG_DIR}/"
fi

/app/scripts/init-mysql.sh
/app/scripts/ensure-tls-cert.sh
/app/scripts/render-nginx-config.sh

exec /usr/bin/supervisord -c /etc/supervisor/conf.d/supervisord.conf