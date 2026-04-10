# 3D-Speaker 说话人分离服务 —— 离线部署开发包

> 基于阿里 3D-Speaker 工具包的说话人分离 + 声纹注册匹配服务，面向局域网私有化部署。

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                    FastAPI 服务层                         │
│                                                         │
│  POST /api/v1/diarize          说话人分离（匿名标签）      │
│  POST /api/v1/diarize-identify 分离 + 声纹匹配（真实姓名） │
│  POST /api/v1/voiceprint/enroll   声纹注册               │
│  DELETE /api/v1/voiceprint/{id}   声纹删除               │
│  GET  /api/v1/voiceprint/list     声纹列表               │
│  GET  /api/v1/health              健康检查               │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                    核心引擎层                             │
│                                                         │
│  DiarizationEngine    说话人分离（VAD→分段→嵌入→聚类）     │
│  VoiceprintManager    声纹库管理（注册/删除/持久化）       │
│  SpeakerMatcher       身份匹配（余弦相似度比对）           │
│  EmbeddingExtractor   嵌入提取（ERes2NetV2 / CAM++）     │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                    存储层                                 │
│                                                         │
│  SQLite / PostgreSQL   声纹元数据                        │
│  本地文件系统           嵌入向量 + 注册音频备份             │
└─────────────────────────────────────────────────────────┘
```

## 目录结构

```
speaker-diarization-service/
├── build.sh                    # 一键构建打包脚本
├── README.md                   # 本文档
├── requirements.txt            # Python 依赖
├── Makefile                    # 常用操作快捷命令
├── config/
│   └── settings.yaml           # 服务配置文件
├── src/
│   ├── __init__.py
│   ├── server.py               # FastAPI 主服务入口
│   ├── engine.py               # 说话人分离引擎
│   ├── embedding.py            # 嵌入提取器
│   ├── voiceprint.py           # 声纹库管理
│   ├── matcher.py              # 说话人身份匹配
│   ├── models.py               # Pydantic 数据模型
│   └── utils.py                # 工具函数（音频预处理等）
├── scripts/
│   ├── download_models.sh      # 模型权重离线下载
│   ├── export_onnx.sh          # 导出 ONNX 格式（可选）
│   └── init_db.py              # 数据库初始化
├── docker/
│   ├── Dockerfile              # Docker 镜像构建
│   └── docker-compose.yaml     # 编排配置
├── tests/
│   ├── test_diarization.py     # 分离功能测试
│   ├── test_voiceprint.py      # 声纹注册匹配测试
│   └── test_api.py             # API 接口测试
├── docs/
│   └── deployment_guide.md     # 部署指南
└── models/                     # 模型权重存放目录（构建时填充）
    └── .gitkeep
```

## 快速开始

```bash
# 1. 构建离线部署包（含模型下载 + Docker 镜像）
bash build.sh --download-models --build-docker

# 2. 仅构建（模型已手动下载到 models/ 目录）
bash build.sh --build-docker

# 3. 本地开发运行
pip install -r requirements.txt
python -m src.server

# 4. Docker 部署
cd docker && docker-compose up -d
```

## 模型说明

| 模型 | ModelScope ID | 用途 | 大小 |
|------|--------------|------|------|
| ERes2NetV2 | iic/speech_eres2netv2_sv_zh-cn_16k-common | 说话人嵌入（默认） | ~70MB |
| CAM++ | iic/speech_campplus_sv_zh-cn_16k-common | 说话人嵌入（轻量） | ~30MB |
| FSMN-VAD | iic/speech_fsmn_vad_zh-cn-16k-common-pytorch | 语音活动检测 | ~40MB |

## 许可证

- 3D-Speaker: Apache License 2.0
- 本服务代码: Apache License 2.0
