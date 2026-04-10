# =============================================================================
# Speaker Analysis Service Docker 镜像
# =============================================================================

ARG PIP_INDEX=https://mirrors.aliyun.com/pypi/simple/
ARG PIP_TRUSTED_HOST=mirrors.aliyun.com

FROM python:3.10-slim

ARG VERSION=1.1.0
ARG PIP_INDEX
ARG PIP_TRUSTED_HOST

ENV APP_VERSION=${VERSION} \
	PYTHONUNBUFFERED=1 \
	PYTHONDONTWRITEBYTECODE=1 \
	MODELSCOPE_CACHE=/app/models \
	HF_HUB_OFFLINE=1 \
	TRANSFORMERS_OFFLINE=1 \
	DEVICE=cpu \
	PORT=8100 \
	HOST=0.0.0.0 \
	WORKERS=1

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
	curl \
	ffmpeg \
	git \
	libsndfile1 \
	&& rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
COPY scripts/ ./scripts/
COPY wheels/ ./wheels/

RUN pip install --no-cache-dir -r requirements.txt \
	-i ${PIP_INDEX} \
	--trusted-host ${PIP_TRUSTED_HOST}

RUN bash ./scripts/install_speakerlab.sh

COPY src/ ./src/
COPY config/ ./config/
COPY models/ ./models/

RUN python scripts/init_db.py

VOLUME ["/app/data", "/app/logs"]

EXPOSE 8100

HEALTHCHECK --interval=30s --timeout=10s --start-period=90s --retries=3 \
	CMD sh -c 'curl -f http://localhost:${PORT:-8100}/api/v1/health || exit 1'

CMD ["sh", "-c", "python -m uvicorn src.server:app --host ${HOST:-0.0.0.0} --port ${PORT:-8100} --workers ${WORKERS:-1}"]