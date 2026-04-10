"""
Pydantic 数据模型 —— API 请求/响应 + 内部数据结构
"""

from __future__ import annotations

import uuid
from datetime import datetime
from typing import Optional

from pydantic import BaseModel, Field


# =============================================================================
# 说话人分离相关
# =============================================================================

class SpeakerSegment(BaseModel):
    """单个说话人片段"""
    speaker_id: str = Field(description="说话人标签（匿名: speaker_0, 或真实姓名）")
    start_time: float = Field(description="起始时间（秒）")
    end_time: float = Field(description="结束时间（秒）")
    duration: float = Field(description="持续时长（秒）")
    confidence: Optional[float] = Field(None, description="匹配置信度（仅声纹匹配时）")


class DiarizeRequest(BaseModel):
    """说话人分离请求参数"""
    min_speakers: Optional[int] = Field(None, ge=1, le=20, description="最少说话人数")
    max_speakers: Optional[int] = Field(None, ge=1, le=20, description="最多说话人数")
    clustering_method: Optional[str] = Field(None, description="聚类算法: spectral / umap_hdbscan")
    enable_voiceprint_match: bool = Field(False, description="是否启用声纹匹配（需先注册声纹）")


class DiarizeResponse(BaseModel):
    """说话人分离响应"""
    task_id: str = Field(description="任务 ID")
    audio_duration: float = Field(description="音频总时长（秒）")
    num_speakers: int = Field(description="检测到的说话人数量")
    segments: list[SpeakerSegment] = Field(description="说话人片段列表")
    rttm: str = Field(description="RTTM 格式输出")
    speaker_summary: list[SpeakerSummary] = Field(description="各说话人统计")
    processing_time: float = Field(description="处理耗时（秒）")


class SpeakerSummary(BaseModel):
    """单个说话人统计"""
    speaker_id: str
    total_duration: float = Field(description="总发言时长（秒）")
    segment_count: int = Field(description="发言片段数")
    percentage: float = Field(description="占比（%）")
    voiceprint_matched: bool = Field(False, description="是否匹配到已注册声纹")
    match_confidence: Optional[float] = Field(None, description="匹配置信度")


# 解决循环引用
DiarizeResponse.model_rebuild()


class VADSegment(BaseModel):
    """单个语音活动片段"""
    start_time: float = Field(description="起始时间（秒）")
    end_time: float = Field(description="结束时间（秒）")
    duration: float = Field(description="持续时长（秒）")


class VADResponse(BaseModel):
    """语音活动检测响应"""
    task_id: str = Field(description="任务 ID")
    audio_duration: float = Field(description="音频总时长（秒）")
    speech_duration: float = Field(description="检测到的人声总时长（秒）")
    speech_ratio: float = Field(description="人声占比（%）")
    num_segments: int = Field(description="人声片段数量")
    segments: list[VADSegment] = Field(description="人声片段列表")
    detector_backend: str = Field(description="实际使用的检测后端")
    processing_time: float = Field(description="处理耗时（秒）")


# =============================================================================
# 声纹注册相关
# =============================================================================

class VoiceprintEnrollRequest(BaseModel):
    """声纹注册请求"""
    speaker_name: str = Field(min_length=1, max_length=100, description="说话人姓名")
    department: Optional[str] = Field(None, max_length=100, description="所属部门")
    notes: Optional[str] = Field(None, max_length=500, description="备注信息")


class VoiceprintRecord(BaseModel):
    """声纹记录"""
    id: str = Field(default_factory=lambda: str(uuid.uuid4()), description="唯一标识")
    speaker_name: str = Field(description="说话人姓名")
    department: Optional[str] = Field(None, description="所属部门")
    notes: Optional[str] = Field(None, description="备注")
    embedding_path: str = Field(description="嵌入向量文件路径")
    audio_path: Optional[str] = Field(None, description="注册音频备份路径")
    audio_duration: float = Field(description="注册音频时长（秒）")
    created_at: datetime = Field(default_factory=datetime.now)
    updated_at: datetime = Field(default_factory=datetime.now)


class VoiceprintListResponse(BaseModel):
    """声纹列表响应"""
    total: int
    records: list[VoiceprintRecord]


class VoiceprintMatchResult(BaseModel):
    """单个说话人的声纹匹配结果"""
    anonymous_id: str = Field(description="原始匿名标签（如 speaker_0）")
    matched_name: Optional[str] = Field(None, description="匹配到的姓名（None 表示未匹配）")
    matched_id: Optional[str] = Field(None, description="匹配到的声纹记录 ID")
    confidence: float = Field(description="最高匹配置信度")
    is_confident: bool = Field(description="是否达到置信阈值")


# =============================================================================
# 通用
# =============================================================================

class HealthResponse(BaseModel):
    """健康检查响应"""
    status: str = "ok"
    service_name: str
    version: str
    models_loaded: dict[str, bool]
    voiceprint_count: int
    device: str


class ErrorResponse(BaseModel):
    """错误响应"""
    error: str
    detail: Optional[str] = None
