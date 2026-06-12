.PHONY: all clean
.PHONY: build-backend build-gateway build-asr-api build-admin-api build-nlp-api build-frontend
.PHONY: dev-backend dev-gateway dev-asr-api dev-admin-api dev-nlp-api dev-frontend
.PHONY: lint test test-backend test-openapi-real test-frontend-unit test-frontend-e2e
.PHONY: docker-up docker-down docker-build
.PHONY: release-jusha-asr release-jusha-all release-jusha-business release-jusha-models
.PHONY: release-fama-business install-fama-business
.PHONY: generate-radiology-term-excel sync-term-catalog generate-radiology-rules-excel sync-rules-catalog

# ============================================================
# 语音转写系统 Monorepo Makefile
# ============================================================

# 常用命令：
#   make build-backend                      # 编译全部后端服务到 bin/
#   make build-frontend                     # 构建前端静态资源
#   make dev-gateway                        # 本地启动 gateway
#   make dev-admin-api                      # 本地启动 admin-api
#   make dev-asr-api                        # 本地启动 asr-api
#   make dev-nlp-api                        # 本地启动 nlp-api
#   make dev-frontend                       # 启动前端开发服务器
#   make lint                               # 执行前后端静态检查
#   make docker-up                          # 启动 deploy/docker compose 环境
#
# Jusha ASR 发布示例：
#   make release-jusha-all VERSION=0.8.6 OUTPUT_DIR=./dist
#   make release-jusha-business VERSION=0.8.6 OUTPUT_DIR=./dist DRY_RUN=1
#   make release-jusha-models VERSION=0.8.6 OUTPUT_DIR=./dist SPEAKER_ARGS="--allow-incomplete-native-cache"
#   make release-jusha-models VERSION=0.9.3 OUTPUT_DIR=./dist JUSHA_ASR_PART_SIZE=2g
#
# release-jusha-* 发布形态：
#   business  产物为 jusha-asr-business-<version>.run 和 .run.partNNN 分包
#   models    产物为 jusha-asr-models-<version>.run，内部包含 jusha-asr-asr-<version>.run
#             和 jusha-asr-speaker-<version>.run；speaker 不是顶层 mode，而是 models 内部组件包
#   all       产物为 jusha-asr-all-<version>.run，内部包含 business + models
#   如需单独构建 3D-Speaker 离线包，请使用 deploy/3d-speaker/scripts/build-offline-run.sh
#
# release-jusha-* 入口参数：
#   VERSION             统一发布版本号
#   OUTPUT_DIR          发布产物输出目录
#   JUSHA_MODE          all|business|models，仅 release-jusha-asr 读取，默认 all
#   BUSINESS_ARGS       额外透传给业务服务打包脚本；
#                       优先使用下列具名参数，只有脚本暂未暴露的参数再走这里
#   SPEAKER_ARGS        额外透传给 deploy/3d-speaker/scripts/build-offline-run.sh，例如 "--allow-incomplete-native-cache"
#   KEEP_WORK=1         保留统一发布脚本临时目录
#
# release-jusha-* 常用具名参数：
#   SERVER_HOST         业务服务包默认访问地址和 TLS 证书主机名
#   HTTP_PORT / HTTPS_PORT
#   ADMIN_USERNAME / ADMIN_PASSWORD / ADMIN_DISPLAY_NAME
#   MYSQL_PASSWORD / JWT_SECRET
#   ASR_SERVICE_URL / SPEAKER_SERVICE_URL
#   DESKTOP_VERSION     业务服务包内桌面端安装包版本；不传则读取 desktop/package.json
#   DESKTOP_INSTALLER   直接复用现成 Tauri Win10/11 安装包
#   DESKTOP_ELECTRON_INSTALLER 直接复用现成 Win7 兼容版安装包
#   SKIP_ELECTRON=1     跳过 Win7 兼容版打包
#   DRY_RUN=1           跳过 Docker 镜像和桌面端自动构建
#
# release-jusha-* 常用环境变量：
#   JUSHA_ASR_PART_SIZE       传给 split -b 的分包大小；支持纯字节数或 k/m/g 后缀，
#                             例如 524288000、500m、2g；默认 500m
#   JUSHA_ASR_KEEP_ARCHIVE=1  保留中间 .tar.gz，默认只保留 .run 和分包
#   SOURCE_DIR / CONTAINER / SOURCE_IMAGE  Qwen3-ASR 来源
#   GPU_COUNT / GPU_DEVICE_IDS             Qwen3-ASR GPU 配置
#   SA_IMAGE_NAME / SA_CONTAINER_NAME      3D-Speaker 镜像名 / 容器名，默认 jusha-asr-speaker
#   SA_GPU_ID                              3D-Speaker GPU ID，默认 0
#   默认网络：jusha-asr；默认端口：业务 HTTP 9855 / HTTPS 9856，ASR 9851，3D-Speaker 9852
#
# release-jusha-* 组合示例：
#   make release-jusha-all VERSION=0.8.6 OUTPUT_DIR=./dist SERVER_HOST=192.168.40.221
#   make release-jusha-business VERSION=0.9.4 JUSHA_ASR_PART_SIZE=2g OUTPUT_DIR=/data/ganttly/releases/fama/0.9.4 SERVER_HOST=192.168.40.221 DRY_RUN=1
#   make release-jusha-models VERSION=0.9.3 OUTPUT_DIR=/data/ganttly/releases/fama/0.9.3 JUSHA_ASR_PART_SIZE=2g
#   make release-jusha-models VERSION=0.9.3 OUTPUT_DIR=/data/ganttly/releases/fama/0.9.3 JUSHA_ASR_PART_SIZE=524288000
#
# 192.168.40.221 /data/ganttly/fama 标准流程：
#   make release-fama-business VERSION=0.10.3 JUSHA_ASR_PART_SIZE=2g
#   make install-fama-business VERSION=0.10.3


