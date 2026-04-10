"""
声纹注册与匹配单元测试
"""

import os
import tempfile

import numpy as np
import pytest


class TestEmbeddingExtractor:
    """嵌入提取器测试（不依赖实际模型权重）"""

    def test_cosine_similarity_identical(self):
        """相同向量相似度应为 1.0"""
        from src.embedding import EmbeddingExtractor
        ext = EmbeddingExtractor.__new__(EmbeddingExtractor)
        vec = np.random.randn(192).astype(np.float32)
        score = ext.cosine_similarity(vec, vec)
        assert abs(score - 1.0) < 1e-5

    def test_cosine_similarity_orthogonal(self):
        """正交向量相似度应接近 0"""
        from src.embedding import EmbeddingExtractor
        ext = EmbeddingExtractor.__new__(EmbeddingExtractor)
        vec_a = np.zeros(192, dtype=np.float32)
        vec_a[0] = 1.0
        vec_b = np.zeros(192, dtype=np.float32)
        vec_b[1] = 1.0
        score = ext.cosine_similarity(vec_a, vec_b)
        assert abs(score) < 1e-5

    def test_cosine_similarity_opposite(self):
        """反方向向量相似度应为 -1.0"""
        from src.embedding import EmbeddingExtractor
        ext = EmbeddingExtractor.__new__(EmbeddingExtractor)
        vec = np.random.randn(192).astype(np.float32)
        score = ext.cosine_similarity(vec, -vec)
        assert abs(score + 1.0) < 1e-5


class TestVoiceprintManager:
    """声纹管理器测试（使用模拟嵌入）"""

    def _create_manager(self, tmp_dir):
        """创建测试用管理器（带 mock extractor）"""
        from unittest.mock import MagicMock
        from src.voiceprint import VoiceprintManager

        mock_extractor = MagicMock()
        # mock extract 返回随机嵌入
        mock_extractor.extract.return_value = np.random.randn(192).astype(np.float32)

        mgr = VoiceprintManager(
            extractor=mock_extractor,
            db_path=os.path.join(tmp_dir, "vp.db"),
            embeddings_dir=os.path.join(tmp_dir, "emb"),
            enrollment_audio_dir=os.path.join(tmp_dir, "audio"),
            min_enrollment_duration=1.0,  # 测试中降低要求
        )
        return mgr

    def _create_test_wav(self, duration: float = 5.0) -> str:
        """生成测试 WAV 文件"""
        import soundfile as sf
        tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False)
        sr = 16000
        samples = np.random.randn(int(duration * sr)).astype(np.float32) * 0.1
        sf.write(tmp.name, samples, sr)
        return tmp.name

    def test_enroll_and_list(self):
        """注册后应能查到记录"""
        with tempfile.TemporaryDirectory() as tmp_dir:
            mgr = self._create_manager(tmp_dir)
            wav_path = self._create_test_wav(5.0)

            try:
                record = mgr.enroll("张三", wav_path, department="技术部")
                assert record.speaker_name == "张三"
                assert record.department == "技术部"
                assert mgr.count == 1
                assert len(mgr.list_all()) == 1
            finally:
                os.unlink(wav_path)

    def test_enroll_and_delete(self):
        """注册后删除，计数应归零"""
        with tempfile.TemporaryDirectory() as tmp_dir:
            mgr = self._create_manager(tmp_dir)
            wav_path = self._create_test_wav(5.0)

            try:
                record = mgr.enroll("李四", wav_path)
                assert mgr.count == 1

                deleted = mgr.delete(record.id)
                assert deleted is True
                assert mgr.count == 0
            finally:
                os.unlink(wav_path)

    def test_enroll_from_embedding(self):
        """从嵌入向量直接注册"""
        with tempfile.TemporaryDirectory() as tmp_dir:
            mgr = self._create_manager(tmp_dir)
            emb = np.random.randn(192).astype(np.float32)

            record = mgr.enroll_from_embedding("王五", emb, audio_duration=10.0)
            assert record.speaker_name == "王五"
            assert mgr.count == 1

            stored_emb = mgr.get_embedding(record.id)
            assert stored_emb is not None
            assert stored_emb.shape == (192,)

    def test_persistence(self):
        """持久化：重新加载后记录应保留"""
        with tempfile.TemporaryDirectory() as tmp_dir:
            mgr = self._create_manager(tmp_dir)
            emb = np.random.randn(192).astype(np.float32)
            mgr.enroll_from_embedding("赵六", emb, audio_duration=10.0)

            # 重新创建管理器（模拟重启）
            mgr2 = self._create_manager(tmp_dir)
            assert mgr2.count == 1
            assert mgr2.list_all()[0].speaker_name == "赵六"

    def test_get_all_embeddings(self):
        """批量获取嵌入"""
        with tempfile.TemporaryDirectory() as tmp_dir:
            mgr = self._create_manager(tmp_dir)
            mgr.enroll_from_embedding("A", np.random.randn(192).astype(np.float32))
            mgr.enroll_from_embedding("B", np.random.randn(192).astype(np.float32))

            all_emb = mgr.get_all_embeddings()
            assert len(all_emb) == 2
            names = {v[0] for v in all_emb.values()}
            assert names == {"A", "B"}


