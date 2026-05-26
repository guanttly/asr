.PHONY: all clean
.PHONY: build-backend build-gateway build-asr-api build-admin-api build-nlp-api build-frontend
.PHONY: dev-backend dev-gateway dev-asr-api dev-admin-api dev-nlp-api dev-frontend
.PHONY: lint test test-backend test-openapi-real test-frontend-unit test-frontend-e2e
.PHONY: docker-up docker-down docker-build
.PHONY: release-all-in-one release-jusha-asr release-jusha-all release-jusha-business release-jusha-models
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
#   make release-jusha-all VERSION=0.8.6 OUTPUT_DIR=./dist
#   make release-jusha-business VERSION=0.8.6 OUTPUT_DIR=./dist DRY_RUN=1
#   make release-jusha-models VERSION=0.8.6 OUTPUT_DIR=./dist
#   make release-all-in-one VERSION=0.3.9 OUTPUT_DIR=./dist DRY_RUN=1
#
# release-jusha-* 可选参数：
#   VERSION             统一发布版本号
#   OUTPUT_DIR          发布产物输出目录
#   JUSHA_MODE          all|business|models，release-jusha-asr 使用，默认 all
#   BUSINESS_ARGS       透传给 deploy/all-in-one/scripts/build-release.sh，例如 "--dry-run --server-host 192.168.40.221"；优先使用下面的具名参数
#   SERVER_HOST         透传给业务服务包构建，用于客户端默认地址和 TLS 证书主机名
#   HTTP_PORT / HTTPS_PORT 透传给业务服务包构建，用于客户端默认端口
#   ADMIN_USERNAME / ADMIN_PASSWORD / ADMIN_DISPLAY_NAME 透传给业务服务包构建
#   MYSQL_PASSWORD / JWT_SECRET 透传给业务服务包构建
#   ASR_SERVICE_URL / SPEAKER_SERVICE_URL 透传给业务服务包构建
#   DESKTOP_VERSION     业务服务包内桌面端安装包版本；不传则由发布脚本读取 desktop/package.json
#   DESKTOP_INSTALLER   透传给业务服务包构建，直接复用现成 Tauri Win10/11 安装包
#   DESKTOP_ELECTRON_INSTALLER 透传给业务服务包构建，直接复用现成 Win7 兼容版安装包
#   SKIP_ELECTRON=1     透传给业务服务包构建，跳过 Win7 兼容版打包
#   DRY_RUN=1           透传给业务服务包构建，跳过 Docker 镜像和桌面端自动构建
#   SPEAKER_ARGS        透传给 deploy/3d-speaker/scripts/build-offline-run.sh，例如 "--allow-incomplete-native-cache"
#   KEEP_WORK=1         保留统一发布脚本临时目录
#   JUSHA_ASR_PART_SIZE .run 分包大小，默认 500m；输出为 xxx.run + xxx.run.part001...
#   JUSHA_ASR_KEEP_ARCHIVE=1 保留中间 .tar.gz，默认不保留大单文件
#   SOURCE_DIR          Qwen3-ASR 源目录，默认仓库内 deploy/qwen3-asr
#   CONTAINER           Qwen3-ASR 源容器名，默认 qwen3-asr-serve
#   SOURCE_IMAGE        Qwen3-ASR 源镜像，默认 qwenllm/qwen3-asr:latest；源容器不存在时回退使用
#   GPU_COUNT           Qwen3-ASR 默认 GPU 数量，默认 1
#   GPU_DEVICE_IDS      Qwen3-ASR 指定 GPU ID，设置后优先于 GPU_COUNT
#   SA_GPU_ID           3D-Speaker GPU ID，默认 0
#   默认包/镜像/容器名：jusha-asr-business、jusha-asr-asr、jusha-asr-speaker
#   默认网络：jusha-asr；默认端口：业务 HTTP 9855 / HTTPS 9856，ASR 9851，3D-Speaker 9852
#
# release-all-in-one 可选参数：
#   VERSION             发布版本号
#   OUTPUT_DIR          发布产物输出目录
#   SERVER_HOST         对外访问域名或 IP
#   HTTP_PORT           HTTP 监听端口
#   HTTPS_PORT          HTTPS 监听端口
#   ADMIN_USERNAME      初始化管理员用户名
#   ADMIN_PASSWORD      初始化管理员密码
#   ADMIN_DISPLAY_NAME  初始化管理员显示名
#   MYSQL_PASSWORD      MySQL 密码
#   JWT_SECRET          JWT 签名密钥
#   ASR_SERVICE_URL     外部 ASR 服务地址
#   SPEAKER_SERVICE_URL 说话人服务地址
#   DESKTOP_VERSION            桌面端安装包版本；不传则由发布脚本读取 desktop/package.json
#   DESKTOP_INSTALLER          桌面端（Tauri Win10/11）安装包路径
#   DESKTOP_ELECTRON_INSTALLER Win7 兼容版（Electron 22）安装包路径
#   SKIP_ELECTRON=1            跳过 Win7 兼容版打包
#   DRY_RUN=1                  仅演练命令，不真正产出发布包
#   示例：make release-all-in-one HTTP_PORT=9855 HTTPS_PORT=9856 ADMIN_PASSWORD=jusha1996 ASR_SERVICE_URL=http://host.docker.internal:9851 SPEAKER_SERVICE_URL=http://host.docker.internal:9852 SERVER_HOST=192.168.40.221 VERSION=0.8.6
#         make release-all-in-one HTTP_PORT=9855 HTTPS_PORT=9856 ADMIN_PASSWORD=jusha1996 ASR_SERVICE_URL=http://host.docker.internal:9851 SPEAKER_SERVICE_URL=http://host.docker.internal:9852 SERVER_HOST=10.10.10.150 VERSION=0.8.6

