"""
FastAPI 服务入口 —— 说话人分离 + 声纹注册匹配 RESTful API
"""

from __future__ import annotations

import os
import tempfile
from contextlib import asynccontextmanager
from pathlib import Path

import yaml
from fastapi import FastAPI, File, Form, HTTPException, UploadFile
from fastapi.middleware.cors import CORSMiddleware
from loguru import logger

from src.embedding import EmbeddingExtractor
from src.engine import DiarizationEngine
from src.matcher import SpeakerMatcher
from src.models import (
    DiarizeResponse,
    ErrorResponse,
    HealthResponse,
    VADResponse,
    VoiceprintEnrollRequest,
    VoiceprintListResponse,
    VoiceprintRecord,
)
from src.vad import VoiceActivityDetector
from src.voiceprint import VoiceprintManager

# ─── 全局实例 ───
_config: dict = {}
_extractor: EmbeddingExtractor | None = None
_voiceprint_mgr: VoiceprintManager | None = None
_matcher: SpeakerMatcher | None = None
_engine: DiarizationEngine | None = None
_vad: VoiceActivityDetector | None = None

SERVICE_NAME = "speaker-analysis-service"
VERSION = "1.1.0"


def load_config(config_path: str = "config/settings.yaml") -> dict:
    """加载配置文件"""
    if os.path.exists(config_path):
        with open(config_path, "r", encoding="utf-8") as f:
            config = yaml.safe_load(f) or {}
            return apply_env_overrides(config)
    logger.warning(f"配置文件未找到: {config_path}, 使用默认配置")
    return apply_env_overrides({})


def apply_env_overrides(config: dict) -> dict:
    server_cfg = config.setdefault("server", {})
    server_cfg["host"] = os.getenv("HOST", server_cfg.get("host", "0.0.0.0"))
    server_cfg["port"] = int(os.getenv("PORT", server_cfg.get("port", 8100)))
    server_cfg["workers"] = int(os.getenv("WORKERS", server_cfg.get("workers", 1)))

    models_cfg = config.setdefault("models", {})
    embedding_cfg = models_cfg.setdefault("embedding", {})
    vad_cfg = models_cfg.setdefault("vad", {})

    default_device = os.getenv("DEVICE")
    embedding_device = os.getenv("EMBEDDING_DEVICE", default_device or embedding_cfg.get("device", "cpu"))
    vad_device = os.getenv("VAD_DEVICE", default_device or vad_cfg.get("device", "cpu"))
    embedding_cfg["device"] = embedding_device
    vad_cfg["device"] = vad_device

    return config


@asynccontextmanager
async def lifespan(app: FastAPI):
    """应用生命周期管理"""
    global _config, _extractor, _voiceprint_mgr, _matcher, _engine, _vad

    logger.info("正在初始化服务...")
    _config = load_config()

    # 初始化嵌入提取器
    model_cfg = _config.get("models", {}).get("embedding", {})
    _extractor = EmbeddingExtractor(
        model_id=model_cfg.get("model_id", "iic/speech_eres2netv2_sv_zh-cn_16k-common"),
        local_dir=model_cfg.get("local_dir", "./models/eres2netv2"),
        device=model_cfg.get("device", "cpu"),
        embedding_dim=model_cfg.get("embedding_dim", 192),
    )

    # 初始化声纹管理器
    vp_cfg = _config.get("voiceprint", {})
    _voiceprint_mgr = VoiceprintManager(
        extractor=_extractor,
        db_path=vp_cfg.get("db_path", "./data/voiceprint.db"),
        embeddings_dir=vp_cfg.get("embeddings_dir", "./data/voiceprint_embeddings"),
        enrollment_audio_dir=vp_cfg.get("enrollment_audio_dir", "./data/enrollment_audio"),
        min_enrollment_duration=vp_cfg.get("min_enrollment_duration", 5.0),
    )

    # 初始化匹配器
    _matcher = SpeakerMatcher(
        extractor=_extractor,
        voiceprint_mgr=_voiceprint_mgr,
        match_threshold=vp_cfg.get("match_threshold", 0.68),
        high_confidence_threshold=vp_cfg.get("high_confidence_threshold", 0.82),
    )

    diar_cfg = _config.get("diarization", {})
    vad_cfg = _config.get("models", {}).get("vad", {})
    _vad = VoiceActivityDetector(
        model_id=vad_cfg.get("model_id", "iic/speech_fsmn_vad_zh-cn-16k-common-pytorch"),
        local_dir=vad_cfg.get("local_dir", "./models/fsmn_vad"),
        device=vad_cfg.get("device", "cpu"),
        target_sr=diar_cfg.get("target_sample_rate", 16000),
        min_speech_duration=vad_cfg.get("min_speech_duration", 0.2),
        min_silence_duration=vad_cfg.get("min_silence_duration", 0.15),
        speech_pad_duration=vad_cfg.get("speech_pad_duration", 0.1),
    )

    # 初始化分离引擎
    _engine = DiarizationEngine(
        extractor=_extractor,
        vad=_vad,
        voiceprint_mgr=_voiceprint_mgr,
        matcher=_matcher,
        clustering_method=diar_cfg.get("clustering_method", "spectral"),
        target_sr=diar_cfg.get("target_sample_rate", 16000),
        segment_duration=diar_cfg.get("segment_duration", 1.5),
        segment_step=diar_cfg.get("segment_step", 0.75),
    )

    logger.info(f"服务初始化完成 (声纹库: {_voiceprint_mgr.count} 条记录)")
    yield
    logger.info("服务关闭")


