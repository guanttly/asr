"""EmbeddingExtractor 单元测试。"""

from pathlib import Path

import torch

from src.embedding import EmbeddingExtractor


def test_resolve_model_path_accepts_ckpt(tmp_path: Path):
    model_file = tmp_path / "pretrained_eres2netv2.ckpt"
    model_file.write_bytes(b"checkpoint")

    extractor = EmbeddingExtractor(local_dir=str(tmp_path))

    assert extractor._resolve_model_path() == str(model_file)


def test_resolve_speakerlab_model_spec_for_eres2netv2():
    extractor = EmbeddingExtractor(model_id="iic/speech_eres2netv2_sv_zh-cn_16k-common")

    import_path, kwargs = extractor._resolve_speakerlab_model_spec()

    assert import_path == "speakerlab.models.eres2net.ERes2NetV2.ERes2NetV2"
    assert kwargs == {"feat_dim": 80, "embedding_size": 192}


def test_resolve_speakerlab_model_spec_for_campplus():
    extractor = EmbeddingExtractor(model_id="iic/speech_campplus_sv_zh-cn_16k-common")

    import_path, kwargs = extractor._resolve_speakerlab_model_spec()

    assert import_path == "speakerlab.models.campplus.DTDNN.CAMPPlus"
    assert kwargs == {"feat_dim": 80, "embedding_size": 192}


def test_normalize_feature_tensor_adds_batch_dim_for_2d_feature():
    extractor = EmbeddingExtractor()

    feat = torch.randn(200, 80)
    normalized = extractor._normalize_feature_tensor(feat)

    assert normalized.shape == (1, 200, 80)


def test_normalize_feature_tensor_transposes_time_feature_layout():
    extractor = EmbeddingExtractor()

    feat = torch.randn(2, 80, 200)
    normalized = extractor._normalize_feature_tensor(feat)

    assert normalized.shape == (2, 200, 80)


def test_normalize_feature_tensor_keeps_channel_first_layout():
    extractor = EmbeddingExtractor()

    feat = torch.randn(2, 200, 80)
    normalized = extractor._normalize_feature_tensor(feat)

    assert normalized.shape == (2, 200, 80)