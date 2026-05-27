#!/bin/sh
set -eu

if [ "${ASR_ENABLE_HTTPS:-1}" != "1" ]; then
  exit 0
fi

mkdir -p "$(dirname "$ASR_TLS_CERT_FILE")" "$(dirname "$ASR_TLS_KEY_FILE")"

if [ -s "$ASR_TLS_CERT_FILE" ] && [ -s "$ASR_TLS_KEY_FILE" ]; then
  exit 0
fi

ALT_NAMES=${ASR_TLS_ALT_NAMES:-DNS:localhost,IP:127.0.0.1}
COMMON_NAME=${ASR_TLS_COMMON_NAME:-localhost}

TMP_OPENSSL_CONFIG=$(mktemp)
cat > "$TMP_OPENSSL_CONFIG" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
x509_extensions = v3_req

[dn]
CN = ${COMMON_NAME}

[v3_req]
subjectAltName = ${ALT_NAMES}
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
EOF

openssl req -x509 -nodes -newkey rsa:2048 -days 3650 \
  -keyout "$ASR_TLS_KEY_FILE" \
  -out "$ASR_TLS_CERT_FILE" \
  -config "$TMP_OPENSSL_CONFIG"

rm -f "$TMP_OPENSSL_CONFIG"