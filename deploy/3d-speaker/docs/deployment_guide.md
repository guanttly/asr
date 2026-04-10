# 部署指南

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
# 1. 解压离线部署包
tar -xzf speaker-diarization-service-1.0.0-offline.tar.gz
cd speaker-diarization-service-1.0.0

# 2. 加载 Docker 镜像
docker load -i docker/speaker-diarization-service-1.0.0.tar

# 3. 启动服务
cd docker && docker-compose up -d

# 4. 验证
curl http://localhost:8100/api/v1/health
```

### 方式二：裸机部署

```bash
# 1. 解压并安装
tar -xzf speaker-diarization-service-1.0.0-offline.tar.gz
cd speaker-diarization-service-1.0.0
bash install.sh

# 2. 启动
cd /opt/speaker-diarization-service
source .venv/bin/activate
make serve
```

### 方式三：GPU 加速

修改 `config/settings.yaml`:

```yaml
models:
  embedding:
    device: "cuda:0"    # 改为 GPU
```

Docker 方式需取消 `docker-compose.yaml` 中 GPU 部分的注释。

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

### 4.3 说话人分离 + 身份识别

```bash
# 上传会议音频，自动匹配已注册声纹
curl -X POST http://localhost:8100/api/v1/diarize-identify \
  -F "file=@meeting.wav"
```

### 4.4 响应示例

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
| 查看日志 | `docker-compose logs -f` 或 `tail -f logs/*.log` |
| 备份声纹库 | 备份 `data/` 目录 |
| 恢复声纹库 | 还原 `data/` 目录后重启服务 |
