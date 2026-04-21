# syntax=docker/dockerfile:1.7
# =============================================================================
# Speaker Analysis Service Docker 镜像
# =============================================================================

ARG PIP_INDEX=https://mirrors.aliyun.com/pypi/simple/
ARG PIP_TRUSTED_HOST=mirrors.aliyun.com
ARG RUNTIME_EXTRA_INDEX=
ARG RUNTIME_EXTRA_TRUSTED_HOST=
ARG TORCH_INDEX=https://download.pytorch.org/whl/cpu
ARG TORCH_TRUSTED_HOST=download.pytorch.org
ARG TORCH_PACKAGES="torch==2.4.1+cpu torchaudio==2.4.1+cpu"
ARG APT_MIRROR=https://mirrors.aliyun.com/debian
ARG APT_SECURITY_MIRROR=https://mirrors.aliyun.com/debian-security

FROM python:3.10-slim

ARG VERSION=1.1.0
ARG PIP_INDEX
ARG PIP_TRUSTED_HOST
ARG RUNTIME_EXTRA_INDEX
ARG RUNTIME_EXTRA_TRUSTED_HOST
ARG TORCH_INDEX
ARG TORCH_TRUSTED_HOST
ARG TORCH_PACKAGES
ARG APT_MIRROR
ARG APT_SECURITY_MIRROR

ENV APP_VERSION=${VERSION} \
	PYTHONUNBUFFERED=1 \
	PYTHONDONTWRITEBYTECODE=1 \
	PIP_DEFAULT_TIMEOUT=1200 \
	MODELSCOPE_CACHE=/app/data/native_cache \
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
COPY wheels/ ./wheels/

RUN grep -viE '^(torch|torchaudio)([<>=!~].*)?$' requirements.txt > /tmp/requirements.runtime.txt

RUN --mount=type=cache,target=/root/.cache/pip,id=speaker-analysis-pip-cache,sharing=locked \
	set -eux; \
	torch_wheel_count="$(find ./wheels/runtime -maxdepth 1 -type f \( -name 'torch-*.whl' -o -name 'torchaudio-*.whl' \) | wc -l | tr -d ' ')"; \
	if [ "${torch_wheel_count}" -ge 2 ]; then \
		pip install --retries 5 --no-index \
			--find-links ./wheels \
			--find-links ./wheels/runtime \
			${TORCH_PACKAGES}; \
	else \
		pip install --retries 5 \
			--find-links ./wheels \
			--find-links ./wheels/runtime \
			--index-url ${TORCH_INDEX} \
			--trusted-host ${TORCH_TRUSTED_HOST} \
			${TORCH_PACKAGES}; \
	fi

RUN --mount=type=cache,target=/root/.cache/pip,id=speaker-analysis-pip-cache,sharing=locked \
	set -eux; \
	runtime_hash="$(sha256sum /tmp/requirements.runtime.txt | awk '{print $1}')"; \
	cached_runtime_hash="$(tr -d '[:space:]' < ./wheels/runtime/.runtime-wheels-ready 2>/dev/null || true)"; \
	set -- pip install --retries 5 -r /tmp/requirements.runtime.txt \
		--find-links ./wheels \
		--find-links ./wheels/runtime \
		-i ${PIP_INDEX} \
		--trusted-host ${PIP_TRUSTED_HOST}; \
	if [ -n "${RUNTIME_EXTRA_INDEX}" ]; then \
		set -- "$@" --extra-index-url ${RUNTIME_EXTRA_INDEX}; \
	fi; \
	if [ -n "${RUNTIME_EXTRA_TRUSTED_HOST}" ]; then \
		set -- "$@" --trusted-host ${RUNTIME_EXTRA_TRUSTED_HOST}; \
	fi; \
	if [ -f ./wheels/runtime/.runtime-wheels-ready ] && [ "${cached_runtime_hash}" = "${runtime_hash}" ]; then \
		pip install --retries 5 --no-index \
			-r /tmp/requirements.runtime.txt \
			--find-links ./wheels \
			--find-links ./wheels/runtime; \
	else \
		"$@"; \
	fi

COPY scripts/ ./scripts/
COPY vendor/ ./vendor/

RUN --mount=type=cache,target=/root/.cache/pip,id=speaker-analysis-pip-cache,sharing=locked \
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

CMD ["sh", "-c", "sh ./scripts/prepare_runtime_cache.sh && python -m uvicorn src.server:app --host ${HOST:-0.0.0.0} --port ${PORT:-8100} --workers ${WORKERS:-1}"]