JUSHA_MODE_VALUE = $(or $(JUSHA_MODE),all)

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

# 编译全部后端二进制到 bin/ 目录。
build-backend:
	cd backend && go build -o ../bin/gateway ./cmd/gateway
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

# 编译单个 gateway 服务。
build-gateway:
	cd backend && go build -o ../bin/gateway ./cmd/gateway

# 编译单个 asr-api 服务。
build-asr-api:
	cd backend && go build -o ../bin/asr-api ./cmd/asr-api

# 编译单个 admin-api 服务。
build-admin-api:
	cd backend && go build -o ../bin/admin-api ./cmd/admin-api

# 编译单个 nlp-api 服务。
build-nlp-api:
	cd backend && go build -o ../bin/nlp-api ./cmd/nlp-api

# 本地启动默认后端入口（gateway）。
dev-backend: dev-gateway

# 以 go run 方式本地启动 gateway。
dev-gateway:
	cd backend && go run ./cmd/gateway

# 以 go run 方式本地启动 asr-api。
dev-asr-api:
	cd backend && go run ./cmd/asr-api

# 以 go run 方式本地启动 admin-api。
dev-admin-api:
	cd backend && go run ./cmd/admin-api

# 以 go run 方式本地启动 nlp-api。
dev-nlp-api:
	cd backend && go run ./cmd/nlp-api

# --- Frontend ---
# 启动前端开发服务器。
dev-frontend:
	cd frontend && pnpm dev

# 构建前端静态资源。
build-frontend:
	cd frontend && pnpm build

# --- Quality ---
# 运行前端 ESLint 与后端 golangci-lint。
lint:
	cd frontend && pnpm lint
	cd backend && golangci-lint run ./...

# 运行后端 Go 单元测试。
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

# 运行前端 Vitest 单元测试。
test-frontend-unit:
	cd frontend && pnpm test:unit

# 运行前端 Playwright 业务流程测试。
test-frontend-e2e:
	cd frontend && pnpm test:e2e

# 运行项目测试：后端单测、前端单测与前端业务流程测试。
test: test-backend test-frontend-unit test-frontend-e2e

# --- Docker ---
# 启动 deploy 目录下的容器环境。
docker-up:
	cd deploy && docker compose up -d

# 停止并清理 deploy 目录下的容器环境。
docker-down:
	cd deploy && docker compose down

# 构建 deploy 目录下的镜像。
docker-build:
	cd deploy && docker compose build

# 生成一体化发布包，参数透传给 deploy/all-in-one/scripts/build-release.sh。
release-all-in-one:
	@set --; \
	$(call append_arg,--version,VERSION) \
	$(call append_arg,--output-dir,OUTPUT_DIR) \
	$(call append_arg,--server-host,SERVER_HOST) \
	$(call append_arg,--http-port,HTTP_PORT) \
	$(call append_arg,--https-port,HTTPS_PORT) \
	$(call append_arg,--admin-username,ADMIN_USERNAME) \
	$(call append_arg,--admin-password,ADMIN_PASSWORD) \
	$(call append_arg,--admin-display-name,ADMIN_DISPLAY_NAME) \
	$(call append_arg,--mysql-password,MYSQL_PASSWORD) \
	$(call append_arg,--jwt-secret,JWT_SECRET) \
	$(call append_arg,--asr-service-url,ASR_SERVICE_URL) \
	$(call append_arg,--speaker-service-url,SPEAKER_SERVICE_URL) \
	$(call append_arg,--desktop-version,DESKTOP_VERSION) \
	$(call append_arg,--desktop-installer,DESKTOP_INSTALLER) \
	$(call append_arg,--desktop-electron-installer,DESKTOP_ELECTRON_INSTALLER) \
	$(call append_flag,--skip-electron,SKIP_ELECTRON) \
	$(call append_flag,--dry-run,DRY_RUN) \
	sh deploy/all-in-one/scripts/build-release.sh "$$@"

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

# 只生成模型服务组合包：ASR + 3D-Speaker。
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
