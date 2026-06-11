# jusha-asr-business Docker 发布包

这个目录提供单镜像发布方案，镜像内包含以下进程：

- MySQL
- gateway
- admin-api
- asr-api
- nlp-api
- Nginx

## 挂载目录

- 配置目录: /app/backend/configs（宿主机路径 `runtime/config/`；实施可修改 `runtime/config/config.yaml` 后重启容器生效。首次安装或旧版本升级时如果文件不存在，会从默认模板生成）
- 数据目录: /var/lib/asr/mysql
- 证书目录: /var/lib/asr/certs
- 下载包目录: /var/lib/asr/downloads
- 临时目录: /var/lib/asr/tmp
- 音频存储目录: /var/lib/asr/uploads
- 影像术语库目录: /var/lib/asr/term-catalog（首次启动自动从镜像内 `/opt/asr/term-catalog-default` 复制；之后运维可直接增删改 `runtime/term-catalog/` 下的 md 文件，无需重新构建镜像，「系统管理 → 影像术语库」会实时读取。整目录清空后重启容器会再次 seed）
- 日志目录: /var/log/asr（宿主机路径 `runtime/logs/`；legacy 兼容接口访问日志默认写入 `legacy-access.log`。其他服务日志仍通过 `docker compose logs` 查看，并由 compose 限制滚动大小）

## 快速启动

1. 打包脚本现在可以自动把桌面端安装包打入 runtime/downloads。
2. 打包时可直接传服务器 IP、HTTP 端口和管理员密码，生成预配置 `.env`。
3. 发布产物默认输出 `.run` 启动脚本和 `.run.partNNN` 分包；如设置 `ASR_RELEASE_KEEP_ARCHIVE=1`，会额外保留 `.tar.gz`。
4. 如果是最终交付，请把 `.run` 和所有 `.run.partNNN` 放在同一目录；服务器上执行 `bash jusha-asr-business-<version>.run` 即可自动解包并安装。

## 下载页说明

- 公共下载页地址是 /downloads。
- 前端通过 /api/admin/public/downloads 读取下载包列表。
- Nginx 通过 /downloads/files/ 直接分发挂载目录中的文件。
- 如果下载目录为空，页面会显示空态，不会影响其他业务模块。

## HTTPS 注意

- 浏览器端实时语音页会调用麦克风采集接口，远程访问时通常必须运行在 HTTPS 安全上下文。
- localhost 是浏览器允许的特例；如果只是本机打开页面，HTTP 也可能正常工作。
- 当前发布包默认同时提供 HTTP 和 HTTPS。
- 浏览器建议使用 HTTPS；桌面客户端建议使用 HTTP。
- 当前发布包默认启用 HTTPS，并会在证书不存在时自动生成自签名证书。
- 首次访问时浏览器会提示证书不受信任，这对内网自签名部署是预期行为；接受证书后即可正常使用网页端实时录音。
- 如果你希望证书里的主机名和 IP 与实际访问地址一致，可以在 .env 里调整 ASR_TLS_COMMON_NAME 与 ASR_TLS_ALT_NAMES，或直接把现成证书放到 runtime/certs。
- 如果你希望 HTTP 全部自动跳转到 HTTPS，可在 .env 中设置 `ASR_HTTP_REDIRECT_TO_HTTPS=1`。

## 离线发布包

