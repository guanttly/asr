"""
嵌入提取器 —— 封装 3D-Speaker 的说话人嵌入模型（ERes2NetV2 / CAM++）
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

import numpy as np
import torch
from loguru import logger


class EmbeddingExtractor:
    """
    说话人嵌入提取器。

    支持两种加载方式:
      1. 从 ModelScope 在线加载（首次需联网）
      2. 从本地路径加载（离线部署）

    用法:
        extractor = EmbeddingExtractor(model_id="iic/speech_eres2netv2_sv_zh-cn_16k-common")
        embedding = extractor.extract("audio.wav")              # 从文件
        embedding = extractor.extract_from_waveform(waveform, sr)  # 从波形
    """

    def __init__(
        self,
        model_id: str = "iic/speech_eres2netv2_sv_zh-cn_16k-common",
        local_dir: Optional[str] = None,
        device: str = "cpu",
        embedding_dim: int = 192,
    ):
        self.model_id = model_id
        self.local_dir = local_dir
        self.device = device
        self.embedding_dim = embedding_dim
        self._model = None
        self._loaded = False

    def _normalize_feature_tensor(self, feat: torch.Tensor) -> torch.Tensor:
        """将 FBank 输出统一整理为模型所需的 [B, T, F] 形状。"""
        if feat.dim() == 1:
            feat = feat.unsqueeze(0).unsqueeze(0)
        elif feat.dim() == 2:
            feat = feat.unsqueeze(0)
        elif feat.dim() != 3:
            raise RuntimeError(f"不支持的特征张量维度: {tuple(feat.shape)}")

        if feat.shape[2] == 80:
            return feat.contiguous()

        if feat.shape[1] == 80:
            return feat.transpose(1, 2).contiguous()

        raise RuntimeError(f"无法识别的 FBank 特征形状: {tuple(feat.shape)}")

    def load(self) -> None:
        """加载模型（延迟加载，首次调用 extract 时自动触发）"""
        if self._loaded:
            return

        logger.info(f"加载嵌入模型: {self.model_id} (device={self.device})")

        try:
            # 优先尝试 3D-Speaker 原生加载方式
            self._load_via_speakerlab()
        except ImportError:
            logger.info("speakerlab 未安装，尝试 ModelScope Pipeline 方式加载")
            self._load_via_modelscope()

        self._loaded = True
        logger.info("嵌入模型加载完成")

    def _load_via_speakerlab(self) -> None:
        """通过 3D-Speaker speakerlab 加载"""
        from speakerlab.process.processor import FBank
        from speakerlab.utils.builder import dynamic_import

        import_path, model_kwargs = self._resolve_speakerlab_model_spec()
        model_cls = dynamic_import(import_path)
        self._feature_extractor = FBank(80, sample_rate=16000, mean_nor=True)

        # 加载权重
        model_path = self._resolve_model_path()
        self._model = model_cls(**model_kwargs)
        state_dict = torch.load(model_path, map_location=self.device)
        self._model.load_state_dict(state_dict)
        self._model.to(self.device)
        self._model.eval()

    def _resolve_speakerlab_model_spec(self) -> tuple[str, dict[str, int]]:
        """根据 ModelScope ID 映射到 speakerlab 内部模型定义。"""
        if "eres2netv2" in self.model_id:
            return (
                "speakerlab.models.eres2net.ERes2NetV2.ERes2NetV2",
                {"feat_dim": 80, "embedding_size": self.embedding_dim},
            )
        if "eres2net" in self.model_id:
            return (
                "speakerlab.models.eres2net.ERes2Net_huge.ERes2Net",
                {"feat_dim": 80, "embedding_size": self.embedding_dim},
            )
        if "campplus" in self.model_id:
            return (
                "speakerlab.models.campplus.DTDNN.CAMPPlus",
                {"feat_dim": 80, "embedding_size": self.embedding_dim},
            )
        raise ValueError(f"不支持的模型: {self.model_id}")

    def _load_via_modelscope(self) -> None:
        """通过 ModelScope Pipeline 加载（兼容方案）"""
        from modelscope.pipelines import pipeline as ms_pipeline

        kwargs = {"task": "speaker-verification", "model": self.model_id}
        if self.local_dir and os.path.isdir(self.local_dir):
            kwargs["model"] = self.local_dir
        if self.device.startswith("cuda"):
            kwargs["device"] = "gpu"
        else:
            kwargs["device"] = "cpu"

        self._model = ms_pipeline(**kwargs)
        self._feature_extractor = None  # ModelScope Pipeline 内部处理

    def _resolve_model_path(self) -> str:
        """查找本地模型权重文件"""
        if self.local_dir and os.path.isdir(self.local_dir):
            # 查找 .pt / .bin / .pth / .ckpt 文件
            for ext in [".pt", ".bin", ".pth", ".ckpt"]:
                for f in Path(self.local_dir).rglob(f"*{ext}"):
                    return str(f)
        raise FileNotFoundError(
            f"模型权重未找到: local_dir={self.local_dir}, "
            f"请先下载: modelscope download --model {self.model_id}"
        )

    @torch.no_grad()
    def extract(self, audio_path: str) -> np.ndarray:
        """
        从音频文件提取说话人嵌入向量。

        Args:
            audio_path: 音频文件路径

        Returns:
            L2 归一化后的嵌入向量, shape: (embedding_dim,)
        """
        self.load()

        if hasattr(self._model, "__call__") and not isinstance(self._model, torch.nn.Module):
            # ModelScope Pipeline 方式
            result = self._model(audio_path)
            if isinstance(result, dict) and "spk_embedding" in result:
                emb = np.array(result["spk_embedding"], dtype=np.float32)
            elif isinstance(result, np.ndarray):
                emb = result.astype(np.float32)
            else:
                raise RuntimeError(f"不支持的 Pipeline 返回类型: {type(result)}")
        else:
            # 原生模型方式
            from src.utils import load_audio
            waveform, sr = load_audio(audio_path, target_sr=16000, mono=True)
            emb = self.extract_from_waveform(waveform, sr)
            return emb  # 已归一化

        # L2 归一化
        norm = np.linalg.norm(emb)
        if norm > 0:
            emb = emb / norm

        return emb.flatten()

    @torch.no_grad()
    def extract_from_waveform(
        self,
        waveform: np.ndarray,
        sample_rate: int = 16000,
    ) -> np.ndarray:
        """
        从波形数组提取嵌入向量。

        Args:
            waveform: shape (num_samples,), float32
            sample_rate: 采样率

        Returns:
            L2 归一化后的嵌入向量, shape: (embedding_dim,)
        """
        self.load()

        if hasattr(self._model, "__call__") and not isinstance(self._model, torch.nn.Module):
            # ModelScope Pipeline: 需先保存到临时文件
            from src.utils import save_temp_audio
            tmp_path = save_temp_audio(waveform, sample_rate)
            try:
                return self.extract(tmp_path)
            finally:
                os.unlink(tmp_path)

        # 原生模型方式: 提取 FBank 特征 → 模型推理
        wav_tensor = torch.from_numpy(waveform).float().unsqueeze(0).to(self.device)
        feat = self._feature_extractor(wav_tensor)
        feat = self._normalize_feature_tensor(feat)
        emb = self._model(feat)

        emb = emb.cpu().numpy().flatten().astype(np.float32)
        norm = np.linalg.norm(emb)
        if norm > 0:
            emb = emb / norm
        return emb

    def cosine_similarity(self, emb_a: np.ndarray, emb_b: np.ndarray) -> float:
        """计算两个嵌入向量的余弦相似度"""
        dot = np.dot(emb_a.flatten(), emb_b.flatten())
        norm_a = np.linalg.norm(emb_a)
        norm_b = np.linalg.norm(emb_b)
        if norm_a == 0 or norm_b == 0:
            return 0.0
        return float(dot / (norm_a * norm_b))

    def export_onnx(self, output_path: str) -> None:
        """导出为 ONNX 格式（用于 Triton 等推理服务器）"""
        self.load()
        if isinstance(self._model, torch.nn.Module):
            dummy_input = torch.randn(1, 200, 80).to(self.device)
            torch.onnx.export(
                self._model,
                dummy_input,
                output_path,
                input_names=["features"],
                output_names=["embedding"],
                dynamic_axes={"features": {1: "time"}, "embedding": {0: "batch"}},
            )
            logger.info(f"ONNX 导出完成: {output_path}")
        else:
            raise RuntimeError("ModelScope Pipeline 模式不支持 ONNX 导出")
