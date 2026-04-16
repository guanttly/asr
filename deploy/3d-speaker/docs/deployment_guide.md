# Speaker Analysis Service 部署指南

## 1. 部署架构

```
┌──────────── 客户局域网 ────────────┐
│                                    │
│   客户端 / 浏览器                    │
│       │ HTTP (POST audio)          │
│       ▼                            │
│   ┌──────────────────────┐         │
│   │  说话人分离服务        │         │
│   │  :8100               │         │
│   │                      │         │
│   │  FastAPI + 3D-Speaker │         │
│   │  (CPU / GPU)         │         │
│   └──────────┬───────────┘         │
│              │                     │
│   ┌──────────▼───────────┐         │
│   │  声纹数据库 + 模型    │         │
│   │  (本地文件系统)       │         │
│   └──────────────────────┘         │
│                                    │
│   完全离线，无外网依赖              │
└────────────────────────────────────┘
```

## 2. 硬件要求

| 组件 | 最低配置 | 推荐配置 |
|------|---------|---------|
| CPU | 4 核 | 8 核+ |
| 内存 | 8 GB | 16 GB+ |
| GPU | 不需要（CPU 可运行） | NVIDIA GPU（加速嵌入提取） |
| 存储 | 2 GB（模型 + 服务） | 10 GB+（含声纹库和音频备份） |

## 3. 部署步骤

### 方式一：Docker 部署（推荐）

```bash
# 1. 将整个部署目录和导出的离线镜像包拷贝到目标服务器
cd speaker-analysis-service

# 2. 加载 Docker 镜像
./build.sh import

# 3. 启动服务
./build.sh start

# 4. 验证
curl http://localhost:8100/api/v1/health
```

说明：`./build.sh export` 只导出 Docker 镜像 tar.gz；因此离线部署时仍需要把当前目录下的 `build.sh`、`docker-compose.yml`、`config/` 等文件一并带到目标服务器。

说明：如果部署环境与打包环境分离，服务器侧需要准备完整的 `./models` 目录，并通过 Compose 挂载到容器内 `/app/models`。除了 `eres2netv2`、`campplus`、`fsmn_vad` 外，还必须包含 `native_cache`，否则 speakerlab 原生 diarization 会因为缺少 `campplus_cn_en_common.pt` 而回退兼容模式。

### 方式二：裸机部署

```bash
# 1. 安装依赖与原生 speakerlab
make init

# 2. 启动
make serve
```

说明：`make init` 会优先使用 `wheels/` 下的离线 wheel；若 wheel 不存在，则自动拉取 3D-Speaker 源码并注册到当前 Python 环境。完全离线且不使用 wheel 时，可先准备源码目录，再执行 `SPEAKERLAB_SOURCE=/path/to/3D-Speaker make init`。

说明：脚本默认先尝试国内镜像 `https://gitcode.com/mirrors/modelscope/3D-Speaker.git`，再回退 GitHub。如果部署机必须使用指定镜像，可在执行前设置 `SPEAKERLAB_REPO_LIST`，例如 `SPEAKERLAB_REPO_LIST="https://your-mirror/3D-Speaker.git https://gitcode.com/mirrors/modelscope/3D-Speaker.git" make init`。

说明：基础依赖里已包含 ModelScope pipeline 的运行时依赖，FSMN-VAD 与 speakerlab 原生音频分离无需在首次请求时再动态补装 Python 包。

如果需要为完全离线服务器准备模型，请在联网环境执行一次：

```bash
./build.sh download-models
```

该命令会同时下载：

- `models/eres2netv2`
- `models/campplus`
- `models/fsmn_vad`
- `models/native_cache`

然后将整个 `models/` 目录同步到目标服务器。

### 方式三：GPU 加速

修改 `config/settings.yaml`:

```yaml
models:
  embedding:
    device: "cuda:0"    # 改为 GPU
```

