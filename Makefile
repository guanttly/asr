.PHONY: all build-backend build-frontend dev-backend dev-frontend lint test test-backend test-frontend-unit test-frontend-e2e clean release-all-in-one generate-radiology-term-excel sync-term-catalog generate-radiology-rules-excel sync-rules-catalog

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
#   make release-all-in-one VERSION=0.3.9 OUTPUT_DIR=./dist DRY_RUN=1
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
#   DESKTOP_INSTALLER          桌面端（Tauri Win10/11）安装包路径
#   DESKTOP_ELECTRON_INSTALLER Win7 兼容版（Electron 22）安装包路径
#   SKIP_ELECTRON=1            跳过 Win7 兼容版打包
#   DRY_RUN=1                  仅演练命令，不真正产出发布包
#   示例：make release-all-in-one HTTP_PORT=9855 HTTPS_PORT=9856 ADMIN_PASSWORD=jusha1996 ASR_SERVICE_URL=http://host.docker.internal:9851 SPEAKER_SERVICE_URL=http://host.docker.internal:9852 SERVER_HOST=192.168.40.221 VERSION=0.8.6
#         make release-all-in-one HTTP_PORT=9855 HTTPS_PORT=9856 ADMIN_PASSWORD=jusha1996 ASR_SERVICE_URL=http://host.docker.internal:9851 SPEAKER_SERVICE_URL=http://host.docker.internal:9852 SERVER_HOST=10.10.10.150 VERSION=0.8.6
# --- Backend ---
.PHONY: build-gateway build-asr-api build-admin-api build-nlp-api

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
	sh deploy/all-in-one/scripts/build-release.sh $(if $(VERSION),--version $(VERSION),) $(if $(OUTPUT_DIR),--output-dir $(OUTPUT_DIR),) $(if $(SERVER_HOST),--server-host $(SERVER_HOST),) $(if $(HTTP_PORT),--http-port $(HTTP_PORT),) $(if $(HTTPS_PORT),--https-port $(HTTPS_PORT),) $(if $(ADMIN_USERNAME),--admin-username $(ADMIN_USERNAME),) $(if $(ADMIN_PASSWORD),--admin-password $(ADMIN_PASSWORD),) $(if $(ADMIN_DISPLAY_NAME),--admin-display-name $(ADMIN_DISPLAY_NAME),) $(if $(MYSQL_PASSWORD),--mysql-password $(MYSQL_PASSWORD),) $(if $(JWT_SECRET),--jwt-secret $(JWT_SECRET),) $(if $(ASR_SERVICE_URL),--asr-service-url $(ASR_SERVICE_URL),) $(if $(SPEAKER_SERVICE_URL),--speaker-service-url $(SPEAKER_SERVICE_URL),) $(if $(DESKTOP_INSTALLER),--desktop-installer $(DESKTOP_INSTALLER),) $(if $(DESKTOP_ELECTRON_INSTALLER),--desktop-electron-installer $(DESKTOP_ELECTRON_INSTALLER),) $(if $(SKIP_ELECTRON),--skip-electron,) $(if $(DRY_RUN),--dry-run,)

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
