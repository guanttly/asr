"""
声纹库管理 —— 注册、存储、删除、持久化
"""

from __future__ import annotations

import json
import os
import shutil
from datetime import datetime
from pathlib import Path
from typing import Optional

import numpy as np
from loguru import logger

from src.embedding import EmbeddingExtractor
from src.models import VoiceprintRecord
from src.utils import get_audio_duration


class VoiceprintManager:
    """
    声纹库管理器。

    职责:
      - 注册: 接收音频 → 提取嵌入 → 存储到声纹库
      - 查询: 列出所有已注册声纹
      - 删除: 按 ID 删除声纹记录
      - 持久化: 嵌入向量以 .npy 存储, 元数据以 JSON 存储（轻量级，无需数据库依赖）

    在生产环境中，元数据可替换为 SQLite / PostgreSQL。
    """

    def __init__(
        self,
        extractor: EmbeddingExtractor,
        db_path: str = "./data/voiceprint.db",
        embeddings_dir: str = "./data/voiceprint_embeddings",
        enrollment_audio_dir: str = "./data/enrollment_audio",
        min_enrollment_duration: float = 5.0,
    ):
        self.extractor = extractor
        self.db_path = db_path
        self.embeddings_dir = embeddings_dir
        self.enrollment_audio_dir = enrollment_audio_dir
        self.min_enrollment_duration = min_enrollment_duration

        # 内存中的声纹记录 {id: VoiceprintRecord}
        self._records: dict[str, VoiceprintRecord] = {}
        # 内存中的嵌入缓存 {id: np.ndarray}
        self._embeddings: dict[str, np.ndarray] = {}

        self._ensure_dirs()
        self._load_from_disk()

    def _ensure_dirs(self) -> None:
        os.makedirs(self.embeddings_dir, exist_ok=True)
        os.makedirs(self.enrollment_audio_dir, exist_ok=True)
        os.makedirs(os.path.dirname(self.db_path) or ".", exist_ok=True)

    # ─── 注册 ───

    def enroll(
        self,
        speaker_name: str,
        audio_path: str,
        department: Optional[str] = None,
        notes: Optional[str] = None,
        keep_audio: bool = True,
    ) -> VoiceprintRecord:
        """
        注册一个说话人的声纹。

        Args:
            speaker_name: 说话人姓名
            audio_path: 注册音频文件路径
            department: 所属部门
            notes: 备注
            keep_audio: 是否备份注册音频

        Returns:
            VoiceprintRecord

        Raises:
            ValueError: 音频时长不足
        """
        # 校验音频时长
        duration = get_audio_duration(audio_path)
        if duration < self.min_enrollment_duration:
            raise ValueError(
                f"注册音频时长不足: {duration:.1f}s < {self.min_enrollment_duration}s 最低要求。"
                f"建议录制 15~30 秒清晰语音。"
            )

        logger.info(f"开始声纹注册: {speaker_name} (音频时长: {duration:.1f}s)")

        # 提取嵌入
        embedding = self.extractor.extract(audio_path)

        # 创建记录
        record = VoiceprintRecord(
            speaker_name=speaker_name,
            department=department,
            notes=notes,
            embedding_path="",  # 后面填充
            audio_duration=duration,
        )

        # 保存嵌入向量
        emb_filename = f"{record.id}.npy"
        emb_path = os.path.join(self.embeddings_dir, emb_filename)
        np.save(emb_path, embedding)
        record.embedding_path = emb_path

        # 备份注册音频
        if keep_audio:
            ext = Path(audio_path).suffix
            audio_backup = os.path.join(self.enrollment_audio_dir, f"{record.id}{ext}")
            shutil.copy2(audio_path, audio_backup)
            record.audio_path = audio_backup

        # 加入内存
        self._records[record.id] = record
        self._embeddings[record.id] = embedding

        # 持久化元数据
        self._save_metadata()

        logger.info(f"声纹注册完成: {speaker_name} (id={record.id})")
        return record

    def enroll_from_embedding(
        self,
        speaker_name: str,
        embedding: np.ndarray,
        audio_duration: float = 0.0,
        department: Optional[str] = None,
        notes: Optional[str] = None,
    ) -> VoiceprintRecord:
        """
        直接从嵌入向量注册（用于从会议分离结果中追加注册）。
        """
        # 归一化
        norm = np.linalg.norm(embedding)
        if norm > 0:
            embedding = embedding / norm

        record = VoiceprintRecord(
            speaker_name=speaker_name,
            department=department,
            notes=notes,
            embedding_path="",
            audio_duration=audio_duration,
        )

        emb_path = os.path.join(self.embeddings_dir, f"{record.id}.npy")
        np.save(emb_path, embedding)
        record.embedding_path = emb_path

        self._records[record.id] = record
        self._embeddings[record.id] = embedding
        self._save_metadata()

        logger.info(f"声纹注册完成（从嵌入）: {speaker_name} (id={record.id})")
        return record

    # ─── 查询 ───

    def list_all(self) -> list[VoiceprintRecord]:
        """列出所有已注册声纹"""
        return list(self._records.values())

    def get(self, record_id: str) -> Optional[VoiceprintRecord]:
        """按 ID 获取声纹记录"""
        return self._records.get(record_id)

    def get_by_name(self, speaker_name: str) -> list[VoiceprintRecord]:
        """按姓名查找（可能有同名）"""
        return [r for r in self._records.values() if r.speaker_name == speaker_name]

    def get_embedding(self, record_id: str) -> Optional[np.ndarray]:
        """获取指定记录的嵌入向量"""
        return self._embeddings.get(record_id)

    def get_all_embeddings(self) -> dict[str, tuple[str, np.ndarray]]:
        """
        获取所有已注册的嵌入向量。

        Returns:
            {record_id: (speaker_name, embedding)}
        """
        result = {}
        for rid, record in self._records.items():
            emb = self._embeddings.get(rid)
            if emb is not None:
                result[rid] = (record.speaker_name, emb)
        return result

    @property
    def count(self) -> int:
        return len(self._records)

    # ─── 删除 ───

    def delete(self, record_id: str) -> bool:
        """删除声纹记录"""
        record = self._records.pop(record_id, None)
        if record is None:
            return False

        self._embeddings.pop(record_id, None)

        # 删除文件
        if record.embedding_path and os.path.exists(record.embedding_path):
            os.remove(record.embedding_path)
        if record.audio_path and os.path.exists(record.audio_path):
            os.remove(record.audio_path)

        self._save_metadata()
        logger.info(f"声纹已删除: {record.speaker_name} (id={record_id})")
        return True

    def delete_by_name(self, speaker_name: str) -> int:
        """按姓名删除所有匹配记录，返回删除数量"""
        ids_to_delete = [
            rid for rid, r in self._records.items()
            if r.speaker_name == speaker_name
        ]
        for rid in ids_to_delete:
            self.delete(rid)
        return len(ids_to_delete)

    # ─── 持久化 ───

    def _save_metadata(self) -> None:
        """将元数据保存到 JSON 文件"""
        data = {
            rid: record.model_dump(mode="json")
            for rid, record in self._records.items()
        }
        with open(self.db_path, "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2, default=str)

    def _load_from_disk(self) -> None:
        """从磁盘加载元数据和嵌入向量"""
        if not os.path.exists(self.db_path):
            logger.info("声纹库为空，跳过加载")
            return

        try:
            with open(self.db_path, "r", encoding="utf-8") as f:
                data = json.load(f)
        except (json.JSONDecodeError, IOError) as e:
            logger.warning(f"声纹库元数据加载失败: {e}")
            return

        loaded = 0
        for rid, record_data in data.items():
            try:
                record = VoiceprintRecord(**record_data)
                self._records[rid] = record

                # 加载嵌入向量
                if record.embedding_path and os.path.exists(record.embedding_path):
                    emb = np.load(record.embedding_path)
                    self._embeddings[rid] = emb
                    loaded += 1
                else:
                    logger.warning(f"嵌入文件缺失: {record.embedding_path}")
            except Exception as e:
                logger.warning(f"加载声纹记录失败 (id={rid}): {e}")

        logger.info(f"声纹库加载完成: {loaded}/{len(data)} 条记录")
