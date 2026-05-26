FROM golang:1.25.4-bookworm AS backend-builder

ENV GOPROXY=https://goproxy.cn,direct \
    GOSUMDB=sum.golang.google.cn

WORKDIR /src/backend

COPY backend/go.mod backend/go.sum* ./
RUN go mod download

WORKDIR /src
COPY backend ./backend

WORKDIR /src/backend
RUN CGO_ENABLED=0 go build -o /out/gateway ./cmd/gateway \
 && CGO_ENABLED=0 go build -o /out/admin-api ./cmd/admin-api \
 && CGO_ENABLED=0 go build -o /out/asr-api ./cmd/asr-api \
 && CGO_ENABLED=0 go build -o /out/nlp-api ./cmd/nlp-api

FROM node:22-bookworm AS frontend-builder

ARG ASR_APP_VERSION
ARG ASR_BUILD_DATE
ENV ASR_APP_VERSION=${ASR_APP_VERSION}
ENV ASR_BUILD_DATE=${ASR_BUILD_DATE}
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com \
    COREPACK_NPM_REGISTRY=https://registry.npmmirror.com

WORKDIR /src/frontend

COPY frontend/package.json frontend/pnpm-lock.yaml* ./
RUN corepack enable && pnpm install --frozen-lockfile

COPY scripts/build-meta.ts /src/scripts/build-meta.ts
COPY frontend ./
RUN pnpm build

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN sed -i \
      -e 's#http://archive.ubuntu.com/ubuntu/#http://mirrors.aliyun.com/ubuntu/#g' \
      -e 's#http://security.ubuntu.com/ubuntu/#http://mirrors.aliyun.com/ubuntu/#g' \
      /etc/apt/sources.list.d/ubuntu.sources \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    ffmpeg \
    mysql-server \
    nginx \
    openssl \
    supervisor \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend-builder /out /app/bin
COPY --from=frontend-builder /src/frontend/dist /srv/asr/frontend-dist
RUN mkdir -p /app/backend/configs
COPY backend/configs/config.example.yaml /app/backend/configs/config.yaml
COPY backend/configs/config.example.yaml /app/backend/configs/config.example.yaml
COPY deploy/all-in-one/supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY deploy/all-in-one/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/all-in-one/scripts /app/scripts

# Ship the radiology term catalog markdown into the image as a *default*
# snapshot. entrypoint.sh seeds /var/lib/asr/term-catalog from it on first
# boot if the mounted volume is empty, so operators can edit md files via
# the host filesystem without rebuilding the image.
COPY docs/terms /opt/asr/term-catalog-default

RUN chmod +x /app/scripts/*.sh /app/bin/* \
 && mkdir -p /var/log/supervisor /var/log/nginx /run/mysqld /var/lib/asr/mysql /var/lib/asr/downloads /var/lib/asr/tmp /var/lib/asr/uploads /var/lib/asr/certs /var/lib/asr/term-catalog

ENV ASR_MYSQL_DATA_DIR=/var/lib/asr/mysql \
    ASR_MYSQL_DATABASE=asr \
    ASR_MYSQL_ROOT_PASSWORD=change-me \
    ASR_DOWNLOAD_DIR=/var/lib/asr/downloads \
    ASR_DOWNLOAD_PUBLIC_BASE_PATH=/downloads/files \
    ASR_ENABLE_HTTPS=1 \
    ASR_TLS_CERT_DIR=/var/lib/asr/certs \
    ASR_TLS_CERT_FILE=/var/lib/asr/certs/tls.crt \
    ASR_TLS_KEY_FILE=/var/lib/asr/certs/tls.key \
    ASR_TLS_COMMON_NAME=localhost \
    ASR_TLS_ALT_NAMES=DNS:localhost,IP:127.0.0.1 \
    ASR_TMP_DIR=/var/lib/asr/tmp \
    ASR_UPLOAD_DIR=/var/lib/asr/uploads \
    ASR_CATALOG_DIR=/var/lib/asr/term-catalog \
    ASR_CATALOG_SEED_DIR=/opt/asr/term-catalog-default \
    ASR_BOOTSTRAP_ADMIN_USERNAME=admin \
    ASR_BOOTSTRAP_ADMIN_PASSWORD=change-me-now \
    ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME=系统管理员 \
    ASR_JWT_SECRET=replace-with-random-secret \
    ASR_SERVICES_SUMMARY_MODEL=qwen3-4b

EXPOSE 80 443

VOLUME ["/var/lib/asr/mysql", "/var/lib/asr/downloads", "/var/lib/asr/tmp", "/var/lib/asr/uploads", "/var/lib/asr/certs", "/var/lib/asr/term-catalog"]

ENTRYPOINT ["/app/scripts/entrypoint.sh"]
