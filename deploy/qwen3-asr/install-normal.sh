docker rm -f qwen3-asr-serve
docker run -d --gpus '"device=2"' --name qwen3-asr-serve \
    -p 10001:8000 \
    -e HF_ENDPOINT=https://hf-mirror.com \
    -v /data/ganttly/qwen3-asr/models/Qwen3-ASR-1.7B:/models/Qwen3-ASR-1.7B \
    --shm-size=4gb \
    qwenllm/qwen3-asr:latest \
    qwen-asr-serve /models/Qwen3-ASR-1.7B \
    --host 0.0.0.0 --port 8000 \
    --gpu-memory-utilization 0.45 \
    --max-model-len 8192