Docker 方式默认读取根目录 `docker-compose.yml`。如需 GPU，请设置 `DEVICE=cuda:0 ./build.sh start`。

如果希望宿主机只分配第 2 张 GPU 给容器使用，应在 Compose 中把可见 GPU 限制为宿主机 GPU 2，例如 `NVIDIA_VISIBLE_DEVICES=2` 或 `device_ids: ["2"]`。此时容器内通常只会看到 1 张卡，因此服务内部仍应使用 `cuda:0`，而不是 `cuda:2`。

## 3.1 模型目录检查

部署前建议在服务器上确认以下目录结构：

```bash
models/
├── eres2netv2/
├── campplus/
├── fsmn_vad/
└── native_cache/
```

其中 `native_cache/` 下面至少应包含 `campplus_cn_en_common.pt`。如果缺少这个文件，原生 diarization 不会再请求时临时下载，而是直接回退到兼容模式。

## 4. API 使用示例

### 4.1 声纹注册

```bash
# 为"张三"注册声纹（上传 15~30 秒清晰语音）
curl -X POST http://localhost:8100/api/v1/voiceprint/enroll \
  -F "file=@zhangsan_voice.wav" \
  -F "speaker_name=张三" \
  -F "department=技术部"
```

### 4.2 说话人分离（匿名）

```bash
# 上传会议音频，返回匿名标签
curl -X POST http://localhost:8100/api/v1/diarize \
  -F "file=@meeting.wav" \
  -F "min_speakers=2" \
  -F "max_speakers=6"
```

### 4.3 语音活动检测（VAD）

```bash
curl -X POST http://localhost:8100/api/v1/vad \
  -F "file=@meeting.wav"
```

### 4.4 说话人分离 + 身份识别

```bash
# 上传会议音频，自动匹配已注册声纹
curl -X POST http://localhost:8100/api/v1/diarize-identify \
  -F "file=@meeting.wav"
```

### 4.5 响应示例

```json
{
  "task_id": "a1b2c3d4",
  "audio_duration": 300.5,
  "num_speakers": 3,
  "segments": [
    {
      "speaker_id": "张三",
      "start_time": 0.5,
      "end_time": 15.2,
      "duration": 14.7,
      "confidence": 0.93
    },
    {
      "speaker_id": "李四",
      "start_time": 15.5,
      "end_time": 28.0,
      "duration": 12.5,
      "confidence": 0.87
    },
    {
      "speaker_id": "speaker_2",
      "start_time": 28.3,
      "end_time": 45.0,
      "duration": 16.7,
      "confidence": null
    }
  ],
  "speaker_summary": [
    {"speaker_id": "张三", "total_duration": 120.5, "percentage": 40.1, "voiceprint_matched": true},
    {"speaker_id": "李四", "total_duration": 95.3, "percentage": 31.7, "voiceprint_matched": true},
    {"speaker_id": "speaker_2", "total_duration": 84.7, "percentage": 28.2, "voiceprint_matched": false}
  ]
}
```

## 5. 与 Qwen3-ASR 集成

本服务输出的 RTTM 格式可直接与 Qwen3-ASR 转写结果按时间戳对齐：

```python
# 伪代码：合并分离结果与转写结果
for asr_segment in asr_results:
    mid_time = (asr_segment.start + asr_segment.end) / 2
    speaker = find_speaker_at_time(diarization_rttm, mid_time)
    print(f"[{speaker}] {asr_segment.text}")
```

## 6. 运维

| 操作 | 命令 |
|------|------|
| 查看服务状态 | `curl localhost:8100/api/v1/health` |
| 查看声纹数量 | `curl localhost:8100/api/v1/voiceprint/list` |
| 查看日志 | `./build.sh logs` 或 `tail -f logs/*.log` |
| 备份声纹库 | 备份 `data/` 目录 |
| 恢复声纹库 | 还原 `data/` 目录后重启服务 |