# ─── 创建 FastAPI 应用 ───

app = FastAPI(
    title="Speaker Analysis Service",
    description="基于 3D-Speaker / FSMN-VAD 的说话人分离、声纹管理与语音活动检测 API，面向局域网私有化部署",
    version=VERSION,
    lifespan=lifespan,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


# ─── 辅助函数 ───

async def save_upload_to_temp(file: UploadFile) -> str:
    """将上传文件保存到临时路径"""
    suffix = Path(file.filename or "audio.wav").suffix or ".wav"
    tmp = tempfile.NamedTemporaryFile(suffix=suffix, delete=False)
    content = await file.read()
    tmp.write(content)
    tmp.flush()
    tmp.close()
    return tmp.name


# =============================================================================
# API 端点
# =============================================================================

# ─── 健康检查 ───

@app.get("/api/v1/health", response_model=HealthResponse, tags=["系统"])
async def health():
    """服务健康检查"""
    return HealthResponse(
        service_name=SERVICE_NAME,
        version=VERSION,
        models_loaded={
            "embedding": _extractor is not None and _extractor._loaded,
            "vad": _vad is not None and _vad.is_loaded,
            "engine": _engine is not None,
        },
        voiceprint_count=_voiceprint_mgr.count if _voiceprint_mgr else 0,
        device=_extractor.device if _extractor else "unknown",
    )


@app.post(
    "/api/v1/vad",
    response_model=VADResponse,
    tags=["语音活动检测"],
    summary="语音活动检测",
)
async def detect_voice_activity(
    file: UploadFile = File(..., description="音频文件 (WAV/MP3/FLAC/M4A)"),
):
    """上传音频文件，返回语音活动片段，可作为实时 ASR 或切段依据。"""
    if _vad is None:
        raise HTTPException(status_code=503, detail="VAD 模型未初始化")

    tmp_path = await save_upload_to_temp(file)
    try:
        return _vad.detect(tmp_path)
    except Exception as exc:
        logger.exception("VAD 检测失败")
        raise HTTPException(status_code=500, detail=str(exc))
    finally:
        os.unlink(tmp_path)


# ─── 说话人分离 ───

@app.post(
    "/api/v1/diarize",
    response_model=DiarizeResponse,
    tags=["说话人分离"],
    summary="说话人分离（匿名标签）",
)
async def diarize(
    file: UploadFile = File(..., description="音频文件 (WAV/MP3/FLAC/M4A)"),
    min_speakers: int | None = Form(None, description="最少说话人数"),
    max_speakers: int | None = Form(None, description="最多说话人数"),
    clustering_method: str | None = Form(None, description="聚类算法: spectral / umap_hdbscan"),
):
    """
    上传音频文件，执行说话人分离，返回匿名标签（speaker_0, speaker_1, ...）。
    """
    tmp_path = await save_upload_to_temp(file)
    try:
        result = _engine.diarize(
            audio_path=tmp_path,
            min_speakers=min_speakers,
            max_speakers=max_speakers,
            clustering_method=clustering_method,
            enable_voiceprint_match=False,
        )
        return result
    except Exception as e:
        logger.exception("分离失败")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        os.unlink(tmp_path)


@app.post(
    "/api/v1/diarize-identify",
    response_model=DiarizeResponse,
    tags=["说话人分离"],
    summary="说话人分离 + 声纹匹配（真实姓名）",
)
async def diarize_with_identification(
    file: UploadFile = File(..., description="音频文件"),
    min_speakers: int | None = Form(None),
    max_speakers: int | None = Form(None),
    clustering_method: str | None = Form(None),
):
    """
    上传音频文件，执行说话人分离，并与声纹库匹配，尝试将匿名标签替换为真实姓名。
    未匹配到的说话人保持匿名标签。
    """
    if _voiceprint_mgr.count == 0:
        raise HTTPException(
            status_code=400,
            detail="声纹库为空，请先通过 /api/v1/voiceprint/enroll 注册声纹",
        )

    tmp_path = await save_upload_to_temp(file)
    try:
        result = _engine.diarize(
            audio_path=tmp_path,
            min_speakers=min_speakers,
            max_speakers=max_speakers,
            clustering_method=clustering_method,
            enable_voiceprint_match=True,
        )
        return result
    except Exception as e:
        logger.exception("分离+匹配失败")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        os.unlink(tmp_path)


# ─── 声纹管理 ───

@app.post(
    "/api/v1/voiceprint/enroll",
    response_model=VoiceprintRecord,
    tags=["声纹管理"],
    summary="注册声纹",
)
async def enroll_voiceprint(
    file: UploadFile = File(..., description="注册音频文件（建议 15~30 秒清晰语音）"),
    speaker_name: str = Form(..., description="说话人姓名"),
    department: str | None = Form(None, description="所属部门"),
    notes: str | None = Form(None, description="备注"),
):
    """
    为一个说话人注册声纹。上传一段该说话人的清晰语音（建议 15~30 秒），
    系统提取声纹嵌入并存入声纹库。后续分离时可自动匹配到该说话人。
    """
    tmp_path = await save_upload_to_temp(file)
    try:
        record = _voiceprint_mgr.enroll(
            speaker_name=speaker_name,
            audio_path=tmp_path,
            department=department,
            notes=notes,
        )
        return record
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.exception("声纹注册失败")
        raise HTTPException(status_code=500, detail=str(e))
    finally:
        os.unlink(tmp_path)


@app.get(
    "/api/v1/voiceprint/list",
    response_model=VoiceprintListResponse,
    tags=["声纹管理"],
    summary="列出所有声纹",
)
async def list_voiceprints():
    """列出声纹库中所有已注册的声纹记录。"""
    records = _voiceprint_mgr.list_all()
    return VoiceprintListResponse(total=len(records), records=records)


@app.get(
    "/api/v1/voiceprint/{record_id}",
    response_model=VoiceprintRecord,
    tags=["声纹管理"],
    summary="查询声纹详情",
)
async def get_voiceprint(record_id: str):
    """按 ID 查询声纹记录详情。"""
    record = _voiceprint_mgr.get(record_id)
    if not record:
        raise HTTPException(status_code=404, detail="声纹记录不存在")
    return record


@app.delete(
    "/api/v1/voiceprint/{record_id}",
    tags=["声纹管理"],
    summary="删除声纹",
)
async def delete_voiceprint(record_id: str):
    """按 ID 删除声纹记录（同时删除嵌入向量和备份音频）。"""
    deleted = _voiceprint_mgr.delete(record_id)
    if not deleted:
        raise HTTPException(status_code=404, detail="声纹记录不存在")
    return {"status": "ok", "deleted_id": record_id}


# ─── 启动入口 ───

if __name__ == "__main__":
    import uvicorn

    server_cfg = _config.get("server", {}) if _config else {}
    uvicorn.run(
        "src.server:app",
        host=server_cfg.get("host", "0.0.0.0"),
        port=server_cfg.get("port", 8100),
        workers=server_cfg.get("workers", 1),
        reload=False,
    )
