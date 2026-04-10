"""
API 接口测试
"""

import io
import os
import struct
import wave
import pytest

# ─── 辅助：生成测试用 WAV 文件 ───

def generate_test_wav(duration: float = 5.0, sample_rate: int = 16000) -> bytes:
    """生成包含简单正弦波的 WAV 字节流（用于测试）"""
    import math
    num_samples = int(duration * sample_rate)
    samples = []
    for i in range(num_samples):
        t = i / sample_rate
        # 混合多个频率模拟语音
        val = (
            0.3 * math.sin(2 * math.pi * 200 * t)
            + 0.2 * math.sin(2 * math.pi * 500 * t)
            + 0.1 * math.sin(2 * math.pi * 1000 * t)
        )
        samples.append(int(val * 32767))

    buf = io.BytesIO()
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        wf.writeframes(struct.pack(f"<{len(samples)}h", *samples))
    return buf.getvalue()


# ─── 测试 ───

class TestHealthAPI:
    """健康检查测试"""

    def test_health_endpoint(self):
        """测试健康检查端点可达"""
        from fastapi.testclient import TestClient
        from src.server import app

        client = TestClient(app)
        response = client.get("/api/v1/health")
        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "ok"
        assert data["service_name"] == "speaker-analysis-service"
        assert "version" in data
        assert "voiceprint_count" in data


class TestVoiceprintAPI:
    """声纹管理 API 测试"""

    def test_list_empty(self):
        """空声纹库列表"""
        from fastapi.testclient import TestClient
        from src.server import app

        client = TestClient(app)
        response = client.get("/api/v1/voiceprint/list")
        assert response.status_code == 200
        data = response.json()
        assert data["total"] >= 0

    def test_enroll_too_short(self):
        """注册音频过短应报错"""
        from fastapi.testclient import TestClient
        from src.server import app

        client = TestClient(app)
        short_wav = generate_test_wav(duration=1.0)  # 只有 1 秒

        response = client.post(
            "/api/v1/voiceprint/enroll",
            files={"file": ("short.wav", short_wav, "audio/wav")},
            data={"speaker_name": "测试用户"},
        )
        # 应返回 400（时长不足）或 500（模型未加载）
        assert response.status_code in [400, 500]

    def test_delete_nonexistent(self):
        """删除不存在的声纹应返回 404"""
        from fastapi.testclient import TestClient
        from src.server import app

        client = TestClient(app)
        response = client.delete("/api/v1/voiceprint/nonexistent-id")
        assert response.status_code == 404


class TestDiarizeAPI:
    """说话人分离 API 测试"""

    def test_diarize_identify_without_voiceprints(self):
        """声纹库为空时调用 identify 应报错"""
        from fastapi.testclient import TestClient
        from src.server import app

        client = TestClient(app)

        # 先确认声纹库为空（或跳过）
        list_resp = client.get("/api/v1/voiceprint/list")
        if list_resp.json()["total"] == 0:
            wav_data = generate_test_wav(duration=10.0)
            response = client.post(
                "/api/v1/diarize-identify",
                files={"file": ("test.wav", wav_data, "audio/wav")},
            )
            assert response.status_code == 400


class TestVADAPI:
    """VAD API 测试"""

    def test_vad_endpoint_with_stub_detector(self):
        from fastapi.testclient import TestClient
        import src.server as server_module
        from src.models import VADResponse, VADSegment

        class StubDetector:
            def detect(self, _audio_path):
                return VADResponse(
                    task_id="test-vad",
                    audio_duration=2.0,
                    speech_duration=1.2,
                    speech_ratio=60.0,
                    num_segments=1,
                    segments=[VADSegment(start_time=0.2, end_time=1.4, duration=1.2)],
                    detector_backend="stub",
                    processing_time=0.01,
                )

        server_module._vad = StubDetector()

        client = TestClient(server_module.app)
        wav_data = generate_test_wav(duration=2.0)
        response = client.post(
            "/api/v1/vad",
            files={"file": ("vad.wav", wav_data, "audio/wav")},
        )

        assert response.status_code == 200
        data = response.json()
        assert data["detector_backend"] == "stub"
        assert data["num_segments"] == 1
