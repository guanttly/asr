"""
说话人分离引擎 —— 封装 3D-Speaker 的完整分离流水线
"""

from __future__ import annotations

import os
import time
import uuid
from typing import TYPE_CHECKING, Optional

import numpy as np
from loguru import logger

from src.embedding import EmbeddingExtractor
from src.matcher import SpeakerMatcher
from src.models import DiarizeResponse, SpeakerSegment, SpeakerSummary
from src.utils import (
    concat_segments,
    load_audio,
    segments_to_rttm,
)
from src.voiceprint import VoiceprintManager

if TYPE_CHECKING:
    from src.vad import VoiceActivityDetector


class DiarizationEngine:
    """
    说话人分离引擎。

    完整流水线:
      1. 音频加载 + 预处理（重采样、单声道）
    2. VAD 检测有效语音段
      3. 语音分段 + 嵌入提取
      4. 聚类（谱聚类 / UMAP-HDBSCAN）
      5. [可选] 声纹匹配 → 匿名标签替换为真实姓名
      6. 输出 RTTM + 结构化结果

    支持两种运行模式:
      - 3D-Speaker 原生流水线（推荐，需安装 speakerlab）
      - 纯 Python 实现的简化流水线（兼容模式，无需 speakerlab）
    """

    def __init__(
        self,
        extractor: EmbeddingExtractor,
        vad: Optional["VoiceActivityDetector"] = None,
        voiceprint_mgr: Optional[VoiceprintManager] = None,
        matcher: Optional[SpeakerMatcher] = None,
        clustering_method: str = "spectral",
        target_sr: int = 16000,
        segment_duration: float = 1.5,
        segment_step: float = 0.75,
    ):
        self.extractor = extractor
        self.vad = vad
        self.voiceprint_mgr = voiceprint_mgr
        self.matcher = matcher
        self.clustering_method = clustering_method
        self.target_sr = target_sr
        self.segment_duration = segment_duration
        self.segment_step = segment_step

        self._native_available = self._check_native()

    def _check_native(self) -> bool:
        """检查 3D-Speaker 原生分离流水线是否可用"""
        try:
            from src.speakerlab_entry import can_run_native_pipeline

            if can_run_native_pipeline():
                return True
        except Exception:
            pass

            logger.info("speakerlab 原生流水线不可用，将使用兼容模式")
            return False

    def diarize(
        self,
        audio_path: str,
        min_speakers: Optional[int] = None,
        max_speakers: Optional[int] = None,
        clustering_method: Optional[str] = None,
        enable_voiceprint_match: bool = False,
    ) -> DiarizeResponse:
        """
        执行说话人分离。

        Args:
            audio_path: 音频文件路径
            min_speakers: 最少说话人数
            max_speakers: 最多说话人数
            clustering_method: 覆盖默认聚类算法
            enable_voiceprint_match: 是否启用声纹匹配

        Returns:
            DiarizeResponse
        """
        task_id = str(uuid.uuid4())[:8]
        t_start = time.time()
        method = clustering_method or self.clustering_method

        logger.info(f"[{task_id}] 开始说话人分离: {audio_path} (聚类={method})")

        # 加载音频
        waveform, sr = load_audio(audio_path, target_sr=self.target_sr, mono=True)
        audio_duration = len(waveform) / sr

        # 执行分离
        if self._native_available:
            raw_segments, cluster_embeddings = self._diarize_native(
                audio_path, min_speakers, max_speakers, method
            )
        else:
            raw_segments, cluster_embeddings = self._diarize_fallback(
                audio_path,
                waveform, sr, min_speakers, max_speakers, method
            )

        num_speakers = len(set(seg["speaker_id"] for seg in raw_segments))

        # 声纹匹配
        identity_map: dict[str, str] = {}
        match_confidences: dict[str, float] = {}

        if enable_voiceprint_match and self.matcher and cluster_embeddings:
            match_results = self.matcher.match(cluster_embeddings)
            for anon_id, result in match_results.items():
                if result.matched_name:
                    identity_map[anon_id] = result.matched_name
                    match_confidences[anon_id] = result.confidence

        # 替换标签
        for seg in raw_segments:
            original_id = seg["speaker_id"]
            if original_id in identity_map:
                seg["speaker_id"] = identity_map[original_id]
                seg["confidence"] = match_confidences.get(original_id)

        # 构建输出
        segments = [
            SpeakerSegment(
                speaker_id=seg["speaker_id"],
                start_time=seg["start_time"],
                end_time=seg["start_time"] + seg["duration"],
                duration=seg["duration"],
                confidence=seg.get("confidence"),
            )
            for seg in raw_segments
        ]

        # 说话人统计
        speaker_stats: dict[str, dict] = {}
        for seg in segments:
            sid = seg.speaker_id
            if sid not in speaker_stats:
                speaker_stats[sid] = {"duration": 0.0, "count": 0}
            speaker_stats[sid]["duration"] += seg.duration
            speaker_stats[sid]["count"] += 1

        summaries = [
            SpeakerSummary(
                speaker_id=sid,
                total_duration=round(stats["duration"], 2),
                segment_count=stats["count"],
                percentage=round(stats["duration"] / audio_duration * 100, 1) if audio_duration > 0 else 0,
                voiceprint_matched=sid in identity_map.values(),
                match_confidence=match_confidences.get(
                    # 反查 anonymous_id
                    next((k for k, v in identity_map.items() if v == sid), ""),
                    None,
                ),
            )
            for sid, stats in speaker_stats.items()
        ]

        rttm = segments_to_rttm(raw_segments, file_id=os.path.basename(audio_path))
        processing_time = round(time.time() - t_start, 3)

        logger.info(
            f"[{task_id}] 分离完成: {num_speakers}人, "
            f"{len(segments)}段, 耗时={processing_time}s"
        )

        return DiarizeResponse(
            task_id=task_id,
            audio_duration=round(audio_duration, 2),
            num_speakers=num_speakers,
            segments=segments,
            rttm=rttm,
            speaker_summary=summaries,
            processing_time=processing_time,
        )

    def _diarize_native(
        self,
        audio_path: str,
        min_speakers: Optional[int],
        max_speakers: Optional[int],
        method: str,
    ) -> tuple[list[dict], dict[str, np.ndarray]]:
        """使用 3D-Speaker 原生流水线"""
        import subprocess
        import tempfile

        out_dir = tempfile.mkdtemp(prefix="diar_")

        cmd = [
            "python", "-m", "src.speakerlab_entry",
            "--wav", audio_path,
            "--out_dir", out_dir,
        ]

        result = subprocess.run(cmd, capture_output=True, text=True, timeout=600)
        if result.returncode != 0:
            logger.error(f"3D-Speaker 分离失败: {result.stderr}")
            raise RuntimeError(f"分离失败: {result.stderr}")

        # 解析 RTTM 输出
        rttm_files = list(
            f for f in os.listdir(out_dir)
            if f.endswith(".rttm")
        )
        if not rttm_files:
            raise RuntimeError(f"未找到 RTTM 输出: {out_dir}")

        raw_segments = self._parse_rttm(os.path.join(out_dir, rttm_files[0]))

        # 提取各聚类的代表性嵌入（用于声纹匹配）
        waveform, sr = load_audio(audio_path, target_sr=self.target_sr)
        cluster_embeddings = self._extract_cluster_embeddings(waveform, sr, raw_segments)

        return raw_segments, cluster_embeddings

    def _diarize_fallback(
        self,
        audio_path: str,
        waveform: np.ndarray,
        sr: int,
        min_speakers: Optional[int],
        max_speakers: Optional[int],
        method: str,
    ) -> tuple[list[dict], dict[str, np.ndarray]]:
        """
        兼容模式：纯 Python 实现的简化分离流水线。
        适用于 speakerlab 未安装的环境。

        步骤:
          1. 优先使用独立 VAD 检测人声段
          2. 固定窗口分段
          3. 嵌入提取
          4. 聚类
        """
        from scipy.cluster.hierarchy import fcluster, linkage
        from src.utils import save_temp_audio

        logger.info("使用兼容模式分离流水线")

        speech_mask = np.zeros(len(waveform), dtype=bool)
        if self.vad is not None:
            try:
                vad_segments = self.vad.detect_segments(audio_path, waveform=waveform, sample_rate=sr)
            except Exception as exc:
                logger.warning(f"调用独立 VAD 失败，改用全量窗口扫描: {exc}")
                vad_segments = []

            for segment in vad_segments:
                start_sample = max(0, int(segment.start_time * sr))
                end_sample = min(len(waveform), int(segment.end_time * sr))
                if end_sample > start_sample:
                    speech_mask[start_sample:end_sample] = True
        else:
            speech_mask[:] = True

        # 分段提取嵌入
        seg_samples = int(self.segment_duration * sr)
        step_samples = int(self.segment_step * sr)
        embeddings_list = []
        segment_times = []

        for start_sample in range(0, len(waveform) - seg_samples, step_samples):
            start_time = start_sample / sr
            end_time = (start_sample + seg_samples) / sr

            # 检查该段是否包含语音
            speech_ratio = np.mean(speech_mask[start_sample:start_sample + seg_samples])
            if speech_ratio < 0.3:
                continue

            seg_waveform = waveform[start_sample:start_sample + seg_samples]
            tmp_path = save_temp_audio(seg_waveform, sr)
            try:
                emb = self.extractor.extract(tmp_path)
                embeddings_list.append(emb)
                segment_times.append((start_time, end_time))
            except Exception as e:
                logger.debug(f"分段嵌入提取失败: {e}")
            finally:
                os.unlink(tmp_path)

        if not embeddings_list:
            return [], {}

        embeddings_array = np.stack(embeddings_list)

        # 聚类
        n_segments = len(embeddings_array)
        n_clusters = max_speakers or min(6, max(2, n_segments // 10))
        if min_speakers:
            n_clusters = max(min_speakers, n_clusters)

        # 层次聚类（使用余弦距离）
        from scipy.spatial.distance import pdist
        dists = pdist(embeddings_array, metric="cosine")
        Z = linkage(dists, method="ward")
        labels = fcluster(Z, t=n_clusters, criterion="maxclust")

        # 构建分离结果
        raw_segments = []
        for i, (start, end) in enumerate(segment_times):
            cluster_id = int(labels[i]) - 1
            raw_segments.append({
                "speaker_id": f"speaker_{cluster_id}",
                "start_time": round(start, 3),
                "duration": round(end - start, 3),
            })

        # 合并相邻同一说话人的片段
        raw_segments = self._merge_adjacent_segments(raw_segments)

        # 提取各聚类代表性嵌入
        cluster_embeddings = {}
        unique_speakers = set(seg["speaker_id"] for seg in raw_segments)
        for spk in unique_speakers:
            spk_indices = [
                i for i, (_, _) in enumerate(segment_times)
                if i < len(labels) and f"speaker_{int(labels[i]) - 1}" == spk
            ]
            if spk_indices:
                spk_embs = embeddings_array[spk_indices]
                cluster_embeddings[spk] = np.mean(spk_embs, axis=0)
                # 归一化
                norm = np.linalg.norm(cluster_embeddings[spk])
                if norm > 0:
                    cluster_embeddings[spk] /= norm

        return raw_segments, cluster_embeddings

    def _extract_cluster_embeddings(
        self,
        waveform: np.ndarray,
        sr: int,
        segments: list[dict],
    ) -> dict[str, np.ndarray]:
        """
        从分离结果中，为每个说话人提取代表性嵌入（取最长片段拼接后提取）。
        """
        from src.utils import concat_segments as concat_segs, save_temp_audio

        speaker_segments: dict[str, list[tuple[float, float]]] = {}
        for seg in segments:
            sid = seg["speaker_id"]
            start = seg["start_time"]
            end = start + seg["duration"]
            speaker_segments.setdefault(sid, []).append((start, end))

        cluster_embeddings = {}
        for sid, segs in speaker_segments.items():
            representative = concat_segs(waveform, sr, segs, max_duration=30.0)
            if len(representative) < sr * 1.0:  # 至少 1 秒
                continue
            tmp_path = save_temp_audio(representative, sr)
            try:
                emb = self.extractor.extract(tmp_path)
                cluster_embeddings[sid] = emb
            except Exception as e:
                logger.warning(f"聚类嵌入提取失败 ({sid}): {e}")
            finally:
                os.unlink(tmp_path)

        return cluster_embeddings

    @staticmethod
    def _parse_rttm(rttm_path: str) -> list[dict]:
        """解析 RTTM 文件"""
        segments = []
        with open(rttm_path, "r") as f:
            for line in f:
                parts = line.strip().split()
                if len(parts) >= 8 and parts[0] == "SPEAKER":
                    segments.append({
                        "speaker_id": parts[7],
                        "start_time": float(parts[3]),
                        "duration": float(parts[4]),
                    })
        return segments

    @staticmethod
    def _merge_adjacent_segments(
        segments: list[dict],
        gap_threshold: float = 0.3,
    ) -> list[dict]:
        """合并相邻的同一说话人片段"""
        if not segments:
            return []

        merged = [segments[0].copy()]
        for seg in segments[1:]:
            prev = merged[-1]
            prev_end = prev["start_time"] + prev["duration"]
            if (
                seg["speaker_id"] == prev["speaker_id"]
                and seg["start_time"] - prev_end <= gap_threshold
            ):
                new_end = seg["start_time"] + seg["duration"]
                prev["duration"] = round(new_end - prev["start_time"], 3)
            else:
                merged.append(seg.copy())
        return merged