class TestSpeakerMatcher:
    """说话人匹配器测试"""

    def test_match_high_confidence(self):
        """高相似度应成功匹配"""
        from unittest.mock import MagicMock
        from src.matcher import SpeakerMatcher

        mock_ext = MagicMock()
        mock_ext.cosine_similarity.return_value = 0.92

        mock_vp = MagicMock()
        base_emb = np.random.randn(192).astype(np.float32)
        mock_vp.get_all_embeddings.return_value = {
            "reg_001": ("张三", base_emb),
        }

        matcher = SpeakerMatcher(
            extractor=mock_ext,
            voiceprint_mgr=mock_vp,
            match_threshold=0.68,
            high_confidence_threshold=0.82,
        )

        cluster_embs = {"speaker_0": base_emb + np.random.randn(192).astype(np.float32) * 0.01}
        results = matcher.match(cluster_embs)

        assert "speaker_0" in results
        assert results["speaker_0"].matched_name == "张三"
        assert results["speaker_0"].is_confident is True

    def test_match_below_threshold(self):
        """低相似度不应匹配"""
        from unittest.mock import MagicMock
        from src.matcher import SpeakerMatcher

        mock_ext = MagicMock()
        mock_ext.cosine_similarity.return_value = 0.30  # 远低于阈值

        mock_vp = MagicMock()
        mock_vp.get_all_embeddings.return_value = {
            "reg_001": ("张三", np.random.randn(192).astype(np.float32)),
        }

        matcher = SpeakerMatcher(
            extractor=mock_ext,
            voiceprint_mgr=mock_vp,
            match_threshold=0.68,
        )

        results = matcher.match({"speaker_0": np.random.randn(192).astype(np.float32)})

        assert results["speaker_0"].matched_name is None
        assert results["speaker_0"].is_confident is False

    def test_greedy_no_duplicate_assignment(self):
        """贪心匹配应保证一对一，不重复分配"""
        from unittest.mock import MagicMock
        from src.matcher import SpeakerMatcher

        mock_ext = MagicMock()
        # speaker_0 和 speaker_1 都跟 "张三" 高度相似
        # 但 speaker_0 分数更高
        def sim_side_effect(a, b):
            if np.array_equal(a, cluster_0):
                return 0.95
            return 0.88

        mock_ext.cosine_similarity.side_effect = sim_side_effect

        mock_vp = MagicMock()
        reg_emb = np.random.randn(192).astype(np.float32)
        mock_vp.get_all_embeddings.return_value = {
            "reg_001": ("张三", reg_emb),
        }

        matcher = SpeakerMatcher(
            extractor=mock_ext,
            voiceprint_mgr=mock_vp,
            match_threshold=0.68,
        )

        cluster_0 = np.random.randn(192).astype(np.float32)
        cluster_1 = np.random.randn(192).astype(np.float32)

        results = matcher.match({
            "speaker_0": cluster_0,
            "speaker_1": cluster_1,
        })

        # speaker_0 应匹配到张三（分数更高）
        assert results["speaker_0"].matched_name == "张三"
        # speaker_1 不应再匹配到张三（已被 speaker_0 占用）
        assert results["speaker_1"].matched_name is None

    def test_empty_voiceprint_db(self):
        """声纹库为空时所有人保持匿名"""
        from unittest.mock import MagicMock
        from src.matcher import SpeakerMatcher

        mock_ext = MagicMock()
        mock_vp = MagicMock()
        mock_vp.get_all_embeddings.return_value = {}

        matcher = SpeakerMatcher(extractor=mock_ext, voiceprint_mgr=mock_vp)

        results = matcher.match({
            "speaker_0": np.random.randn(192).astype(np.float32),
            "speaker_1": np.random.randn(192).astype(np.float32),
        })

        assert results["speaker_0"].matched_name is None
        assert results["speaker_1"].matched_name is None
