"""
语音活动检测器 —— FSMN-VAD 优先，失败时回退到能量阈值 VAD
"""

from __future__ import annotations

import ast
import os
import time
import uuid
from pathlib import Path
from typing import Any, Optional

import numpy as np
from loguru import logger

from src.models import VADResponse, VADSegment
from src.utils import load_audio, save_temp_audio


class VoiceActivityDetector:
    def __init__(
        self,
        model_id: str = "iic/speech_fsmn_vad_zh-cn-16k-common-pytorch",
        local_dir: Optional[str] = None,
        device: str = "cpu",
        target_sr: int = 16000,
        min_speech_duration: float = 0.2,
        min_silence_duration: float = 0.15,
        speech_pad_duration: float = 0.1,
    ):
        self.model_id = model_id
        self.local_dir = local_dir
        self.device = device
        self.target_sr = target_sr
        self.min_speech_duration = min_speech_duration
        self.min_silence_duration = min_silence_duration
        self.speech_pad_duration = speech_pad_duration
        self._pipeline = None
        self._loaded = False
        self.backend = "energy"

    def load(self) -> None:
        if self._loaded:
            return

        try:
            from modelscope.pipelines import pipeline as ms_pipeline

            model_ref = self._resolve_model_reference()
            self._pipeline = ms_pipeline(
                task="voice-activity-detection",
                model=model_ref,
                device="gpu" if self.device.startswith("cuda") else "cpu",
            )
            self.backend = "modelscope-fsmn-vad"
            logger.info(f"VAD 模型加载完成: {model_ref} (device={self.device})")
        except Exception as exc:
            self._pipeline = None
            self.backend = "energy"
            logger.warning(f"VAD 模型加载失败，回退到能量阈值模式: {exc}")

        self._loaded = True

    @property
    def is_loaded(self) -> bool:
        return self._loaded

    def detect(self, audio_path: str) -> VADResponse:
        task_id = str(uuid.uuid4())[:8]
        t_start = time.time()
        waveform, sr = load_audio(audio_path, target_sr=self.target_sr, mono=True)
        audio_duration = len(waveform) / sr if sr else 0.0
        segments = self.detect_segments(audio_path, waveform=waveform, sample_rate=sr)
        speech_duration = round(sum(seg.duration for seg in segments), 3)

        return VADResponse(
            task_id=task_id,
            audio_duration=round(audio_duration, 3),
            speech_duration=speech_duration,
            speech_ratio=round((speech_duration / audio_duration) * 100, 2) if audio_duration > 0 else 0.0,
            num_segments=len(segments),
            segments=segments,
            detector_backend=self.backend,
            processing_time=round(time.time() - t_start, 3),
        )

    def detect_segments(
        self,
        audio_path: str,
        waveform: Optional[np.ndarray] = None,
        sample_rate: Optional[int] = None,
    ) -> list[VADSegment]:
        self.load()

        if waveform is None or sample_rate is None:
            waveform, sample_rate = load_audio(audio_path, target_sr=self.target_sr, mono=True)

        audio_duration = len(waveform) / sample_rate if sample_rate else 0.0

        if self._pipeline is not None:
            try:
                result = self._pipeline(audio_path)
                segments = self._parse_pipeline_output(result, audio_duration)
                if segments:
                    return self._merge_segments(segments)
            except Exception as exc:
                logger.warning(f"VAD 模型推理失败，回退到能量阈值模式: {exc}")

        self.backend = "energy"
        return self._run_energy_vad(waveform, sample_rate)

    def detect_segments_from_waveform(self, waveform: np.ndarray, sample_rate: int) -> list[VADSegment]:
        tmp_path = save_temp_audio(waveform, sample_rate)
        try:
            return self.detect_segments(tmp_path, waveform=waveform, sample_rate=sample_rate)
        finally:
            os.unlink(tmp_path)

    def _resolve_model_reference(self) -> str:
        if self.local_dir and os.path.isdir(self.local_dir):
            return self.local_dir
        return self.model_id

    def _parse_pipeline_output(self, payload: Any, audio_duration: float) -> list[VADSegment]:
        segments: list[VADSegment] = []

        def collect(item: Any):
            if item is None:
                return

            if isinstance(item, dict):
                segment = self._build_segment(
                    item.get("start", item.get("start_time", item.get("begin", item.get("begin_time")))),
                    item.get("end", item.get("end_time", item.get("stop", item.get("finish_time")))),
                    audio_duration,
                )
                if segment is not None:
                    segments.append(segment)
                    return

                for key in ("segments", "text", "value", "output", "result", "results", "prediction", "predictions"):
                    if key in item:
                        collect(item[key])
                return

            if isinstance(item, str):
                text = item.strip()
                if not text:
                    return
                if text[0] in "[{(":
                    try:
                        collect(ast.literal_eval(text))
                        return
                    except Exception:
                        pass
                for line in text.splitlines():
                    line = line.strip()
                    if not line:
                        continue
                    parts = line.replace(",", " ").split()
                    if len(parts) >= 2:
                        segment = self._build_segment(parts[0], parts[1], audio_duration)
                        if segment is not None:
                            segments.append(segment)
                return

            if isinstance(item, (list, tuple)):
                if len(item) >= 2 and self._is_number(item[0]) and self._is_number(item[1]):
                    segment = self._build_segment(item[0], item[1], audio_duration)
                    if segment is not None:
                        segments.append(segment)
                        return
                for child in item:
                    collect(child)

        collect(payload)
        return self._merge_segments(segments)

    def _build_segment(self, start: Any, end: Any, audio_duration: float) -> Optional[VADSegment]:
        start_value = self._normalize_time(start, audio_duration)
        end_value = self._normalize_time(end, audio_duration)
        if start_value is None or end_value is None:
            return None
        if end_value < start_value:
            start_value, end_value = end_value, start_value
        if end_value - start_value <= 0:
            return None
        return VADSegment(
            start_time=round(start_value, 3),
            end_time=round(end_value, 3),
            duration=round(end_value - start_value, 3),
        )

    def _normalize_time(self, value: Any, audio_duration: float) -> Optional[float]:
        if not self._is_number(value):
            return None
        numeric = float(value)
        if audio_duration > 0 and numeric > max(audio_duration * 1.5, 30.0):
            numeric /= 1000.0
        return max(0.0, numeric)

    @staticmethod
    def _is_number(value: Any) -> bool:
        try:
            float(value)
            return True
        except (TypeError, ValueError):
            return False

    def _run_energy_vad(self, waveform: np.ndarray, sample_rate: int) -> list[VADSegment]:
        if len(waveform) == 0:
            return []

        frame_length = max(1, int(0.025 * sample_rate))
        hop_length = max(1, int(0.010 * sample_rate))
        energy = np.array([
            np.mean(np.square(waveform[idx:idx + frame_length]))
            for idx in range(0, max(len(waveform) - frame_length, 1), hop_length)
        ])

        if len(energy) == 0:
            return []

        floor = float(np.percentile(energy, 20))
        ceiling = float(np.percentile(energy, 85))
        threshold = max(floor + (ceiling - floor) * 0.35, 1e-6)
        speech_mask = energy > threshold

        min_speech_frames = max(1, int(self.min_speech_duration / (hop_length / sample_rate)))
        min_silence_frames = max(1, int(self.min_silence_duration / (hop_length / sample_rate)))
        pad_frames = max(0, int(self.speech_pad_duration / (hop_length / sample_rate)))

        segments: list[VADSegment] = []
        start_frame: Optional[int] = None
        silence_count = 0

        for frame_index, is_speech in enumerate(speech_mask):
            if is_speech:
                if start_frame is None:
                    start_frame = frame_index
                silence_count = 0
                continue

            if start_frame is None:
                continue

            silence_count += 1
            if silence_count < min_silence_frames:
                continue

            end_frame = frame_index - silence_count + 1
            if end_frame - start_frame >= min_speech_frames:
                segments.append(self._frames_to_segment(start_frame, end_frame, hop_length, sample_rate, pad_frames, len(waveform)))
            start_frame = None
            silence_count = 0

        if start_frame is not None:
            end_frame = len(speech_mask)
            if end_frame - start_frame >= min_speech_frames:
                segments.append(self._frames_to_segment(start_frame, end_frame, hop_length, sample_rate, pad_frames, len(waveform)))

        return self._merge_segments([seg for seg in segments if seg is not None])

    def _frames_to_segment(
        self,
        start_frame: int,
        end_frame: int,
        hop_length: int,
        sample_rate: int,
        pad_frames: int,
        waveform_length: int,
    ) -> VADSegment:
        start_sample = max(0, (start_frame - pad_frames) * hop_length)
        end_sample = min(waveform_length, (end_frame + pad_frames) * hop_length)
        start_time = start_sample / sample_rate
        end_time = end_sample / sample_rate
        return VADSegment(
            start_time=round(start_time, 3),
            end_time=round(end_time, 3),
            duration=round(end_time - start_time, 3),
        )

    def _merge_segments(self, segments: list[VADSegment]) -> list[VADSegment]:
        if not segments:
            return []

        ordered = sorted(segments, key=lambda item: item.start_time)
        merged = [ordered[0]]

        for segment in ordered[1:]:
            previous = merged[-1]
            if segment.start_time - previous.end_time <= self.min_silence_duration:
                merged[-1] = VADSegment(
                    start_time=previous.start_time,
                    end_time=max(previous.end_time, segment.end_time),
                    duration=round(max(previous.end_time, segment.end_time) - previous.start_time, 3),
                )
            else:
                merged.append(segment)

        return merged