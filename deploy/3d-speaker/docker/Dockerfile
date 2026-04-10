# =============================================================================
# 3D-Speaker 说话人分离服务 Docker 镜像
# =============================================================================
FROM python:3.10-slim

ARG VERSION=1.0.0
ENV APP_VERSION=${VERSION}
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1

WORKDIR /app

# 系统依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    libsndfile1 \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Python 依赖
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 源码
COPY src/ ./src/
COPY config/ ./config/
COPY scripts/ ./scripts/

# 模型权重（构建时需已下载到 models/ 目录）
COPY models/ ./models/

# 初始化数据目录
RUN python scripts/init_db.py

# 数据卷（声纹库 + 日志 持久化）
VOLUME ["/app/data", "/app/logs"]

EXPOSE 8100

HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8100/api/v1/health')" || exit 1

CMD ["python", "-m", "uvicorn", "src.server:app", \
     "--host", "0.0.0.0", "--port", "8100", "--workers", "1"]
