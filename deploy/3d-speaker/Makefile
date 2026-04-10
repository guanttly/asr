.PHONY: help dev serve test clean init docker docker-up docker-down download-models pack

PYTHON := python3
VENV := .venv
PIP := $(VENV)/bin/pip
PY := $(VENV)/bin/python
UVICORN := $(VENV)/bin/uvicorn

help: ## 显示帮助
	@echo "═══════════════════════════════════════════════════"
	@echo " Speaker Analysis Service — 开发命令"
	@echo "═══════════════════════════════════════════════════"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── 环境 ───

venv: ## 创建虚拟环境并安装依赖
	$(PYTHON) -m venv $(VENV)
	$(PIP) install --upgrade pip -q
	$(PIP) install -r requirements.txt -q
	bash scripts/install_speakerlab.sh $(PIP)
	@echo "虚拟环境就绪: source $(VENV)/bin/activate"

init: venv ## 初始化项目（虚拟环境 + 数据目录）
	$(PY) scripts/init_db.py

# ─── 运行 ───

dev: init ## 开发模式启动（热重载）
	$(UVICORN) src.server:app --host 0.0.0.0 --port 8100 --reload

serve: init ## 生产模式启动
	$(UVICORN) src.server:app --host 0.0.0.0 --port 8100 --workers 1

# ─── 测试 ───

test: ## 运行测试
	$(PY) -m pytest tests/ -v --tb=short

test-api: ## 运行 API 测试
	$(PY) -m pytest tests/test_api.py -v

test-voiceprint: ## 运行声纹测试
	$(PY) -m pytest tests/test_voiceprint.py -v

# ─── 模型 ───

download-models: ## 下载模型权重（需联网）
	bash scripts/download_models.sh

# ─── Docker ───

docker: ## 构建 Docker 镜像
	bash build.sh build

docker-up: ## Docker Compose 启动
	bash build.sh start

docker-down: ## Docker Compose 停止
	bash build.sh stop

docker-logs: ## 查看 Docker 日志
	bash build.sh logs

# ─── 打包 ───

pack: ## 构建离线部署包
	bash build.sh build
	bash build.sh export

pack-full: ## 完整构建（含模型下载）
	bash build.sh download-models
	bash build.sh build
	bash build.sh export

# ─── 清理 ───

clean: ## 清理临时文件
	rm -rf data/temp/*
	rm -rf __pycache__ src/__pycache__ tests/__pycache__
	rm -rf .pytest_cache
	find . -name "*.pyc" -delete

clean-all: clean ## 清理所有（含虚拟环境和构建产物）
	rm -rf $(VENV) dist/