JUSHA_MODE_VALUE = $(or $(JUSHA_MODE),all)
FAMA_RELEASE_ROOT ?= /data/ganttly/releases/fama
FAMA_INSTALL_ROOT ?= /data/ganttly
FAMA_SERVER_HOST ?= 192.168.40.221

sh_quote = '$(subst ','\'',$(1))'

define append_arg
if [ -n $(call sh_quote,$($(2))) ]; then set -- "$$@" $(1) $(call sh_quote,$($(2))); fi;
endef

define append_flag
if [ -n $(call sh_quote,$($(2))) ]; then set -- "$$@" $(1); fi;
endef

define append_business_arg
if [ -n $(call sh_quote,$($(2))) ]; then set -- "$$@" --business-arg $(1) --business-arg $(call sh_quote,$($(2))); fi;
endef

define append_business_flag
if [ -n $(call sh_quote,$($(2))) ]; then set -- "$$@" --business-arg $(1); fi;
endef

append_business_args = $(foreach ARG,$(BUSINESS_ARGS),set -- "$$@" --business-arg $(call sh_quote,$(ARG));)
append_speaker_args = $(foreach ARG,$(SPEAKER_ARGS),set -- "$$@" --speaker-arg $(call sh_quote,$(ARG));)

# --- Backend ---

build-backend:
	cd backend && go build -o ../bin/gateway ./cmd/gateway
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

build-gateway:
	cd backend && go build -o ../bin/gateway ./cmd/gateway

build-asr-api:
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api

build-admin-api:
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api

build-nlp-api:
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

dev-backend: dev-gateway

dev-gateway:
	cd backend && go run ./cmd/gateway

dev-asr-api:
	cd backend && go run ./cmd/asr-api

dev-admin-api:
	cd backend && go run ./cmd/admin-api

dev-nlp-api:
	cd backend && go run ./cmd/nlp-api

# --- Frontend ---
dev-frontend:
	cd frontend && pnpm dev

build-frontend:
	cd frontend && pnpm build

# --- Quality ---
lint:
	cd frontend && pnpm lint
	cd backend && golangci-lint run ./...

test-backend:
	cd backend && go test ./...

# 针对运行中的网关执行 OpenAPI 真实可用性测试。
test-openapi-real:
	@set --; \
	$(call append_arg,--base-url,OPENAPI_BASE_URL) \
	$(call append_arg,--admin-username,OPENAPI_ADMIN_USERNAME) \
	$(call append_arg,--admin-password,OPENAPI_ADMIN_PASSWORD) \
	$(call append_arg,--audio-file,OPENAPI_AUDIO_FILE) \
	$(call append_flag,--skip-asr-audio,OPENAPI_SKIP_ASR_AUDIO) \
	$(call append_flag,--full-stream,OPENAPI_FULL_STREAM) \
	$(call append_flag,--keep-apps,OPENAPI_KEEP_APPS) \
	cd backend && go run ./cmd/openapi-real-test "$$@"

test-frontend-unit:
	cd frontend && pnpm test:unit

test-frontend-e2e:
	cd frontend && pnpm test:e2e

test: test-backend test-frontend-unit test-frontend-e2e

# --- Docker ---
docker-up:
	cd deploy && docker compose up -d

docker-down:
	cd deploy && docker compose down

docker-build:
	cd deploy && docker compose build

