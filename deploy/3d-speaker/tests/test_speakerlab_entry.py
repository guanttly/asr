"""speakerlab 原生入口兼容性测试。"""

from __future__ import annotations

import importlib
import sys


def test_import_installs_pyannote_stub_when_missing(monkeypatch):
    monkeypatch.delitem(sys.modules, "pyannote.audio", raising=False)
    monkeypatch.delitem(sys.modules, "pyannote", raising=False)
    monkeypatch.delitem(sys.modules, "src.speakerlab_entry", raising=False)

    module = importlib.import_module("src.speakerlab_entry")

    assert module is not None
    assert "pyannote.audio" in sys.modules
    assert hasattr(sys.modules["pyannote.audio"], "Inference")
    assert hasattr(sys.modules["pyannote.audio"], "Model")


def test_ensure_pyannote_audio_keeps_real_module(monkeypatch):
    module = importlib.import_module("src.speakerlab_entry")

    class _RealPyannoteAudio:
        class Inference:
            pass

        class Model:
            pass

    monkeypatch.setitem(sys.modules, "pyannote.audio", _RealPyannoteAudio)

    module._ensure_pyannote_audio()

    assert sys.modules["pyannote.audio"] is _RealPyannoteAudio