# ========================================
# 说话人分离服务 Dockerfile
# 基于 3D-Speaker CAM++ 模型
# 支持离线运行（模型权重打包在镜像内）
# ========================================

# 国内 pip 镜像源
ARG PIP_INDEX=https://mirrors.aliyun.com/pypi/simple/
ARG PIP_TRUSTED_HOST=mirrors.aliyun.com

# --------------------------------------------------
# 阶段1：模型下载（联网环境中执行，仅构建时需要）
# --------------------------------------------------
FROM python:3.10-slim AS model-downloader

ARG PIP_INDEX
ARG PIP_TRUSTED_HOST
ARG TORCH_VERSION=2.5.1
ARG TORCHAUDIO_VERSION=2.5.1
ARG PYTORCH_INDEX_URL=https://download.pytorch.org/whl/cu124

RUN pip install --no-cache-dir modelscope \
    -i ${PIP_INDEX} \
    --trusted-host ${PIP_TRUSTED_HOST}

# 下载说话人分离模型
RUN python -c "\
from modelscope.hub.snapshot_download import snapshot_download; \
snapshot_download('damo/speech_campplus_speaker-diarization_common', cache_dir='/models')"

# 下载声纹提取模型
RUN python -c "\
from modelscope.hub.snapshot_download import snapshot_download; \
snapshot_download('damo/speech_campplus_sv_zh-cn_16k-common', cache_dir='/models')"

# --------------------------------------------------
# 阶段2：运行环境
# --------------------------------------------------
FROM python:3.10-slim AS runtime

ARG PIP_INDEX
ARG PIP_TRUSTED_HOST
ARG TORCH_VERSION=2.5.1
ARG TORCHAUDIO_VERSION=2.5.1
ARG PYTORCH_INDEX_URL=https://download.pytorch.org/whl/cu124

LABEL maintainer="Speech Team"
LABEL description="说话人分离服务 - 基于 3D-Speaker CAM++"
LABEL version="1.0.0"

# 系统依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        libsndfile1 \
        libgomp1 \
        curl \
    && rm -rf /var/lib/apt/lists/*

# 工作目录
WORKDIR /app

# ── 第一步：单独安装 PyTorch GPU 版（大包约 530MB，单独一层便于缓存） ──
# 固定版本并显式指定 CUDA 轮子源，避免随时间漂移到与宿主机驱动不兼容的新版本。
RUN pip install --no-cache-dir \
    torch==${TORCH_VERSION} \
    torchaudio==${TORCHAUDIO_VERSION} \
    --index-url ${PYTORCH_INDEX_URL}
 
# ── 第二步：安装其余 Python 依赖（小包，走阿里云镜像） ──
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt \
    -i ${PIP_INDEX} \
    --trusted-host ${PIP_TRUSTED_HOST}

# 从阶段1复制预下载的模型
COPY --from=model-downloader /models /app/models

# 在构建阶段预热 pipeline，确保运行时需要的附加模型资源也被打包进镜像。
RUN python - <<'PY'
import os
from modelscope.pipelines import pipeline

os.environ['MODELSCOPE_CACHE'] = '/app/models'
os.environ['MODEL_CACHE_DIR'] = '/app/models'

pipeline(
    task='speaker-diarization',
    model='damo/speech_campplus_speaker-diarization_common',
    device='cpu',
)
pipeline(
    task='speaker-verification',
    model='damo/speech_campplus_sv_zh-cn_16k-common',
    device='cpu',
)
PY

# 环境变量
ENV MODELSCOPE_CACHE=/app/models \
    MODEL_CACHE_DIR=/app/models \
    DEVICE=cpu \
    PORT=8080 \
    HOST=0.0.0.0 \
    WORKERS=1 \
    TEMP_DIR=/tmp/diarization \
    MAX_UPLOAD_MB=500 \
    # 离线模式：禁止 ModelScope 联网下载
    MODELSCOPE_DOMAIN="" \
    HF_HUB_OFFLINE=1 \
    TRANSFORMERS_OFFLINE=1

# 创建临时目录
RUN mkdir -p /tmp/diarization

# 最后复制应用代码，避免仅修改服务逻辑时触发前面的大层重建。
COPY app.py .

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# 暴露端口
EXPOSE 8080

# 启动服务
CMD ["python", "app.py"]