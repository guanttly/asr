"""
工具函数 —— 音频预处理、格式转换、通用辅助
"""

from __future__ import annotations

import os
import tempfile
from pathlib import Path

import numpy as np
import soundfile as sf
from loguru import logger


def load_audio(
    file_path: str | Path,
    target_sr: int = 16000,
    mono: bool = True,
) -> tuple[np.ndarray, int]:
    """
    加载音频文件并统一为目标采样率和单声道。

    Returns:
        (waveform, sample_rate)  waveform shape: (num_samples,)
    """
    file_path = str(file_path)

    try:
        import librosa
        waveform, sr = librosa.load(file_path, sr=target_sr, mono=mono)
    except Exception:
        # fallback: soundfile + 手动重采样
        waveform, sr = sf.read(file_path, dtype="float32")
        if waveform.ndim > 1 and mono:
            waveform = np.mean(waveform, axis=1)
        if sr != target_sr:
            import scipy.signal
            num_samples = int(len(waveform) * target_sr / sr)
            waveform = scipy.signal.resample(waveform, num_samples)
            sr = target_sr

    logger.debug(f"音频加载完成: {file_path}, 时长={len(waveform)/sr:.2f}s, sr={sr}")
    return waveform, sr


def get_audio_duration(file_path: str | Path) -> float:
    """获取音频时长（秒）"""
    info = sf.info(str(file_path))
    return info.duration


def save_audio(
    waveform: np.ndarray,
    file_path: str | Path,
    sample_rate: int = 16000,
) -> str:
    """保存音频到文件"""
    file_path = str(file_path)
    os.makedirs(os.path.dirname(file_path) or ".", exist_ok=True)
    sf.write(file_path, waveform, sample_rate, subtype="PCM_16")
    return file_path


def extract_segment(
    waveform: np.ndarray,
    sr: int,
    start_time: float,
    end_time: float,
) -> np.ndarray:
    """从波形中截取指定时间范围的片段"""
    start_sample = int(start_time * sr)
    end_sample = int(end_time * sr)
    start_sample = max(0, start_sample)
    end_sample = min(len(waveform), end_sample)
    return waveform[start_sample:end_sample]


def concat_segments(
    waveform: np.ndarray,
    sr: int,
    segments: list[tuple[float, float]],
    max_duration: float = 30.0,
) -> np.ndarray:
    """
    拼接多个时间片段，生成代表性音频。
    用于从说话人分离结果中提取某个说话人的代表性片段。

    Args:
        segments: [(start_time, end_time), ...]
        max_duration: 最大拼接时长
    """
    pieces = []
    total_duration = 0.0

    # 按片段时长降序排列，优先取长片段
    sorted_segs = sorted(segments, key=lambda s: s[1] - s[0], reverse=True)

    for start, end in sorted_segs:
        if total_duration >= max_duration:
            break
        piece = extract_segment(waveform, sr, start, end)
        if len(piece) > 0:
            pieces.append(piece)
            total_duration += len(piece) / sr

    if not pieces:
        return np.array([], dtype=np.float32)

    return np.concatenate(pieces)


def save_temp_audio(waveform: np.ndarray, sr: int = 16000) -> str:
    """保存到临时文件，返回路径"""
    tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False)
    sf.write(tmp.name, waveform, sr, subtype="PCM_16")
    return tmp.name


def segments_to_rttm(
    segments: list[dict],
    file_id: str = "audio",
) -> str:
    """
    将分离结果转为 RTTM 格式字符串。

    Args:
        segments: [{"speaker_id": "speaker_0", "start_time": 0.5, "duration": 3.2}, ...]
    """
    lines = []
    for seg in segments:
        speaker = seg["speaker_id"]
        start = seg["start_time"]
        dur = seg["duration"]
        line = f"SPEAKER {file_id} 1 {start:.3f} {dur:.3f} <NA> <NA> {speaker} <NA> <NA>"
        lines.append(line)
    return "\n".join(lines)
