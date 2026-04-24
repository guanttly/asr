"""
说话人分离引擎测试
"""

import threading

from src.engine import DiarizationEngine, _clean_subprocess_output, _summarize_subprocess_output
from src.utils import segments_to_rttm


class TestRTTMParsing:
    """RTTM 格式测试"""

    def test_segments_to_rttm(self):
        """测试 RTTM 格式输出"""
        segments = [
            {"speaker_id": "speaker_0", "start_time": 0.5, "duration": 3.2},
            {"speaker_id": "speaker_1", "start_time": 4.0, "duration": 5.1},
        ]
        rttm = segments_to_rttm(segments, file_id="test")
        lines = rttm.strip().split("\n")
        assert len(lines) == 2
        assert "speaker_0" in lines[0]
        assert "speaker_1" in lines[1]
        assert "SPEAKER test" in lines[0]

    def test_parse_rttm(self):
        """测试 RTTM 解析"""
        import tempfile, os
        rttm_content = (
            "SPEAKER test 1 0.500 3.200 <NA> <NA> speaker_0 <NA> <NA>\n"
            "SPEAKER test 1 4.000 5.100 <NA> <NA> speaker_1 <NA> <NA>\n"
        )
        tmp = tempfile.NamedTemporaryFile(mode="w", suffix=".rttm", delete=False)
        tmp.write(rttm_content)
        tmp.close()

        try:
            segments = DiarizationEngine._parse_rttm(tmp.name)
            assert len(segments) == 2
            assert segments[0]["speaker_id"] == "speaker_0"
            assert segments[0]["start_time"] == 0.5
            assert segments[0]["duration"] == 3.2
            assert segments[1]["speaker_id"] == "speaker_1"
        finally:
            os.unlink(tmp.name)


class TestMergeSegments:
    """片段合并测试"""

    def test_merge_adjacent_same_speaker(self):
        """相邻同一说话人片段应合并"""
        segments = [
            {"speaker_id": "A", "start_time": 0.0, "duration": 2.0},
            {"speaker_id": "A", "start_time": 2.1, "duration": 3.0},  # gap=0.1
            {"speaker_id": "B", "start_time": 5.5, "duration": 2.0},
        ]
        merged = DiarizationEngine._merge_adjacent_segments(segments, gap_threshold=0.3)
        assert len(merged) == 2
        assert merged[0]["speaker_id"] == "A"
        assert merged[0]["duration"] == pytest.approx(5.1, abs=0.01)
        assert merged[1]["speaker_id"] == "B"

    def test_no_merge_different_speakers(self):
        """不同说话人不应合并"""
        segments = [
            {"speaker_id": "A", "start_time": 0.0, "duration": 2.0},
            {"speaker_id": "B", "start_time": 2.1, "duration": 3.0},
        ]
        merged = DiarizationEngine._merge_adjacent_segments(segments, gap_threshold=0.3)
        assert len(merged) == 2

    def test_no_merge_large_gap(self):
        """间隔过大的同一说话人不应合并"""
        segments = [
            {"speaker_id": "A", "start_time": 0.0, "duration": 2.0},
            {"speaker_id": "A", "start_time": 5.0, "duration": 3.0},  # gap=3.0
        ]
        merged = DiarizationEngine._merge_adjacent_segments(segments, gap_threshold=0.3)
        assert len(merged) == 2

    def test_empty_segments(self):
        """空列表"""
        merged = DiarizationEngine._merge_adjacent_segments([])
        assert merged == []


class TestSubprocessOutputCleaning:
    """子进程输出清洗测试"""

    def test_clean_subprocess_output_strips_ansi_and_blank_lines(self):
        raw = "\u001b[A\u001b[0mDownloading model.bin: 100%\r\n\r\nTraceback line\n"
        cleaned = _clean_subprocess_output(raw)
        assert "\u001b" not in cleaned
        assert cleaned == "Downloading model.bin: 100%\nTraceback line"

    def test_summarize_subprocess_output_keeps_tail_lines(self):
        raw = "\n".join([f"line-{index}" for index in range(30)])
        summary = _summarize_subprocess_output(raw, max_lines=5)
        assert summary == "line-25\nline-26\nline-27\nline-28\nline-29"


