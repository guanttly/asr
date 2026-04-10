# 说话人分离服务（Speaker Diarization Service）

基于阿里 **3D-Speaker CAM++** 模型的说话人分离服务，提供 RESTful API 接口，支持 Docker 容器化部署与完全离线运行。

---

## 目录结构

```
speaker-diarization-service/
├── app.py               # 服务主程序（FastAPI）
├── requirements.txt     # Python 依赖
├── Dockerfile           # Docker 构建文件（多阶段，含模型下载）
├── docker-compose.yml   # Docker Compose 编排
├── build.sh             # 一键构建/导出/导入/启动脚本
├── client_example.py    # 客户端调用示例（含 ASR 结果合并逻辑）
└── README.md            # 本文档
```

---

## 快速开始

### 在联网机器上构建

```bash
# 1. 构建镜像（自动下载模型，约 10-30 分钟）
chmod +x build.sh
./build.sh build

# 2. 导出为离线包
./build.sh export
# → 生成 speaker-diarization-offline.tar.gz
```

### 在离线服务器上部署

```bash
# 3. 将 tar.gz 和本目录文件拷贝到目标服务器后：
./build.sh import

# 4. 启动服务
./build.sh start

# 5. 验证
./build.sh test

# 6. 测试音频分离
./build.sh test /path/to/meeting.wav
```

默认镜像固定使用 CUDA 12.4 对应的 PyTorch 轮子，目的是避免 `pip install torch` 随时间拉到过新的版本，导致容器内 CUDA 运行时高于宿主机驱动能力。

如果目标机器驱动偏老，优先处理顺序如下：

1. 升级宿主机 NVIDIA 驱动。
2. 如果驱动暂时不能升级，则在构建时改用更低版本的 PyTorch CUDA 轮子。
3. 如果只追求稳定可用，直接按 CPU 模式部署。

示例：显式指定 PyTorch 版本/轮子源构建

```bash
docker build \
  --build-arg TORCH_VERSION=2.5.1 \
  --build-arg TORCHAUDIO_VERSION=2.5.1 \
  --build-arg PYTORCH_INDEX_URL=https://download.pytorch.org/whl/cu124 \
  -t speaker-diarization:1.0.0 .
```

---

## API 接口

服务启动后，可通过浏览器访问 `http://<服务器IP>:8080/docs` 查看交互式 API 文档。

### GET /health — 健康检查

```bash
curl http://localhost:8080/health
```

```json
{
  "status": "ok",
  "model_loaded": true,
  "device": "cpu",
  "diarization_model": "damo/speech_campplus_speaker-diarization_common"
}
```

### POST /diarize — 说话人分离

```bash
curl -X POST http://localhost:8080/diarize \
  -F "file=@meeting.wav"
```

可选参数：`num_speakers=3`（预设说话人数量）

```json
{
  "task_id": "a1b2c3d4",
  "audio_duration": 120.5,
  "num_speakers": 3,
  "segments": [
    {"speaker": "spk_0", "start": 0.5, "end": 3.2, "duration": 2.7},
    {"speaker": "spk_1", "start": 3.5, "end": 8.1, "duration": 4.6},
    {"speaker": "spk_0", "start": 8.3, "end": 12.7, "duration": 4.4}
  ],
  "process_time": 2.35
}
```

### POST /diarize/detail — 详细分离（含声纹嵌入）

返回额外的声纹嵌入向量（192 维），可用于跨音频的说话人匹配。

---

## 与其他服务的集成

本服务在整体架构中的位置：

```
应用服务器（调度中心）
    ├── POST /diarize     → 本服务（CAM++, CPU, 端口 8080）
    ├── POST /transcribe  → ASR 服务（Qwen3-ASR, GPU, 端口 8000）
    ├── POST /align       → 时间戳对齐（ForcedAligner, GPU）
    └── POST /summarize   → NLP 摘要（Qwen3 LLM, GPU）
```

参考 `client_example.py` 了解完整的调用与合并流程。

---

## 配置项

通过环境变量配置，可在 `docker-compose.yml` 中修改：

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| DEVICE | cpu | 推理设备：`cpu` 或 `cuda:0` |
| PORT | 8080 | 服务端口 |
| WORKERS | 1 | uvicorn worker 数（建议 CPU 部署设为 1） |
| MAX_UPLOAD_MB | 500 | 最大上传文件大小（MB） |
| DIARIZATION_MODEL | damo/speech_campplus_speaker-diarization_common | 说话人分离模型 |
| SV_MODEL | damo/speech_campplus_sv_zh-cn_16k-common | 声纹提取模型 |

---

## 硬件要求

| 部署方式 | CPU | 内存 | 说明 |
|---|---|---|---|
| CPU 部署 | 4 核+ | 4GB+ | 推荐，CAM++ 模型轻量，CPU 即可满足 |
| GPU 部署 | — | 2GB 显存+ | 可选，加速处理但非必须 |

---

## 注意事项

- 如果日志里出现 “The NVIDIA driver on your system is too old”，含义通常不是模型有问题，而是容器内 PyTorch 对应的 CUDA 版本高于宿主机驱动所支持的版本。
- 首次启动需 60-120 秒加载模型，后续请求响应在秒级
- 音频会自动转为 16kHz 单声道 WAV（需容器内有 ffmpeg）
- 支持格式：WAV、MP3、M4A、AAC、FLAC、OGG
- 声纹相近的说话人在短音频中可能分离效果不理想，建议音频 > 30 秒
- 生产环境建议在反向代理（Nginx）后面运行
