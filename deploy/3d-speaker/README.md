# Speaker Analysis Service —— 离线部署开发包

> 基于 3D-Speaker + FSMN-VAD 的说话人分析服务，提供说话人分离、声纹注册匹配和独立语音活动检测接口，面向局域网私有化部署。

说明：目录仍保留为 deploy/3d-speaker，用于表达底层模型技术栈；对外交付名统一为 Speaker Analysis Service。

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│                    FastAPI 服务层                         │
│                                                         │
│  POST /api/v1/diarize          说话人分离（匿名标签）      │
│  POST /api/v1/diarize-identify 分离 + 声纹匹配（真实姓名） │
│  POST /api/v1/vad              语音活动检测               │
│  POST /api/v1/voiceprint/enroll   声纹注册               │
│  DELETE /api/v1/voiceprint/{id}   声纹删除               │
│  GET  /api/v1/voiceprint/list     声纹列表               │
│  GET  /api/v1/health              健康检查               │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                    核心引擎层                             │
│                                                         │
│  DiarizationEngine    说话人分离（VAD→分段→嵌入→聚类）     │
│  VoiceActivityDetector 独立 VAD 接口 + fallback VAD      │
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
├── build.sh                    # 构建/导出/导入/启动脚本（与 cam++ 风格统一）
├── README.md                   # 本文档
├── requirements.txt            # Python 依赖
├── Makefile                    # 常用操作快捷命令
├── Dockerfile                  # 生产镜像构建入口
├── docker-compose.yml          # Compose 启动入口
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
│   ├── vad.py                  # VAD 检测器
│   └── utils.py                # 工具函数（音频预处理等）
├── scripts/
│   ├── download_models.sh      # 模型权重离线下载
│   ├── install_speakerlab.sh   # 注册 wheel/源码并补齐原生依赖
│   └── init_db.py              # 数据库初始化
├── tests/
│   ├── test_diarization.py     # 分离功能测试
│   ├── test_voiceprint.py      # 声纹注册匹配测试
│   ├── test_api.py             # API 接口测试
│   └── test_vad.py             # VAD 单元测试
├── docs/
│   └── deployment_guide.md     # 部署指南
├── wheels/                     # 可选：speakerlab 离线 wheel
│   └── .gitkeep
└── models/                     # 模型权重存放目录（构建时填充）
    ├── eres2netv2/             # 业务层嵌入模型
    ├── campplus/               # 业务层轻量嵌入模型
    ├── fsmn_vad/               # 业务层 VAD 模型
    ├── native_cache/           # speakerlab 原生 diarization 额外缓存
    └── .gitkeep
```

## 快速开始

```bash
# 1. 下载模型（联网构建机执行一次）
./build.sh download-models

# 2. 构建镜像
./build.sh build

# 3. 导出离线镜像包
./build.sh export

# 4. 本地开发运行
make init
make serve

# 5. Docker 部署
./build.sh start
```

和 cam++ 一样，这套脚本统一使用 build / export / import / start / stop / test / logs 命令；不再使用旧的参数式构建入口。

`make init` 会优先安装 `wheels/` 下的 `speakerlab-*.whl`；如果没有 wheel，则自动拉取 3D-Speaker 源码并通过 `.pth` 注册到当前 Python 环境。

`./build.sh build` 在正式执行 `docker build` 之前，会先把 `requirements.txt` 里的 Python 依赖增量下载到 `wheels/runtime/`，并把 pip 缓存落到 `.cache/pip/`。如果中途因为网络抖动失败，重新执行构建时会优先复用这两处本地缓存，而不是把大体积 wheel 全部从头重下。

如果目标是 GPU 运行，构建阶段会把 `torch` / `torchaudio` 单独切到官方 PyTorch CUDA wheel 源处理，而不是继续走通用 PyPI 镜像去拉取 `nvidia-cusparse-cu12` 这类大依赖。默认使用 `cu121` 通道；如果目标环境需要别的 CUDA 轮子通道，可在构建时覆盖，例如 `TORCH_CUDA_CHANNEL=cu124 ./build.sh build`。

现在缓存分两段处理：`torch` / `torchaudio` 是第 7 步，普通运行时依赖是第 8 步。`.runtime-wheels-ready` 不再只是“存在即有效”，而是会记录当前非 torch 依赖集合的哈希；只要 `requirements.txt` 里的普通依赖发生变化，构建脚本就会自动判定旧标记失效并重新预下载，避免把过期缓存误当成完整离线包。

像 `funasr>=1.0.0` 这类包在阿里云、清华、北外、科大等常见国内 PyPI 镜像里版本并不完整，所以默认策略不再是走海外补充索引，而是在 `docker build` 之前先从国内 Git 镜像拉取 FunASR 源码，并在本地构建 `funasr-*.whl` 放进 `wheels/runtime/`。这样第 8 步仍然可以优先使用本地 wheel，不依赖出海网络。只有在你明确配置了 `RUNTIME_EXTRA_INDEX_URL` 时，普通依赖安装才会额外带补充索引。

源码拉取默认优先走国内镜像 `https://gitcode.com/mirrors/modelscope/3D-Speaker.git` 和 `https://gitcode.com/mirrors/modelscope/FunASR.git`，失败后再回退到 GitHub。若客户内网只允许特定镜像，可分别通过 `SPEAKERLAB_REPO_LIST`、`FUNASR_REPO_LIST` 覆盖候选地址。

