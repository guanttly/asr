"""
speakerlab 原生分离兼容入口。

上游 infer_diarization 在模块导入阶段会直接依赖 pyannote.audio，
但当前服务的基础音频分离路径并不启用 overlap detection。
这里在缺少 pyannote.audio 时注入最小 stub，使非 overlap 场景仍可复用上游流水线。
"""

from __future__ import annotations

import importlib
import sys
import types


def _ensure_pyannote_audio() -> None:
    try:
        import pyannote.audio  # noqa: F401
        return
    except ImportError:
        pass

    if "pyannote.audio" in sys.modules:
        return

    pyannote_pkg = sys.modules.setdefault("pyannote", types.ModuleType("pyannote"))
    pyannote_audio = types.ModuleType("pyannote.audio")

    class _MissingModel:
        @classmethod
        def from_pretrained(cls, *args, **kwargs):
            raise ImportError("pyannote.audio 未安装，仅在 include_overlap=True 时需要")

    class _MissingInference:
        def __init__(self, *args, **kwargs):
            raise ImportError("pyannote.audio 未安装，仅在 include_overlap=True 时需要")

        @staticmethod
        def aggregate(*args, **kwargs):
            raise ImportError("pyannote.audio 未安装，仅在 include_overlap=True 时需要")

    pyannote_audio.Model = _MissingModel
    pyannote_audio.Inference = _MissingInference
    pyannote_pkg.audio = pyannote_audio
    sys.modules["pyannote.audio"] = pyannote_audio


# torch.multiprocessing spawn 会在子进程里重新导入当前入口模块。
# 这里在模块导入阶段就完成 stub 注入，避免子进程在反序列化
# speakerlab.bin.infer_diarization 的函数对象前先触发 pyannote 导入失败。
_ensure_pyannote_audio()


def _load_infer_module():
    return importlib.import_module("speakerlab.bin.infer_diarization")


def create_diarization_pipeline(
    *,
    device=None,
    include_overlap: bool = False,
    hf_access_token=None,
    speaker_num=None,
    model_cache_dir=None,
):
    module = _load_infer_module()
    return module.Diarization3Dspeaker(
        device=device,
        include_overlap=include_overlap,
        hf_access_token=hf_access_token,
        speaker_num=speaker_num,
        model_cache_dir=model_cache_dir,
    )


def can_run_native_pipeline() -> bool:
    try:
        _load_infer_module()
        return True
    except Exception:
        return False


def main() -> None:
    module = _load_infer_module()
    module.main()


if __name__ == "__main__":
    main()