- 执行 deploy/jusha-asr-business/scripts/build-release.sh 会输出完整离线发布目录、`.run` 启动脚本和 `.run.partNNN` 分包；`.tar.gz` 中间压缩包默认会在分包后删除，可用 `ASR_RELEASE_KEEP_ARCHIVE=1` 保留。
- 如果在部署服务器本机打包，请使用独立输出目录（例如 `/data/ganttly/releases`），不要把输出目录设为正在运行的 `jusha-asr-business` 安装目录或它的父目录中会覆盖安装目录的位置；打包脚本会拒绝覆盖正在运行容器的安装目录。
- 发布包内包含 docker-compose.yml、install.sh、uninstall.sh、.env、.env.example、.release-manifest、runtime 目录和离线镜像包。
- 发布包解压后的根目录固定为 jusha-asr-business，便于新版本直接覆盖到同一路径后执行升级。
- 目标服务器解压后执行 install.sh 或 install.sh upgrade，即可自动 load 镜像并启动或原地升级服务。
- 如果直接执行 `.run` 文件，会自动解包到当前目录并执行 install.sh。升级时请在旧安装目录的父目录执行，保证目标仍是原来的 `jusha-asr-business` 目录；如果需要指定父目录，请设置 `ASR_RUN_TARGET_DIR=/path/to/parent`。
- 如果在已有 `jusha-asr-business` 目录上再次执行新的 `.run`，安装包会先解包到临时目录，再同步到现有目录：保留现有 `.env`、runtime/config、runtime/mysql、runtime/uploads、runtime/tmp、runtime/term-catalog、runtime/logs 和已有证书，同时刷新 docker-compose.yml、安装脚本、离线镜像包与 runtime/downloads，并清理旧版本残留的发布文件。
- install.sh 会检查已有容器的 runtime/config、runtime/mysql、runtime/uploads、runtime/term-catalog、runtime/logs 挂载来源是否属于当前安装目录；如果在新目录误执行升级，会拒绝继续，避免容器切到空 MySQL 数据目录导致工作流、术语库等数据看似丢失。
- install.sh 在升级前会备份当前 `.env`、compose、证书、MySQL 数据、runtime/config 和 runtime/term-catalog 到 `backups/<timestamp>/`。运行中的容器会优先生成 MySQL 逻辑备份 `mysql-<dbname>.sql.gz`；停机状态则备份 runtime/mysql 目录归档。
- 这样既能避免 MySQL 数据目录因属主为容器用户而在覆盖解包时触发 tar 报错，也能确保升级后实际运行的是新版本 compose 和下载资源。
- install.sh 默认使用共享 Docker 网络 `jusha-asr`。如果该网络已存在，会复用已有网段；如果不存在，会在启动前检查宿主机 IPv4 路由和已有 Docker 网络，自动选择未占用的 Docker 内部网段并创建该网络。
- install.sh 会等待容器健康检查通过；如果升级后服务未通过健康检查，脚本会尝试回滚到上一版镜像。
- install.sh 完成后会输出当前证书的 SAN、推荐访问地址，以及 Windows Chrome/Edge 与 Firefox 的自签证书导入提示。
- 首次安装时，admin-api 会按 `.env` 中的 `ASR_BOOTSTRAP_ADMIN_USERNAME`、`ASR_BOOTSTRAP_ADMIN_PASSWORD`、`ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME` 自动创建管理员账号；如果同名管理员已存在，则不会覆盖旧密码。
- 打包时如果传入 `--server-host` 和端口参数，桌面客户端会以内置 HTTPS 默认地址重新构建，并自动放到 `runtime/downloads`。桌面端安装包版本默认读取 `desktop/package.json`，也可传 `--desktop-version <version>` 显式指定；自动构建 Tauri 时缺少 Rust、Windows MSVC target 或 cargo-xwin 会默认尝试自动安装，并默认使用 rsproxy Rust/Cargo 国内镜像，可用 `ASR_RELEASE_AUTO_INSTALL_RUST=0` 禁用自动安装，或用 `ASR_RELEASE_USE_RUST_MIRROR=0` 禁用镜像。
- Linux 服务器自动构建 Win7 兼容版时默认使用 Docker Wine + xvfb，不需要宿主机桌面 UI；如需改用宿主机 Wine，可设置 `ASR_RELEASE_ELECTRON_WIN_BUILD_MODE=host`。
- Win10/11 推荐版由 `desktop/`（Tauri）打包，Win7 兼容版由 `desktop-electron/`（Electron 22）打包；发布脚本会同时启动两者并按文件名中的 `_win10_` / `_win7_` 标识让公共下载页分组展示。
- 如果仅需发 Win10/11 主推荐版，可传 `--skip-electron` 跳过 Win7 包构建；如果手边已有提前打好的安装包，可用 `--desktop-installer <path>` 和 `--desktop-electron-installer <path>` 直接复用。
- Tauri 在 Windows 上使用系统 WebView2（Win10 1803+ 后预装）；Win7 用户请下载 Win7 兼容版。

## 卸载

- 执行 `sh uninstall.sh` 可以停止并删除当前容器，但保留 runtime 数据目录和 `.env`。
- 执行 `sh uninstall.sh purge` 可以同时清空 runtime/config、runtime/mysql、runtime/uploads、runtime/downloads、runtime/tmp、runtime/certs、runtime/logs 和 backups。
- `purge` 会在普通 `rm -rf` 遇到 mysql 用户写入的受限文件时，自动回退到容器内 root 清理，避免因为 runtime/mysql 被 chown 成 mysql:mysql 而删不干净。
- 执行 `sh uninstall.sh purge --remove-image` 会额外尝试删除本地离线镜像标签。

## 外部依赖

- ASR 服务地址通过 ASR_SERVICES_ASR 配置。
- 默认组合部署时，业务容器通过共享网络直接访问 `http://jusha-asr-asr:8000` 和 `http://jusha-asr-speaker:8100`，宿主机对外端口分别是 9851 和 9852。
- 如需改成访问宿主机端口或另一台机器，可在 `.env` 中调整 `ASR_SERVICES_ASR` 和 `ASR_SERVICES_SPEAKER_SERVICE_URL`。同机宿主端口示例：`http://host.docker.internal:9851`、`http://host.docker.internal:9852`。
- `host.docker.internal` 会在安装时写到共享 Docker 网络网关；如需手动指定，可在 `.env` 中设置 `ASR_DOCKER_NETWORK_NAME`、`ASR_DOCKER_SUBNET`，安装脚本会同步生成对应网关。
- 如果 3D-Speaker 同时承担说话人分离和声纹能力，发布脚本层现在只传一个统一的 `SPEAKER_SERVICE_URL`；生成的发布包会把后端内部的 diarization 和 speaker-analysis 地址都写成同一个值。
- 如果外部服务部署在另一台机器上，应填写那台机器的实际内网 IP 或域名，例如 `http://192.168.40.223:9852`。
- 说话人分析与分离服务可选；未配置时相关能力保持当前后端已有降级行为。
