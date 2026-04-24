# All-in-One Docker 发布包

这个目录提供单镜像发布方案，镜像内包含以下进程：

- MySQL
- gateway
- admin-api
- asr-api
- nlp-api
- Nginx

## 挂载目录

- 数据目录: /var/lib/asr/mysql
- 证书目录: /var/lib/asr/certs
- 下载包目录: /var/lib/asr/downloads
- 临时目录: /var/lib/asr/tmp
- 音频存储目录: /var/lib/asr/uploads

## 快速启动

1. 打包脚本现在可以自动把桌面端安装包打入 runtime/downloads。
2. 打包时可直接传服务器 IP、HTTP 端口和管理员密码，生成预配置 `.env`。
3. 发布产物除了 `.tar.gz`，还会额外输出一个可直接执行的 `.run` 一键安装包。
4. 如果是最终交付，优先使用 `.run`；服务器上执行 `bash asr-all-in-one-<version>.run` 即可自动解包并安装。

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

- 执行 deploy/all-in-one/scripts/build-release.sh 会输出完整离线发布目录、tar.gz 压缩包和 `.run` 一键安装包。
- 发布包内包含 docker-compose.yml、install.sh、uninstall.sh、.env、.env.example、.release-manifest、runtime 目录和离线镜像包。
- 发布包解压后的根目录固定为 asr-all-in-one，便于新版本直接覆盖到同一路径后执行升级。
- 目标服务器解压后执行 install.sh 或 install.sh upgrade，即可自动 load 镜像并启动或原地升级服务。
- 如果直接执行 `.run` 文件，会自动解包到当前目录并执行 install.sh。
- 升级时数据目录继续复用 runtime/mysql、runtime/uploads、runtime/downloads 和 runtime/tmp，install.sh 会额外备份当前 .env 与 compose 文件。
- install.sh 会等待容器健康检查通过；如果升级后服务未通过健康检查，脚本会尝试回滚到上一版镜像。
- install.sh 完成后会输出当前证书的 SAN、推荐访问地址，以及 Windows Chrome/Edge 与 Firefox 的自签证书导入提示。
- 首次安装时，admin-api 会按 `.env` 中的 `ASR_BOOTSTRAP_ADMIN_USERNAME`、`ASR_BOOTSTRAP_ADMIN_PASSWORD`、`ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME` 自动创建管理员账号；如果同名管理员已存在，则不会覆盖旧密码。
- 打包时如果传入 `--server-host` 和端口参数，桌面客户端会以内置 HTTPS 默认地址重新构建，并自动放到 `runtime/downloads`。
- 当前 Windows 桌面发布构建会为 WebView2 附加忽略证书错误参数，便于内网自签名 HTTPS 直接访问；如果 HTTPS 失败，客户端仍会按现有逻辑回退尝试 HTTP。

## 卸载

- 执行 `sh uninstall.sh` 可以停止并删除当前容器，但保留 runtime 数据目录和 `.env`。
- 执行 `sh uninstall.sh purge` 可以同时清空 runtime/mysql、runtime/uploads、runtime/downloads、runtime/tmp、runtime/certs 和 backups。
- 执行 `sh uninstall.sh purge --remove-image` 会额外尝试删除本地离线镜像标签。

## 外部依赖

- ASR 服务地址通过 ASR_SERVICES_ASR 配置。
- 说话人分析与分离服务可选；未配置时相关能力保持当前后端已有降级行为。