基础 `requirements.txt` 还会一并安装 ModelScope pipeline 所需依赖，并显式包含 `funasr`。现在 VAD 的实际加载顺序是：先尝试 ModelScope pipeline；如果当前 `modelscope` 版本没有把 `voice_activity_detection` 注册出来，或者注册链路因为缺少音频组件失效，则自动切到 FunASR `AutoModel`；只有这两条链路都失败时，才回退到能量阈值模式。

注意：原生 diarization 除了 `models/eres2netv2`、`models/campplus`、`models/fsmn_vad` 这三类业务层模型外，还会额外依赖 `models/native_cache`。其中至少需要包含 `campplus_cn_en_common.pt`，否则服务会自动回退到兼容模式，不再在请求过程中临时下载模型。

完全离线的裸机场景如果没有 wheel，需要先准备一份 3D-Speaker 源码目录，然后执行：

```bash
SPEAKERLAB_SOURCE=/path/to/3D-Speaker make init
```

## 模型说明

| 模型 | ModelScope ID | 用途 | 大小 |
|------|--------------|------|------|
| ERes2NetV2 | iic/speech_eres2netv2_sv_zh-cn_16k-common | 说话人嵌入（默认） | ~70MB |
| CAM++ | iic/speech_campplus_sv_zh-cn_16k-common | 说话人嵌入（轻量） | ~30MB |
| FSMN-VAD | iic/speech_fsmn_vad_zh-cn-16k-common-pytorch | 语音活动检测 | ~40MB |
| Native diarization cache | iic/speech_campplus_sv_zh_en_16k-common_advanced | speakerlab 原生分离额外依赖 | ~30MB |

如果部署环境与打包环境分离，建议把服务器上的 `./models` 整体只读挂载到容器内的 `/app/models`。当前默认配置会把原生 diarization 的运行时 `MODELSCOPE_CACHE` 固定到可写目录 `/app/data/native_cache`，容器启动时会自动从 `/app/models/native_cache` 预热过去，避免 speakerlab 在只读模型目录上写缓存时报 `Read-only file system`。

## 服务命名

- 对外交付名建议使用 Speaker Analysis Service。
- 原因是当前职责已经覆盖 VAD、说话人分离、声纹注册与匹配，不再只是单一 diarization。
- 目录名保留 3d-speaker，是为了保留底层技术来源，不建议再把目录和交付名混用。

## API 概览

- POST /api/v1/vad：独立 VAD 接口，返回语音片段，可直接作为实时 ASR 切段依据。
- POST /api/v1/diarize：说话人分离，返回匿名标签。
- POST /api/v1/diarize-identify：说话人分离并尝试匹配已注册声纹。
- POST /api/v1/voiceprint/enroll：注册声纹。
- GET /api/v1/voiceprint/list：列出声纹库。
- GET /api/v1/health：健康检查。

## 许可证

- 3D-Speaker: Apache License 2.0
- 本服务代码: Apache License 2.0
