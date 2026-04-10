"""
说话人身份匹配器 —— 将分离结果中的匿名标签映射到已注册的真实姓名
"""

from __future__ import annotations

from typing import Optional

import numpy as np
from loguru import logger

from src.embedding import EmbeddingExtractor
from src.models import VoiceprintMatchResult
from src.voiceprint import VoiceprintManager


class SpeakerMatcher:
    """
    说话人身份匹配器。

    工作流程:
      1. 从分离结果中，对每个聚类簇提取代表性嵌入（取最长片段拼接）
      2. 遍历声纹库中所有已注册嵌入，计算余弦相似度
      3. 对每个匿名说话人，找到最高匹配分，超过阈值则映射为真实姓名
      4. 处理冲突：多个匿名说话人匹配到同一注册人时，保留分数最高者

    配置参数:
      match_threshold:           低于此值判定为"未匹配"
      high_confidence_threshold: 高于此值直接确认
    """

    def __init__(
        self,
        extractor: EmbeddingExtractor,
        voiceprint_mgr: VoiceprintManager,
        match_threshold: float = 0.68,
        high_confidence_threshold: float = 0.82,
    ):
        self.extractor = extractor
        self.voiceprint_mgr = voiceprint_mgr
        self.match_threshold = match_threshold
        self.high_confidence_threshold = high_confidence_threshold

    def match(
        self,
        cluster_embeddings: dict[str, np.ndarray],
    ) -> dict[str, VoiceprintMatchResult]:
        """
        批量匹配：将分离结果中所有匿名说话人映射到已注册身份。

        Args:
            cluster_embeddings: {anonymous_id: embedding}
                例: {"speaker_0": np.array([...]), "speaker_1": np.array([...])}

        Returns:
            {anonymous_id: VoiceprintMatchResult}
        """
        registered = self.voiceprint_mgr.get_all_embeddings()

        if not registered:
            logger.info("声纹库为空，所有说话人保持匿名标签")
            return {
                anon_id: VoiceprintMatchResult(
                    anonymous_id=anon_id,
                    confidence=0.0,
                    is_confident=False,
                )
                for anon_id in cluster_embeddings
            }

        # ─── 第一步：计算相似度矩阵 ───
        # similarity_matrix[anon_id][reg_id] = cosine_score
        similarity_matrix: dict[str, dict[str, float]] = {}

        for anon_id, anon_emb in cluster_embeddings.items():
            similarity_matrix[anon_id] = {}
            for reg_id, (reg_name, reg_emb) in registered.items():
                score = self.extractor.cosine_similarity(anon_emb, reg_emb)
                similarity_matrix[anon_id][reg_id] = score

        # ─── 第二步：贪心匹配（解决冲突）───
        results = self._greedy_match(similarity_matrix, registered)

        # 日志
        for anon_id, result in results.items():
            if result.matched_name:
                logger.info(
                    f"声纹匹配: {anon_id} → {result.matched_name} "
                    f"(置信度={result.confidence:.3f})"
                )
            else:
                logger.info(
                    f"声纹未匹配: {anon_id} (最高分={result.confidence:.3f})"
                )

        return results

    def _greedy_match(
        self,
        similarity_matrix: dict[str, dict[str, float]],
        registered: dict[str, tuple[str, np.ndarray]],
    ) -> dict[str, VoiceprintMatchResult]:
        """
        贪心匹配算法：

        1. 收集所有 (anon_id, reg_id, score) 三元组
        2. 按 score 降序排列
        3. 依次分配，确保一对一映射（一个匿名 ID 只匹配一个注册人，反之亦然）
        4. 未达阈值的保持为未匹配
        """
        # 收集所有候选对
        candidates = []
        for anon_id, scores in similarity_matrix.items():
            for reg_id, score in scores.items():
                candidates.append((score, anon_id, reg_id))

        # 按分数降序排列
        candidates.sort(key=lambda x: x[0], reverse=True)

        # 贪心分配
        matched_anon: set[str] = set()     # 已匹配的匿名 ID
        matched_reg: set[str] = set()      # 已匹配的注册 ID
        assignments: dict[str, tuple[str, str, float]] = {}  # anon_id → (reg_id, name, score)

        for score, anon_id, reg_id in candidates:
            if anon_id in matched_anon or reg_id in matched_reg:
                continue
            if score < self.match_threshold:
                continue

            reg_name = registered[reg_id][0]
            assignments[anon_id] = (reg_id, reg_name, score)
            matched_anon.add(anon_id)
            matched_reg.add(reg_id)

        # 构建结果
        results: dict[str, VoiceprintMatchResult] = {}
        for anon_id in similarity_matrix:
            if anon_id in assignments:
                reg_id, name, score = assignments[anon_id]
                results[anon_id] = VoiceprintMatchResult(
                    anonymous_id=anon_id,
                    matched_name=name,
                    matched_id=reg_id,
                    confidence=score,
                    is_confident=score >= self.high_confidence_threshold,
                )
            else:
                # 未匹配：记录最高分
                scores = similarity_matrix[anon_id]
                max_score = max(scores.values()) if scores else 0.0
                results[anon_id] = VoiceprintMatchResult(
                    anonymous_id=anon_id,
                    confidence=max_score,
                    is_confident=False,
                )

        return results

    def match_single(
        self,
        embedding: np.ndarray,
    ) -> VoiceprintMatchResult:
        """
        单个说话人匹配（用于实时场景或增量匹配）。
        """
        registered = self.voiceprint_mgr.get_all_embeddings()
        best_score = 0.0
        best_reg_id: Optional[str] = None
        best_name: Optional[str] = None

        for reg_id, (reg_name, reg_emb) in registered.items():
            score = self.extractor.cosine_similarity(embedding, reg_emb)
            if score > best_score:
                best_score = score
                best_reg_id = reg_id
                best_name = reg_name

        if best_score >= self.match_threshold:
            return VoiceprintMatchResult(
                anonymous_id="unknown",
                matched_name=best_name,
                matched_id=best_reg_id,
                confidence=best_score,
                is_confident=best_score >= self.high_confidence_threshold,
            )
        else:
            return VoiceprintMatchResult(
                anonymous_id="unknown",
                confidence=best_score,
                is_confident=False,
            )
