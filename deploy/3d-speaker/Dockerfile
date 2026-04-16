# syntax=docker/dockerfile:1.7
# =============================================================================
# Speaker Analysis Service Docker 镜像
# =============================================================================

ARG PIP_INDEX=https://mirrors.aliyun.com/pypi/simple/
ARG PIP_TRUSTED_HOST=mirrors.aliyun.com
ARG APT_MIRROR=https://mirrors.aliyun.com/debian
ARG APT_SECURITY_MIRROR=https://mirrors.aliyun.com/debian-security

FROM python:3.10-slim

ARG VERSION=1.1.0
ARG PIP_INDEX
ARG PIP_TRUSTED_HOST
ARG APT_MIRROR
ARG APT_SECURITY_MIRROR

ENV APP_VERSION=${VERSION} \
	PYTHONUNBUFFERED=1 \
	PYTHONDONTWRITEBYTECODE=1 \
	PIP_DEFAULT_TIMEOUT=1200 \
	MODELSCOPE_CACHE=/app/models/native_cache \
	HF_HUB_OFFLINE=1 \
	TRANSFORMERS_OFFLINE=1 \
	DEVICE=cpu \
	PORT=8100 \
	HOST=0.0.0.0 \
	WORKERS=1

WORKDIR /app

RUN set -eux; \
	if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
		sed -i "s|http://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
		sed -i "s|http://security.debian.org/debian-security|${APT_SECURITY_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
		sed -i "s|https://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
		sed -i "s|https://security.debian.org/debian-security|${APT_SECURITY_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
	fi; \
	if [ -f /etc/apt/sources.list ]; then \
		sed -i "s|http://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list; \
		sed -i "s|http://security.debian.org/debian-security|${APT_SECURITY_MIRROR}|g" /etc/apt/sources.list; \
		sed -i "s|https://deb.debian.org/debian|${APT_MIRROR}|g" /etc/apt/sources.list; \
		sed -i "s|https://security.debian.org/debian-security|${APT_SECURITY_MIRROR}|g" /etc/apt/sources.list; \
	fi; \
	apt-get update && apt-get install -y --no-install-recommends \
	curl \
	ffmpeg \
	git \
	libsndfile1 \
	&& rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
COPY scripts/ ./scripts/
COPY wheels/ ./wheels/
COPY vendor/ ./vendor/

RUN --mount=type=cache,target=/root/.cache/pip \
	pip install --retries 5 -r requirements.txt \
	-i ${PIP_INDEX} \
	--trusted-host ${PIP_TRUSTED_HOST}

RUN --mount=type=cache,target=/root/.cache/pip \
	PIP_INDEX_URL=${PIP_INDEX} \
	PIP_TRUSTED_HOST=${PIP_TRUSTED_HOST} \
	SPEAKERLAB_HOME=/opt/3d-speaker \
	bash ./scripts/install_speakerlab.sh pip python

COPY src/ ./src/
COPY config/ ./config/
COPY models/ ./models/

RUN python scripts/init_db.py

VOLUME ["/app/data", "/app/logs"]

EXPOSE 8100

HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=3 \
	CMD sh -c 'curl -f http://localhost:${PORT:-8100}/api/v1/health || exit 1'

CMD ["sh", "-c", "python -m uvicorn src.server:app --host ${HOST:-0.0.0.0} --port ${PORT:-8100} --workers ${WORKERS:-1}"]