# 统一 Jusha ASR 发布入口，支持 all / business / models 三种发布形态。
release-jusha-asr:
	@set -- --mode $(call sh_quote,$(JUSHA_MODE_VALUE)); \
	$(call append_arg,--version,VERSION) \
	$(call append_arg,--output-dir,OUTPUT_DIR) \
	if [ $(call sh_quote,$(JUSHA_MODE_VALUE)) != 'models' ]; then \
		$(append_business_args) \
		$(call append_business_arg,--server-host,SERVER_HOST) \
		$(call append_business_arg,--http-port,HTTP_PORT) \
		$(call append_business_arg,--https-port,HTTPS_PORT) \
		$(call append_business_arg,--admin-username,ADMIN_USERNAME) \
		$(call append_business_arg,--admin-password,ADMIN_PASSWORD) \
		$(call append_business_arg,--admin-display-name,ADMIN_DISPLAY_NAME) \
		$(call append_business_arg,--mysql-password,MYSQL_PASSWORD) \
		$(call append_business_arg,--jwt-secret,JWT_SECRET) \
		$(call append_business_arg,--asr-service-url,ASR_SERVICE_URL) \
		$(call append_business_arg,--speaker-service-url,SPEAKER_SERVICE_URL) \
		$(call append_business_arg,--desktop-version,DESKTOP_VERSION) \
		$(call append_business_arg,--desktop-installer,DESKTOP_INSTALLER) \
		$(call append_business_arg,--desktop-electron-installer,DESKTOP_ELECTRON_INSTALLER) \
		$(call append_business_flag,--skip-electron,SKIP_ELECTRON) \
		$(call append_business_flag,--dry-run,DRY_RUN) \
	fi; \
	$(append_speaker_args) \
	$(call append_flag,--keep-work,KEEP_WORK) \
	bash deploy/jusha-asr/build-release.sh "$$@"

# 生成大包：业务服务 + ASR + 3D-Speaker。
release-jusha-all:
	$(MAKE) release-jusha-asr JUSHA_MODE=all

# 只生成业务服务包。
release-jusha-business:
	$(MAKE) release-jusha-asr JUSHA_MODE=business

release-fama-business:
	@test -n "$(VERSION)" || { echo "VERSION is required, for example: make release-fama-business VERSION=0.10.3" >&2; exit 2; }
	$(MAKE) release-jusha-business VERSION="$(VERSION)" OUTPUT_DIR="$(FAMA_RELEASE_ROOT)/$(VERSION)" SERVER_HOST="$(FAMA_SERVER_HOST)"

install-fama-business:
	@test -n "$(VERSION)" || { echo "VERSION is required, for example: make install-fama-business VERSION=0.10.3" >&2; exit 2; }
	@cd "$(FAMA_RELEASE_ROOT)/$(VERSION)" && \
		test -f "jusha-asr-business-$(VERSION).run" && \
		test -f "jusha-asr-business-$(VERSION).run.part001" && \
		ASR_RUN_TARGET_DIR="$(FAMA_INSTALL_ROOT)" sh "./jusha-asr-business-$(VERSION).run"

# 只生成模型服务组合包；顶层产物名为 models，内部包含 ASR 与 3D-Speaker 两个安装包。
release-jusha-models:
	$(MAKE) release-jusha-asr JUSHA_MODE=models

# --- Utilities ---
# 根据影像科 markdown 生成随目录发布的术语 Excel。
generate-radiology-term-excel:
	cd backend && go run ./cmd/term-catalog-xlsx -source ../docs/terms -scope radiology -out ../docs/terms/radiology/影像科术语.xlsx

# 同步术语库快照到 backend embed 目录（原始 docs/terms 保持不变）。
sync-term-catalog: generate-radiology-term-excel
	rm -rf backend/internal/application/catalog/terms
	mkdir -p backend/internal/application/catalog/terms
	cp -a docs/terms/. backend/internal/application/catalog/terms/
	cd backend && go test ./internal/application/catalog/... -run TestEmbeddedCatalogParsesCleanly

# 根据影像科规则 markdown 生成随目录发布的规则 Excel。
generate-radiology-rules-excel:
	cd backend && go run ./cmd/rules-catalog-xlsx -source ../docs/rules -scope radiology -out ../docs/rules/radiology/影像规则.xlsx

# 同步规则库快照到 backend embed 目录（原始 docs/rules 保持不变）。
sync-rules-catalog: generate-radiology-rules-excel
	rm -rf backend/internal/application/rulescatalog/rules
	mkdir -p backend/internal/application/rulescatalog/rules
	cp -a docs/rules/. backend/internal/application/rulescatalog/rules/
	cd backend && go test ./internal/application/rulescatalog/... -run TestEmbeddedRulesCatalogParsesCleanly

# 清理常见构建产物与前端缓存。
clean:
	rm -rf bin/
	cd frontend && rm -rf dist node_modules/.vite

# 一次性构建后端与前端。
all: build-backend build-frontend
