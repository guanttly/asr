"""
VAD 检测单元测试
"""

import numpy as np

from src.vad import VoiceActivityDetector


class TestVoiceActivityDetector:
    def test_energy_vad_detects_single_speech_region(self):
        detector = VoiceActivityDetector.__new__(VoiceActivityDetector)
        detector.min_speech_duration = 0.1
        detector.min_silence_duration = 0.1
        detector.speech_pad_duration = 0.0

        sr = 16000
        silence = np.zeros(int(0.5 * sr), dtype=np.float32)
        speech = np.ones(int(1.0 * sr), dtype=np.float32) * 0.2
        waveform = np.concatenate([silence, speech, silence])

        segments = VoiceActivityDetector._run_energy_vad(detector, waveform, sr)

        assert len(segments) >= 1
        first = segments[0]
        assert 0.35 <= first.start_time <= 0.65
        assert 1.35 <= first.end_time <= 1.65

    def test_merge_segments_bridges_short_silence(self):
        detector = VoiceActivityDetector.__new__(VoiceActivityDetector)
        detector.min_silence_duration = 0.2

        segments = detector._merge_segments([
            detector._build_segment(0.0, 0.8, 5.0),
            detector._build_segment(0.9, 1.2, 5.0),
        ])

        assert len(segments) == 1
        assert segments[0].duration == 1.2