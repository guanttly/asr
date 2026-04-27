"""
说话人分离服务 - 客户端调用示例

展示应用服务器如何调用说话人分离接口，
并与 ASR 转写结果合并生成会议记录。
"""

import requests
import json
from typing import Optional


# ========================================
# 基础调用示例
# ========================================

def diarize_audio(
    audio_path: str,
    server_url: str = "http://localhost:8080",
    num_speakers: Optional[int] = None,
) -> dict:
    """
    调用说话人分离接口

    Args:
        audio_path: 音频文件路径
        server_url: 说话人分离服务地址
        num_speakers: 预设说话人数量（可选）

    Returns:
        分离结果 dict
    """
    url = f"{server_url}/diarize"
    params = {}
    if num_speakers:
        params["num_speakers"] = num_speakers

    with open(audio_path, "rb") as f:
        response = requests.post(
            url,
            files={"file": (audio_path.split("/")[-1], f)},
            params=params,
            timeout=600,
        )

    response.raise_for_status()
    return response.json()


# ========================================
# 与 ASR 结果合并的示例
# ========================================

def merge_diarization_with_asr(
    diarization_result: dict,
    asr_result: list[dict],
    overlap_threshold: float = 0.5,
) -> list[dict]:
    """
    将说话人分离结果与 ASR 转写结果按时间戳对齐合并。

    Args:
        diarization_result: 说话人分离返回的结果
            {"segments": [{"speaker": "spk_0", "start": 0.5, "end": 3.2}, ...]}
        asr_result: ASR 转写返回的结果
            [{"text": "你好", "start": 0.5, "end": 1.2}, ...]
        overlap_threshold: 时间重叠比例阈值，超过此比例则认为匹配

    Returns:
        合并后的结果:
        [{"speaker": "spk_0", "start": 0.5, "end": 3.2, "text": "你好世界"}, ...]
    """
    diar_segments = diarization_result.get("segments", [])
    merged = []

    for asr_seg in asr_result:
        asr_start = asr_seg["start"]
        asr_end = asr_seg["end"]
        asr_duration = asr_end - asr_start

        if asr_duration <= 0:
            continue

        # 找与 ASR 片段时间重叠最多的说话人
        best_speaker = "unknown"
        best_overlap = 0

        for diar_seg in diar_segments:
            # 计算时间重叠
            overlap_start = max(asr_start, diar_seg["start"])
            overlap_end = min(asr_end, diar_seg["end"])
            overlap = max(0, overlap_end - overlap_start)

            if overlap > best_overlap:
                best_overlap = overlap
                best_speaker = diar_seg["speaker"]

        # 重叠比例检查
        overlap_ratio = best_overlap / asr_duration if asr_duration > 0 else 0
        if overlap_ratio < overlap_threshold:
            best_speaker = "unknown"

        merged.append({
            "speaker": best_speaker,
            "start": asr_start,
            "end": asr_end,
            "text": asr_seg["text"],
        })

    return merged


def format_meeting_transcript(merged_result: list[dict]) -> str:
    """
    将合并结果格式化为可读的会议记录文本。

    输出示例:
      [说话人1] 00:00:01 - 00:00:05
      我们先讨论一下第一个议题

      [说话人2] 00:00:06 - 00:00:12
      好的，我来汇报一下最新进展
    """

    def fmt_time(seconds: float) -> str:
        m, s = divmod(int(seconds), 60)
        h, m = divmod(m, 60)
        return f"{h:02d}:{m:02d}:{s:02d}"

    # 合并相邻的相同说话人片段
    consolidated = []
    for seg in merged_result:
        if (
            consolidated
            and consolidated[-1]["speaker"] == seg["speaker"]
            and seg["start"] - consolidated[-1]["end"] < 1.0  # 间隔 < 1秒则合并
        ):
            consolidated[-1]["end"] = seg["end"]
            consolidated[-1]["text"] += seg["text"]
        else:
            consolidated.append(dict(seg))

    # 格式化输出
    lines = []
    speaker_map = {}  # spk_0 → 说话人1
    counter = 1

    for seg in consolidated:
        spk = seg["speaker"]
        if spk not in speaker_map:
            speaker_map[spk] = f"说话人{counter}"
            counter += 1

        display_name = speaker_map[spk]
        time_range = f"{fmt_time(seg['start'])} - {fmt_time(seg['end'])}"

        lines.append(f"[{display_name}] {time_range}")
        lines.append(f"{seg['text']}")
        lines.append("")

    return "\n".join(lines)


# ========================================
# 完整调用流程示例
# ========================================

def process_meeting_audio(
    audio_path: str,
    service_url: str = "http://localhost:8080",
    asr_url: str = "http://localhost:8000",
):
    """
    完整的会议音频处理流程：

    1. 调用说话人分离服务
    2. 调用 ASR 转写服务
    3. 合并结果
    4. 输出格式化会议记录
    """
    print(f"处理音频: {audio_path}")
    print("=" * 50)

    # 第一步：说话人分离
    print("\n[1/3] 调用说话人分离服务...")
    diar_result = diarize_audio(audio_path, service_url)
    print(f"  检测到 {diar_result['num_speakers']} 位说话人")
    print(f"  共 {len(diar_result['segments'])} 个分段")
    print(f"  耗时: {diar_result['process_time']}s")

    # 第二步：ASR 转写（示例，实际替换为你的 ASR 服务调用）
    print("\n[2/3] 调用 ASR 转写服务...")
    # asr_result = call_asr_service(audio_path, asr_url)
    # 这里用模拟数据演示
    asr_result = [
        {"text": "我们先讨论一下今天的第一个议题", "start": 0.5, "end": 3.2},
        {"text": "好的我来汇报一下最新的进展情况", "start": 3.5, "end": 8.1},
        {"text": "数据显示上个月的指标有明显提升", "start": 8.3, "end": 12.7},
        {"text": "这个结果很不错下一步计划是什么", "start": 13.0, "end": 16.5},
    ]
    print(f"  转写完成: {len(asr_result)} 个句子")

    # 第三步：合并
    print("\n[3/3] 合并说话人与转写结果...")
    merged = merge_diarization_with_asr(diar_result, asr_result)

    # 输出
    print("\n" + "=" * 50)
    print("会议记录：")
    print("=" * 50)
    transcript = format_meeting_transcript(merged)
    print(transcript)

    return {
        "diarization": diar_result,
        "asr": asr_result,
        "merged": merged,
        "transcript": transcript,
    }


# ========================================
# 直接运行
# ========================================
if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("用法: python client_example.py <音频文件路径>")
        print("")
        print("示例:")
        print("  python client_example.py meeting.wav")
        print("  python client_example.py meeting.mp3")
        print("")
        print("确保说话人分离服务已启动: http://localhost:8080")
        sys.exit(1)

    audio_file = sys.argv[1]
    result = process_meeting_audio(audio_file)

    # 保存结果
    output_file = audio_file.rsplit(".", 1)[0] + "_transcript.json"
    with open(output_file, "w", encoding="utf-8") as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    print(f"\n结果已保存: {output_file}")
