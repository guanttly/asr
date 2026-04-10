"""
说话人分离服务 (Speaker Diarization Service)
基于阿里 3D-Speaker CAM++ 模型，提供 RESTful API 接口

功能：
  1. /diarize          - 上传音频文件，返回说话人分段结果
  2. /diarize/detail    - 上传音频文件，返回带声纹嵌入的详细结果
  3. /health           - 健康检查
"""

import os
import sys
import time
import uuid
import logging
import tempfile
import shutil
import re
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any, Optional

import numpy as np
import soundfile as sf
from fastapi import FastAPI, UploadFile, File, HTTPException, Query, Request
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel

# ──────────────────────────────────────────────
# 日志配置
# ──────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(name)s - %(message)s",
)
logger = logging.getLogger("speaker-diarization")


def getenv_nonempty(name: str, default: str) -> str:
    value = os.getenv(name)
    if value is None:
        return default
    value = value.strip()
    return value if value else default

# ──────────────────────────────────────────────
# 配置项（可通过环境变量覆盖）
# ──────────────────────────────────────────────
# 说话人分离模型（ModelScope 模型 ID 或本地路径）
DIARIZATION_MODEL = os.getenv(
    "DIARIZATION_MODEL",
    "damo/speech_campplus_speaker-diarization_common",
)
# 声纹提取模型（用于 /diarize/detail 接口提取 embedding）
SV_MODEL = os.getenv(
    "SV_MODEL",
    "damo/speech_campplus_sv_zh-cn_16k-common",
)
# 设备：cpu 或 cuda:0 等
DEVICE = getenv_nonempty("DEVICE", "cpu")
DIARIZATION_DEVICE = getenv_nonempty("DIARIZATION_DEVICE", DEVICE)
SV_DEVICE = getenv_nonempty("SV_DEVICE", DEVICE)
# 最大上传文件大小（MB）
MAX_UPLOAD_MB = int(os.getenv("MAX_UPLOAD_MB", "500"))
# 临时文件目录
TEMP_DIR = os.getenv("TEMP_DIR", "/tmp/diarization")
# 模型本地缓存目录
MODEL_CACHE_DIR = os.getenv("MODEL_CACHE_DIR", "/app/models")
# 支持的音频格式
SUPPORTED_EXTENSIONS = {".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".wma"}


def resolve_model_reference(model_ref: str) -> str:
    """优先使用镜像内已打包的本地模型目录，避免运行时再次访问远端仓库。"""
    if not model_ref:
        return model_ref

    direct_path = Path(model_ref)
    if direct_path.exists():
        return str(direct_path)

    candidates = [
        Path(MODEL_CACHE_DIR) / "models" / model_ref,
        Path(MODEL_CACHE_DIR) / model_ref,
    ]
    for candidate in candidates:
        if candidate.exists():
            return str(candidate)
    return model_ref


def resolve_runtime_device(requested_device: str, pipeline_name: str) -> str:
    """校验请求的 CUDA 设备号；无效时回退到可用设备。"""
    device = (requested_device or "cpu").strip()
    if not device.startswith("cuda"):
        return device

    try:
        import torch
    except Exception as exc:
        logger.warning(f"{pipeline_name} 无法导入 torch 校验设备，回退到 CPU: {exc}")
        return "cpu"

    if not torch.cuda.is_available():
        logger.warning(f"{pipeline_name} 请求使用 {device}，但容器内未检测到可用 CUDA，回退到 CPU")
        return "cpu"

    visible_count = torch.cuda.device_count()
    if visible_count <= 0:
        logger.warning(f"{pipeline_name} 请求使用 {device}，但容器内可见 GPU 数量为 0，回退到 CPU")
        return "cpu"

    matched = re.match(r"^cuda(?::(\d+))?$", device)
    ordinal = int(matched.group(1)) if matched and matched.group(1) is not None else 0
    if ordinal >= visible_count:
        fallback = "cuda:0"
        logger.warning(
            f"{pipeline_name} 请求使用 {device}，但容器内仅检测到 {visible_count} 张 GPU，"
            f"回退到 {fallback}"
        )
        return fallback

    return device


def should_fallback_to_cpu(exc: Exception) -> bool:
    message = str(exc).lower()
    markers = (
        "invalid device ordinal",
        "no cuda",
        "cuda error",
        "driver",
        "device-side assert",
        "not compiled with cuda",
    )
    return any(marker in message for marker in markers)


def build_pipeline(ms_pipeline, task: str, model: str, requested_device: str, pipeline_name: str):
    """按请求设备加载 pipeline；GPU 不可用时自动退回 CPU。"""
    runtime_device = resolve_runtime_device(requested_device, pipeline_name)
    try:
        return ms_pipeline(task=task, model=model, device=runtime_device), runtime_device
    except Exception as exc:
        if runtime_device != "cpu" and should_fallback_to_cpu(exc):
            logger.warning(
                f"{pipeline_name} 使用 {runtime_device} 加载失败，回退到 CPU: {exc}"
            )
            return ms_pipeline(task=task, model=model, device="cpu"), "cpu"
        raise

# ──────────────────────────────────────────────
# 全局模型实例
# ──────────────────────────────────────────────
diarization_pipeline = None
sv_pipeline = None
ACTIVE_DIARIZATION_DEVICE = DIARIZATION_DEVICE
ACTIVE_SV_DEVICE = SV_DEVICE

# ──────────────────────────────────────────────
# FastAPI 应用
# ──────────────────────────────────────────────
app = FastAPI(
    title="说话人分离服务",
    description="基于 3D-Speaker CAM++ 的说话人分离 API，支持局域网私有化部署",
    version="1.0.0",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


# ──────────────────────────────────────────────
# 数据模型
# ──────────────────────────────────────────────
class SpeakerSegment(BaseModel):
    speaker: str
    start: float
    end: float
    duration: float


class DiarizeResponse(BaseModel):
    task_id: str
    audio_duration: float
    num_speakers: int
    segments: list[SpeakerSegment]
    process_time: float


class LegacySpeakerSegment(BaseModel):
    speaker: str
    start_time: float
    end_time: float


class AudioURLRequest(BaseModel):
    audio_url: str
    num_speakers: Optional[int] = None


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    device: str
    diarization_model: str


# ──────────────────────────────────────────────
# 音频预处理：统一转为 16kHz 单声道 WAV
# ──────────────────────────────────────────────
def convert_to_wav_16k(input_path: str, output_path: str) -> float:
    """
    使用 ffmpeg 将任意音频格式转为 16kHz 单声道 WAV。
    返回音频时长（秒）。
    """
    import subprocess

    cmd = [
        "ffmpeg", "-y",
        "-i", input_path,
        "-ar", "16000",
        "-ac", "1",
        "-f", "wav",
        output_path,
    ]
    try:
        subprocess.run(cmd, capture_output=True, check=True, timeout=300)
    except subprocess.CalledProcessError as e:
        stderr = e.stderr.decode("utf-8", errors="replace")
        raise HTTPException(
            status_code=400,
            detail=f"音频转码失败: {stderr[:500]}",
        )
    except FileNotFoundError:
        raise HTTPException(
            status_code=500,
            detail="服务器未安装 ffmpeg，无法处理音频",
        )

    # 读取时长
    data, sr = sf.read(output_path)
    duration = len(data) / sr
    return duration


def _validate_uploaded_audio(file: UploadFile) -> str:
    """校验上传文件并返回扩展名。"""
    if file.size and file.size > MAX_UPLOAD_MB * 1024 * 1024:
        raise HTTPException(
            status_code=413,
            detail=f"文件大小超过限制（最大 {MAX_UPLOAD_MB}MB）",
        )

    ext = Path(file.filename or "unknown.wav").suffix.lower()
    if ext not in SUPPORTED_EXTENSIONS:
        raise HTTPException(
            status_code=400,
            detail=f"不支持的音频格式: {ext}，支持: {', '.join(SUPPORTED_EXTENSIONS)}",
        )
    return ext


def _guess_extension_from_url(audio_url: str) -> str:
    path = urllib.parse.urlparse(audio_url).path
    ext = Path(path).suffix.lower()
    if ext in SUPPORTED_EXTENSIONS:
        return ext
    return ".wav"


def _download_audio_from_url(audio_url: str, output_path: str):
    """下载远程音频到本地临时路径。"""
    try:
        request = urllib.request.Request(
            audio_url,
            headers={
                "User-Agent": "campp-diarization-service/1.0",
            },
        )
        with urllib.request.urlopen(request, timeout=300) as response:
            status_code = getattr(response, "status", 200)
            if status_code >= 400:
                raise HTTPException(
                    status_code=400,
                    detail=f"音频下载失败，HTTP {status_code}",
                )

            content_length = response.headers.get("Content-Length")
            if content_length:
                try:
                    if int(content_length) > MAX_UPLOAD_MB * 1024 * 1024:
                        raise HTTPException(
                            status_code=413,
                            detail=f"远程音频大小超过限制（最大 {MAX_UPLOAD_MB}MB）",
                        )
                except ValueError:
                    pass

            with open(output_path, "wb") as target:
                shutil.copyfileobj(response, target)
    except HTTPException:
        raise
    except urllib.error.HTTPError as e:
        raise HTTPException(
            status_code=400,
            detail=f"音频下载失败，HTTP {e.code}",
        ) from e
    except urllib.error.URLError as e:
        raise HTTPException(
            status_code=400,
            detail=f"音频下载失败: {e.reason}",
        ) from e


def _legacy_segments_response(segments: list[SpeakerSegment]) -> list[LegacySpeakerSegment]:
    """兼容现有 Go client：返回 speaker/start_time/end_time 数组。"""
    return [
        LegacySpeakerSegment(
            speaker=segment.speaker,
            start_time=segment.start,
            end_time=segment.end,
        )
        for segment in segments
    ]


def _run_diarization(wav_path: str) -> list[SpeakerSegment]:
    """执行分离并统一解析结果。"""
    result = diarization_pipeline(wav_path)
    return _parse_diarization_result(result)


# ──────────────────────────────────────────────
# 模型加载
# ──────────────────────────────────────────────
def load_models():
    """启动时加载说话人分离和声纹提取模型"""
    global diarization_pipeline, sv_pipeline, ACTIVE_DIARIZATION_DEVICE, ACTIVE_SV_DEVICE

    from modelscope.pipelines import pipeline as ms_pipeline

    logger.info(f"正在加载说话人分离模型: {DIARIZATION_MODEL}")
    logger.info(f"分离设备: {DIARIZATION_DEVICE}")
    logger.info(f"声纹设备: {SV_DEVICE}")

    os.makedirs(MODEL_CACHE_DIR, exist_ok=True)
    os.environ["MODELSCOPE_CACHE"] = MODEL_CACHE_DIR

    diarization_model_ref = resolve_model_reference(DIARIZATION_MODEL)
    sv_model_ref = resolve_model_reference(SV_MODEL)
    if diarization_model_ref != DIARIZATION_MODEL:
        logger.info(f"说话人分离模型使用本地缓存: {diarization_model_ref}")
    if sv_model_ref != SV_MODEL:
        logger.info(f"声纹提取模型使用本地缓存: {sv_model_ref}")

    # 加载说话人分离 pipeline
    diarization_pipeline, ACTIVE_DIARIZATION_DEVICE = build_pipeline(
        ms_pipeline=ms_pipeline,
        task="speaker-diarization",
        model=diarization_model_ref,
        requested_device=DIARIZATION_DEVICE,
        pipeline_name="说话人分离模型",
    )
    logger.info(f"说话人分离模型加载完成 (device={ACTIVE_DIARIZATION_DEVICE})")

    # 加载声纹提取 pipeline（用于详细接口）
    try:
        sv_pipeline, ACTIVE_SV_DEVICE = build_pipeline(
            ms_pipeline=ms_pipeline,
            task="speaker-verification",
            model=sv_model_ref,
            requested_device=SV_DEVICE,
            pipeline_name="声纹提取模型",
        )
        logger.info(f"声纹提取模型加载完成 (device={ACTIVE_SV_DEVICE})")
    except Exception as e:
        logger.warning(f"声纹提取模型加载失败（/diarize/detail 接口将不可用）: {e}")
        sv_pipeline = None
        ACTIVE_SV_DEVICE = "unavailable"


@app.on_event("startup")
async def startup_event():
    """服务启动时加载模型"""
    os.makedirs(TEMP_DIR, exist_ok=True)
    try:
        load_models()
    except Exception as e:
        logger.error(f"模型加载失败: {e}")
        logger.error("服务将以降级模式运行，请检查模型文件是否存在")


# ──────────────────────────────────────────────
# API 接口
# ──────────────────────────────────────────────
@app.get("/health", response_model=HealthResponse)
async def health_check():
    """健康检查接口"""
    return HealthResponse(
        status="ok" if diarization_pipeline is not None else "degraded",
        model_loaded=diarization_pipeline is not None,
        device=ACTIVE_DIARIZATION_DEVICE,
        diarization_model=DIARIZATION_MODEL,
    )


@app.post("/diarize", response_model=DiarizeResponse | list[LegacySpeakerSegment])
async def diarize(
    request: Request,
    file: Optional[UploadFile] = File(None, description="音频文件（WAV/MP3/M4A/AAC/FLAC）"),
    num_speakers: Optional[int] = Query(
        None,
        description="预设说话人数量（可选，不填则自动检测）",
        ge=2,
        le=20,
    ),
):
    """
    说话人分离接口

    上传音频文件，返回按说话人分段的时间信息。

    - 支持格式：WAV、MP3、M4A、AAC、FLAC、OGG
    - 采样率会自动转为 16kHz
    - 返回每个说话人的发言时间段
    """
    if diarization_pipeline is None:
        raise HTTPException(status_code=503, detail="模型未加载，服务不可用")

    content_type = (request.headers.get("content-type") or "").lower()
    legacy_json_mode = "application/json" in content_type

    task_id = str(uuid.uuid4())[:8]
    start_time = time.time()
    work_dir = os.path.join(TEMP_DIR, task_id)
    os.makedirs(work_dir, exist_ok=True)

    try:
        if legacy_json_mode:
            payload = AudioURLRequest.model_validate(await request.json())
            if payload.num_speakers is not None:
                num_speakers = payload.num_speakers
            ext = _guess_extension_from_url(payload.audio_url)
            raw_path = os.path.join(work_dir, f"raw{ext}")
            _download_audio_from_url(payload.audio_url, raw_path)
            source_label = payload.audio_url
        else:
            if file is None:
                raise HTTPException(status_code=400, detail="缺少音频文件")
            ext = _validate_uploaded_audio(file)
            raw_path = os.path.join(work_dir, f"raw{ext}")
            with open(raw_path, "wb") as f:
                content = await file.read()
                f.write(content)
            source_label = file.filename or raw_path

        # 转为 16kHz WAV
        wav_path = os.path.join(work_dir, "audio_16k.wav")
        audio_duration = convert_to_wav_16k(raw_path, wav_path)

        logger.info(
            f"[{task_id}] 开始处理: {source_label}, "
            f"时长: {audio_duration:.1f}s, "
            f"预设说话人: {num_speakers or '自动检测'}"
        )

        # 调用说话人分离模型
        segments = _run_diarization(wav_path)

        # 统计说话人数量
        speakers = set(seg.speaker for seg in segments)

        process_time = time.time() - start_time
        logger.info(
            f"[{task_id}] 处理完成: "
            f"检测到 {len(speakers)} 位说话人, "
            f"{len(segments)} 个分段, "
            f"耗时 {process_time:.2f}s"
        )

        if legacy_json_mode:
            return _legacy_segments_response(segments)

        return DiarizeResponse(
            task_id=task_id,
            audio_duration=round(audio_duration, 2),
            num_speakers=len(speakers),
            segments=segments,
            process_time=round(process_time, 3),
        )

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[{task_id}] 处理失败: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"处理失败: {str(e)}")
    finally:
        # 清理临时文件
        shutil.rmtree(work_dir, ignore_errors=True)


@app.post("/diarize/detail")
async def diarize_detail(
    file: UploadFile = File(..., description="音频文件"),
):
    """
    详细说话人分离接口（含声纹嵌入向量）

    除了基础分段信息外，还返回每位说话人的声纹嵌入向量（192维），
    可用于后续跨音频的说话人匹配。
    """
    if diarization_pipeline is None:
        raise HTTPException(status_code=503, detail="模型未加载")
    if sv_pipeline is None:
        raise HTTPException(
            status_code=503,
            detail="声纹提取模型未加载，/diarize/detail 不可用",
        )

    task_id = str(uuid.uuid4())[:8]
    start_time = time.time()
    work_dir = os.path.join(TEMP_DIR, task_id)
    os.makedirs(work_dir, exist_ok=True)

    try:
        # 保存并转码
        ext = Path(file.filename or "unknown.wav").suffix.lower()
        raw_path = os.path.join(work_dir, f"raw{ext}")
        with open(raw_path, "wb") as f:
            f.write(await file.read())

        wav_path = os.path.join(work_dir, "audio_16k.wav")
        audio_duration = convert_to_wav_16k(raw_path, wav_path)

        # 说话人分离
        result = diarization_pipeline(wav_path)
        segments = _parse_diarization_result(result)
        speakers = sorted(set(seg.speaker for seg in segments))

        # 为每位说话人提取声纹嵌入
        # 取每位说话人最长的一段语音作为代表
        audio_data, sr = sf.read(wav_path)
        speaker_embeddings = {}

        for spk in speakers:
            spk_segs = [s for s in segments if s.speaker == spk]
            # 找最长的片段
            longest = max(spk_segs, key=lambda s: s.duration)
            start_sample = int(longest.start * sr)
            end_sample = int(longest.end * sr)
            spk_audio = audio_data[start_sample:end_sample]

            # 保存片段
            spk_wav_path = os.path.join(work_dir, f"{spk}.wav")
            sf.write(spk_wav_path, spk_audio, sr)

            # 提取嵌入（使用两个相同文件做 verification 来获取 embedding）
            try:
                sv_result = sv_pipeline([spk_wav_path, spk_wav_path])
                # 不同版本的 pipeline 返回结构可能不同，尝试提取
                if isinstance(sv_result, dict) and "embedding" in sv_result:
                    embedding = sv_result["embedding"]
                else:
                    embedding = None
            except Exception:
                embedding = None

            speaker_embeddings[spk] = {
                "speaker": spk,
                "total_duration": round(
                    sum(s.duration for s in spk_segs), 2
                ),
                "num_segments": len(spk_segs),
                "embedding": (
                    embedding.tolist()
                    if embedding is not None
                       and hasattr(embedding, "tolist")
                    else None
                ),
            }

        process_time = time.time() - start_time

        return {
            "task_id": task_id,
            "audio_duration": round(audio_duration, 2),
            "num_speakers": len(speakers),
            "segments": [s.model_dump() for s in segments],
            "speakers": speaker_embeddings,
            "process_time": round(process_time, 3),
        }

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"[{task_id}] 详细处理失败: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=f"处理失败: {str(e)}")
    finally:
        shutil.rmtree(work_dir, ignore_errors=True)


# ──────────────────────────────────────────────
# 结果解析
# ──────────────────────────────────────────────
def _parse_diarization_result(result) -> list[SpeakerSegment]:
    """
    解析 ModelScope 说话人分离 pipeline 的输出。

    输出格式因模型版本可能略有差异，这里做兼容处理。
    典型格式:
      - result['text']: "0.32 3.15 spk_0\n3.52 8.10 spk_1\n..."
      - 或 result 本身是 dict/list 结构
    """
    segments: list[SpeakerSegment] = []

    def _to_float(value: Any) -> Optional[float]:
        try:
            if value is None:
                return None
            return float(value)
        except (TypeError, ValueError):
            return None

    def _build_segment(speaker: Any, start: Any, end: Any) -> Optional[SpeakerSegment]:
        start_value = _to_float(start)
        end_value = _to_float(end)
        if start_value is None or end_value is None:
            return None
        if end_value < start_value:
            start_value, end_value = end_value, start_value
        speaker_value = str(speaker or "unknown").strip() or "unknown"
        return SpeakerSegment(
            speaker=speaker_value,
            start=round(start_value, 3),
            end=round(end_value, 3),
            duration=round(end_value - start_value, 3),
        )

    def _collect_from_line(text_line: str):
        parts = text_line.split()
        if len(parts) < 3:
            return
        segment = _build_segment(parts[2], parts[0], parts[1])
        if segment is not None:
            segments.append(segment)

    def _collect(payload: Any):
        if payload is None:
            return

        if isinstance(payload, str):
            for line in payload.strip().splitlines():
                line = line.strip()
                if line:
                    _collect_from_line(line)
            return

        if isinstance(payload, dict):
            segment = _build_segment(
                payload.get("speaker") or payload.get("spk") or payload.get("label") or payload.get("name"),
                payload.get("start", payload.get("start_time", payload.get("begin", payload.get("begin_time")))),
                payload.get("end", payload.get("end_time", payload.get("stop", payload.get("finish_time")))),
            )
            if segment is not None:
                segments.append(segment)
                return

            for key in ("segments", "text", "value", "output", "result", "results", "prediction", "predictions"):
                if key in payload:
                    _collect(payload[key])
            return

        if isinstance(payload, (list, tuple)):
            if len(payload) >= 3:
                first, second, third = payload[0], payload[1], payload[2]
                segment = None
                if _to_float(first) is not None and _to_float(second) is not None:
                    segment = _build_segment(third, first, second)
                elif _to_float(second) is not None and _to_float(third) is not None:
                    segment = _build_segment(first, second, third)
                if segment is not None:
                    segments.append(segment)
                    return

            for item in payload:
                _collect(item)

    _collect(result)

    if not segments:
        logger.warning("未能解析说话人分离结果", extra={"result_type": type(result).__name__})

    # 按时间排序
    segments.sort(key=lambda s: s.start)
    return segments


# ──────────────────────────────────────────────
# 入口
# ──────────────────────────────────────────────
if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("PORT", "8080"))
    host = os.getenv("HOST", "0.0.0.0")
    workers = int(os.getenv("WORKERS", "1"))

    logger.info(f"启动说话人分离服务: {host}:{port}")
    uvicorn.run(
        "app:app",
        host=host,
        port=port,
        workers=workers,
        log_level="info",
    )
