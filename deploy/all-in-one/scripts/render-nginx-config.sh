#!/bin/sh
set -eu

CONFIG_PATH=/etc/nginx/conf.d/default.conf
HTTP_REDIRECT_TO_HTTPS=${ASR_HTTP_REDIRECT_TO_HTTPS:-0}

COMMON_LOCATIONS=$(cat <<'EOF'
  root /srv/asr/frontend-dist;
  index index.html;
  client_max_body_size 1024m;

  location = /healthz {
    proxy_pass http://127.0.0.1:10010/healthz;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }

  location = /readyz {
    proxy_pass http://127.0.0.1:10010/readyz;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }

  location / {
    try_files $uri $uri/ /index.html;
  }

  location /api/ {
    proxy_pass http://127.0.0.1:10010;
    proxy_http_version 1.1;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }

  location /ws/ {
    proxy_pass http://127.0.0.1:10010;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }

  location /uploads/ {
    proxy_pass http://127.0.0.1:10010;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-Host $host;
  }

  location /downloads/files/ {
    alias /var/lib/asr/downloads/;
    autoindex off;
    add_header Cache-Control "public, max-age=60";
    types {
      application/octet-stream exe;
      application/octet-stream msi;
      application/zip zip;
      application/x-7z-compressed 7z;
      application/gzip tgz;
    }
    default_type application/octet-stream;
  }

  location = /downloads/certs/tls.crt {
    alias /var/lib/asr/certs/tls.crt;
    add_header Cache-Control "public, max-age=60";
    add_header Content-Disposition 'attachment; filename="asr-server.crt"';
    default_type application/x-x509-ca-cert;
  }
EOF
)

if [ "${ASR_ENABLE_HTTPS:-1}" = "1" ]; then
  if [ "$HTTP_REDIRECT_TO_HTTPS" = "1" ]; then
    cat > "$CONFIG_PATH" <<EOF
server {
  listen 80;
  server_name _;
  return 301 https://\$host\$request_uri;
}

server {
  listen 443 ssl;
  server_name _;
  ssl_certificate ${ASR_TLS_CERT_FILE};
  ssl_certificate_key ${ASR_TLS_KEY_FILE};
  ssl_session_timeout 1d;
  ssl_session_cache shared:SSL:10m;
  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_prefer_server_ciphers off;

${COMMON_LOCATIONS}
}
EOF
  else
    cat > "$CONFIG_PATH" <<EOF
server {
  listen 80;
  server_name _;

${COMMON_LOCATIONS}
}

server {
  listen 443 ssl;
  server_name _;
  ssl_certificate ${ASR_TLS_CERT_FILE};
  ssl_certificate_key ${ASR_TLS_KEY_FILE};
  ssl_session_timeout 1d;
  ssl_session_cache shared:SSL:10m;
  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_prefer_server_ciphers off;

${COMMON_LOCATIONS}
}
EOF
  fi
else
  cat > "$CONFIG_PATH" <<EOF
server {
  listen 80;
  server_name _;

${COMMON_LOCATIONS}
}
EOF
fi