class TestNativeModelCache:
    """原生 diarization 模型缓存测试"""

    def test_native_model_cache_ready_returns_false_when_checkpoint_missing(self, tmp_path):
        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine.native_model_cache_dir = str(tmp_path)
        assert engine._native_model_cache_ready() is False

    def test_native_model_cache_ready_returns_true_when_checkpoint_exists(self, tmp_path):
        checkpoint = tmp_path / "iic" / "speech_campplus_sv_zh_en_16k-common_advanced" / "campplus_cn_en_common.pt"
        checkpoint.parent.mkdir(parents=True, exist_ok=True)
        checkpoint.write_bytes(b"ok")

        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine.native_model_cache_dir = str(tmp_path)
        assert engine._native_model_cache_ready() is True

    def test_native_model_cache_ready_hydrates_runtime_cache_from_seed(self, tmp_path, monkeypatch):
        runtime_cache = tmp_path / "runtime"
        seed_cache = tmp_path / "seed"
        checkpoint = seed_cache / "iic" / "speech_campplus_sv_zh_en_16k-common_advanced" / "campplus_cn_en_common.pt"
        checkpoint.parent.mkdir(parents=True, exist_ok=True)
        checkpoint.write_bytes(b"ok")

        monkeypatch.setenv("NATIVE_MODEL_CACHE_SEED_DIR", str(seed_cache))

        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine.native_model_cache_dir = str(runtime_cache)

        assert engine._native_model_cache_ready() is True
        assert any(runtime_cache.rglob("campplus_cn_en_common.pt"))

    def test_diarize_native_reuses_initialized_pipeline(self, monkeypatch):
        class DummyExtractor:
            device = "cpu"

        class DummyPipeline:
            def __init__(self):
                self.calls = []

            def __call__(self, audio_path, speaker_num=None):
                self.calls.append((audio_path, speaker_num))
                return [[0.0, 1.2, 0]]

        created = []
        pipeline = DummyPipeline()

        def fake_create_diarization_pipeline(**kwargs):
            created.append(kwargs)
            return pipeline

        def fake_load_audio(*args, **kwargs):
            return np.zeros(16000, dtype=np.float32), 16000

        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine.extractor = DummyExtractor()
        engine.target_sr = 16000
        engine.native_model_cache_dir = None
        engine._native_pipeline = None
        engine._native_pipeline_lock = threading.Lock()
        engine._native_inference_lock = threading.Lock()

        monkeypatch.setattr("src.speakerlab_entry.create_diarization_pipeline", fake_create_diarization_pipeline)
        monkeypatch.setattr("src.engine.load_audio", fake_load_audio)
        monkeypatch.setattr(engine, "_extract_cluster_embeddings", lambda waveform, sr, segments: {"speaker_0": np.ones(3, dtype=np.float32)})

        first_segments, _ = engine._diarize_native("/tmp/a.wav", None, None, "spectral")
        second_segments, _ = engine._diarize_native("/tmp/b.wav", None, None, "spectral")

        assert len(created) == 1
        assert pipeline.calls == [("/tmp/a.wav", None), ("/tmp/b.wav", None)]
        assert first_segments[0]["speaker_id"] == "speaker_0"
        assert second_segments[0]["speaker_id"] == "speaker_0"

    def test_warmup_native_pipeline_initializes_pipeline_when_ready(self, monkeypatch):
        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine._native_available = True

        warmups = []

        monkeypatch.setattr(engine, "_native_model_cache_ready", lambda: True)
        monkeypatch.setattr(engine, "_get_native_pipeline", lambda: warmups.append("ready") or object())

        assert engine.warmup_native_pipeline() is True
        assert warmups == ["ready"]

    def test_warmup_native_pipeline_skips_when_unavailable(self):
        engine = DiarizationEngine.__new__(DiarizationEngine)
        engine._native_available = False

        assert engine.warmup_native_pipeline() is False


# 需要 pytest
import numpy as np
import pytest
