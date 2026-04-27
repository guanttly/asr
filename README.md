# ASR 项目说明

这个仓库当前已经整理出一套适合内网部署的 all-in-one Docker 发布流程。

这份 README 只放最常用、最直接的命令，目标是：

- 在打包机上一键生成离线发布包
- 把压缩包拷到服务器
- 在服务器上首次安装或后续升级
- 使用自签名 HTTPS 打开网页端实时语音识别

## 目录说明

- 后端源码: [backend](backend)
- 前端源码: [frontend](frontend)
- 桌面端源码: [desktop](desktop)
- 一体化发布目录: [deploy/all-in-one](deploy/all-in-one)

## 一句话结论

打包机执行：

```bash
make release-all-in-one VERSION=0.3.10 SERVER_HOST=192.168.40.221 HTTP_PORT=11010 ADMIN_PASSWORD='jusha1996'
```

服务器执行：

```bash
bash asr-all-in-one-0.2.7.run
```

后续升级执行：

```bash
tar -xzf asr-all-in-one-0.2.6.tar.gz
cd asr-all-in-one
sh install.sh upgrade
```

卸载执行：

```bash
cd asr-all-in-one
sh uninstall.sh
```

## 前置条件

服务器至少需要：

- Docker
- `docker compose` 或 `docker-compose`

可以先检查：

```bash
docker --version
docker compose version
```

如果你的机器只有老版本：

```bash
docker-compose --version
```

## 打包机上如何生成正式发布包

注意：不要用 `--dry-run` 做正式安装包。

`--dry-run` 只生成目录结构，不导出离线镜像，不能直接拿到服务器安装。

正式打包命令：

```bash
cd /home/lgt/asr
sh deploy/all-in-one/scripts/build-release.sh
```

或者：

```bash
cd /home/lgt/asr
make release-all-in-one
```

默认情况下，打包版本号来自 [desktop/package.json](desktop/package.json) 里的 `version`。

当前打包脚本已经支持直接生成“免配置发布包”，包含这些内容：

- 预生成好的 `.env`
- 自动打入 `runtime/downloads` 的桌面客户端安装包
- `.tar.gz` 离线包
- 可直接执行的 `.run` 一键安装包

推荐用法：

```bash
cd /home/lgt/asr
make release-all-in-one \
	VERSION=0.2.7 \
	SERVER_HOST=192.168.40.223 \
	HTTP_PORT=11010 \
	ADMIN_PASSWORD='Admin@123456'
```

上面这组参数会自动得到：

- 客户端默认地址: `https://192.168.40.223:11011`
- 服务端 HTTP 端口: `11010`
- 服务端 HTTPS 端口: `11011`
- 默认管理员密码: 你传入的 `ADMIN_PASSWORD`

当前发布构建会把桌面客户端默认地址打成 HTTPS，并在 Windows WebView2 中显式附加忽略证书错误参数，方便内网自签名证书直接连通；如果 HTTPS 不通，客户端自身仍会按现有逻辑回退尝试 HTTP。

也就是说，打好的包可以直接安装，不再需要手工复制客户端到 `downloads`，也不需要服务器上再手改 `.env`。

如果你想临时指定版本号，不用改代码，直接传参数：

```bash
cd /home/lgt/asr
sh deploy/all-in-one/scripts/build-release.sh --version 0.2.7
```

或者直接用 Make 参数：

```bash
cd /home/lgt/asr
make release-all-in-one VERSION=0.2.7
```

也支持一起传输出目录和 dry-run：

```bash
cd /home/lgt/asr
make release-all-in-one VERSION=0.2.7 OUTPUT_DIR=/tmp/asr-release DRY_RUN=1
```

常用参数：

```bash
make release-all-in-one \
	VERSION=0.2.7 \
	SERVER_HOST=192.168.40.223 \
	HTTP_PORT=11010 \
	ADMIN_USERNAME=admin \
	ADMIN_PASSWORD='Admin@123456' \
	ADMIN_DISPLAY_NAME='系统管理员' \
	MYSQL_PASSWORD='Mysql@123456' \
	JWT_SECRET='your-jwt-secret' \
	ASR_SERVICE_URL='http://host.docker.internal:8000' \
	SPEAKER_SERVICE_URL='http://host.docker.internal:10002'
```

说明：

- `SERVER_HOST` 用于客户端默认连接地址和 TLS 证书主机名
- `HTTP_PORT` 用于桌面客户端默认连接端口
- 如果你没有显式传 `HTTPS_PORT`，脚本会自动取 `HTTP_PORT + 1`
- `ADMIN_PASSWORD` 会直接写入发布包里的 `.env`
- 如果不传 `MYSQL_PASSWORD` 和 `JWT_SECRET`，脚本会自动生成随机值
- `ASR_SERVICE_URL` 和 `SPEAKER_SERVICE_URL` 都是“从 all-in-one 容器内部看出去”的地址
- 如果外部服务和 all-in-one 部署在同一台宿主机上，推荐填 `http://host.docker.internal:<端口>`；服务器 shell 里 `ping host.docker.internal` 不通是正常的
- 这里的 `<端口>` 必须写“宿主机实际暴露出来的端口”，不是外部容器自己的内部监听端口；例如外部 ASR 如果是 `-p 11001:8000`，那这里应填 `http://host.docker.internal:11001`
- 如果外部服务部署在另一台机器上，就填那台机器的实际内网 IP 或域名
- 如果 3D-Speaker 同时承担说话人分离和声纹能力，优先只传 `SPEAKER_SERVICE_URL`
- 当前发布脚本不再单独暴露 `DIARIZATION_SERVICE_URL` 和 `SPEAKER_ANALYSIS_SERVICE_URL` 参数；发布包会把后端内部这两个地址统一写成同一个 `SPEAKER_SERVICE_URL`
- 如果你已经提前构建好了桌面安装包，也可以传 `DESKTOP_INSTALLER=/path/to/setup.exe` 直接复用

生成物在：

- 发布目录: [deploy/all-in-one/dist/asr-all-in-one](deploy/all-in-one/dist/asr-all-in-one)
- 压缩包: [deploy/all-in-one/dist/asr-all-in-one-0.2.6.tar.gz](deploy/all-in-one/dist/asr-all-in-one-0.2.6.tar.gz)
- 一键安装包: [deploy/all-in-one/dist/asr-all-in-one-0.2.6.run](deploy/all-in-one/dist/asr-all-in-one-0.2.6.run)

## 为什么现在不会再把 9G 上下文塞进 Docker build

当前仓库已经新增了 [ .dockerignore ](.dockerignore)，会排除这些大目录和无关内容：

- [desktop](desktop)
- [backend/uploads](backend/uploads)
- 前端 `node_modules`
- 前端 `dist`
- 发布目录 `deploy/all-in-one/dist`
- Go 调试二进制

所以 all-in-one 镜像不会再把桌面端 8G+ 构建产物、上传音频、前端依赖一起送进 Docker build context。

另外，all-in-one 镜像不会内置 ASR 服务和模型；这些是外部依赖，通过环境变量指向你另外打的服务包。

## 把压缩包拷到服务器

例如：

```bash
scp deploy/all-in-one/dist/asr-all-in-one-0.2.6.tar.gz user@your-server:/data/
```

如果你用的是一键安装包，也可以直接传：

```bash
scp deploy/all-in-one/dist/asr-all-in-one-0.2.6.run user@your-server:/data/
```

如果是 Windows 跳板、SFTP 或其他传输方式也可以，重点只有一个：

把 `asr-all-in-one-0.2.6.tar.gz` 传到服务器即可。

## 服务器首次安装步骤

### 1. 解压

```bash
cd /data
tar -xzf asr-all-in-one-0.2.6.tar.gz
cd asr-all-in-one
```

解压后的目录固定叫：

```bash
asr-all-in-one
```

这是为了后续升级时可以直接覆盖同一路径。

如果你使用 `.run` 一键包，这一步可以省略，直接执行：

```bash
cd /data
bash asr-all-in-one-0.2.6.run
```

它会自动：

- 解压到当前目录
- 进入 `asr-all-in-one`
- 直接执行 `install.sh`
- 使用打包时预生成好的 `.env`

### 2. 生成配置文件

```bash
cp .env.example .env
vi .env
```

如果你的发布包是在打包机上带参数生成的，那么 `.env` 已经预填好了，通常不需要再手工复制和修改；只有你想临时覆盖打包参数时，才需要重新编辑它。

注意：

- 第一次安装前，必须先改好 `.env` 再执行 `sh install.sh`
- 不要让安装脚本直接用默认密码初始化 MySQL
- 因为 `runtime/mysql` 一旦完成首次初始化，后续再改 `ASR_MYSQL_ROOT_PASSWORD` 不会自动改库里的 root 密码
- 如果第一次已经用错密码初始化，而且库里还没有重要数据，直接删除 `runtime/mysql` 后重新安装最省事

至少要改这些：

```bash
ASR_MYSQL_ROOT_PASSWORD=你的数据库密码
ASR_BOOTSTRAP_ADMIN_USERNAME=admin
ASR_BOOTSTRAP_ADMIN_PASSWORD=你的管理员密码
ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME=系统管理员
ASR_JWT_SECRET=你的随机密钥
ASR_SERVICES_ASR=http://你的外部ASR服务地址
```

admin 账号初始化说明：

- 不需要再额外执行单独的“初始化 admin”命令
- 首次安装启动时，`admin-api` 会自动按 `.env` 中的 `ASR_BOOTSTRAP_ADMIN_USERNAME`、`ASR_BOOTSTRAP_ADMIN_PASSWORD`、`ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME` 植入一个管理员账号
- 也就是说，安装完成后可以直接用这个管理员账号登录后台
- 这个自动植入逻辑是“确保存在”，不是“每次强制覆盖”
- 如果数据库里已经存在同名 admin，后续再次安装或升级不会自动重置它的密码和显示名

如果你有外部人声服务，再改：

```bash
ASR_SERVICES_SPEAKER_SERVICE_URL=http://你的3D-Speaker地址
```

如果说话人分离和声纹能力都由同一套 3D-Speaker 提供，例如它和 all-in-one 跑在同一台宿主机、对外端口是 `10002`，可填写：

```bash
ASR_SERVICES_SPEAKER_SERVICE_URL=http://host.docker.internal:10002
```

注意：`host.docker.internal` 是给容器内部访问宿主机用的别名，不要求服务器本机 shell 能直接解析；如果你的 3D-Speaker 部署在另一台服务器，就改成那台机器的实际 IP 或域名。

同理，如果你的 ASR 或 3D-Speaker 本身也是 Docker 容器，并且端口映射类似：

```text
11001 -> 8000
11002 -> 8100
```

那么 all-in-one 里应填写：

```bash
ASR_SERVICES_ASR=http://host.docker.internal:11001
ASR_SERVICES_SPEAKER_SERVICE_URL=http://host.docker.internal:11002
```

而不是填写容器内部端口 `8000` 或 `8100`。

### 3. 自签名 HTTPS 配置

默认启用 HTTPS，自签名证书会自动生成。

如果你希望证书里的主机名/IP 更符合你的服务器访问地址，可以改：

```bash
ASR_ENABLE_HTTPS=1
ASR_HTTP_REDIRECT_TO_HTTPS=0
ASR_HTTP_PORT=80
ASR_HTTPS_PORT=443
ASR_TLS_COMMON_NAME=你的服务器IP或域名
ASR_TLS_ALT_NAMES=DNS:localhost,DNS:你的主机名,IP:127.0.0.1,IP:你的服务器IP
```

如果不改，脚本也会自动根据服务器主机名和主 IP 推导默认值。

当前默认行为是：

- HTTPS 开启
- HTTP 也保持可访问
- 不强制把 HTTP 跳转到 HTTPS

也就是说：

- 浏览器页面建议走 `https://...`，这样网页端麦克风可用
- 桌面客户端可以直接走 `http://...`

如果你以后明确只想保留 HTTPS，并强制 HTTP 全部跳转到 HTTPS，再把下面这个值改成 `1`：

```bash
ASR_HTTP_REDIRECT_TO_HTTPS=1
```

### 4. 放桌面端安装包

把终端安装包放到：

```bash
runtime/downloads
```

例如：

```bash
cp /data/巨鲨语音助手_0.2.6_x64-setup.exe runtime/downloads/
```

这个目录会在网页的公共下载页中直接显示。

### 5. 执行安装

```bash
sh install.sh
```

安装脚本会自动做这些事：

- 加载离线镜像
- 启动 MySQL、gateway、admin-api、asr-api、nlp-api、Nginx
- 首次启动时自动植入 `.env` 中配置的 admin 账号
- 自动生成或复用 HTTPS 证书
- 等待健康检查通过
- 输出证书 SAN
- 输出推荐访问地址
- 输出浏览器导入自签证书提示

如果你在首次安装时遇到类似下面的错误：

```bash
failed to connect to mysql: Error 1045 (28000): Access denied for user 'root'@'localhost'
```

通常表示：

- `runtime/mysql` 已经按旧密码初始化过
- 但你后来又修改了 `.env` 里的 `ASR_MYSQL_ROOT_PASSWORD`

如果当前还是第一次部署、没有要保留的数据，直接执行：

```bash
docker compose -f docker-compose.yml down || docker-compose -f docker-compose.yml down
rm -rf runtime/mysql
sh install.sh
```

前提是你已经先把 `.env` 中的数据库密码改成最终想要的值。

## 服务器升级步骤

后续升级时，不需要删数据目录。

把新压缩包传到服务器后，直接覆盖原目录再执行：

```bash
cd /data
tar -xzf asr-all-in-one-0.2.6.tar.gz
cd asr-all-in-one
sh install.sh upgrade
```

升级脚本会：

- 保留 `runtime/mysql`
- 保留 `runtime/uploads`
- 保留 `runtime/downloads`
- 保留 `runtime/tmp`
- 保留 `runtime/certs`
- 备份当前 `.env`
- 备份当前 `docker-compose.yml`
- 备份当前 `.release-manifest`
- 备份已有证书
- 如果健康检查失败，尝试回滚到上一版镜像

## 卸载与重装

### 1. 仅卸载服务，保留数据

适合临时下线、准备重装但不想删数据：

```bash
cd /data/asr/asr-all-in-one
sh uninstall.sh
```

这个模式会：

- 停止并删除当前容器
- 保留 `runtime/mysql`
- 保留 `runtime/uploads`
- 保留 `runtime/downloads`
- 保留 `runtime/tmp`
- 保留 `runtime/certs`
- 保留 `.env`

后续可以直接重新执行：

```bash
sh install.sh
```

### 2. 卸载并清空本地数据

适合第一次部署失败、没有要保留的数据，或者你确定要从零开始：

```bash
cd /data/asr/asr-all-in-one
sh uninstall.sh purge
```

这个模式会额外删除：

- `runtime/mysql`
- `runtime/uploads`
- `runtime/downloads`
- `runtime/tmp`
- `runtime/certs`
- `backups`

如果还想把本机上的离线镜像标签一起删掉：

```bash
sh uninstall.sh purge --remove-image
```

### 3. 卸载后重新安装

如果你要完整重装，执行：

```bash
cd /data/asr
tar -xzf asr-all-in-one-0.2.6.tar.gz
cd asr-all-in-one
cp .env.example .env
vi .env
sh install.sh
```

如果只是因为数据库密码初始化错了，而当前还没有正式数据，更推荐：

```bash
cd /data/asr/asr-all-in-one
vi .env
sh uninstall.sh purge
sh install.sh
```

这样不需要重新传包，只清空当前实例数据后重装即可。

## 安装完成后访问什么地址

安装脚本完成后会自动打印实际地址。

通常你可以访问：

```bash
http://你的服务器IP
https://你的服务器IP/downloads
https://你的服务器IP/login
```

说明：

- HTTP 根地址：可给桌面客户端直接连接
- 下载页：公共入口，不登录也能访问
- 登录页：后台入口
- 网页端实时录音：必须跑在 HTTPS 安全上下文下，内网自签名是可以的

## 桌面客户端 URL 规则

桌面客户端当前包是支持自定义服务地址的，安装后可以在“连接设置”里直接修改服务器地址。

### 1. 可以填什么格式

下面几种都支持：

```text
192.168.40.223:11010
http://192.168.40.223:11010
https://192.168.40.223:11010
```

规则是：

- 如果你不写前缀，客户端会自动补成 `http://`
- 也就是说，`192.168.40.223:11010` 实际会按 `http://192.168.40.223:11010` 处理
- 如果你明确写了 `https://`，客户端会先按 HTTPS 连接

### 2. 什么时候该用 http，什么时候该用 https

如果你的服务端入口是：

- `11010 -> 80`，那客户端填 `http://你的服务器IP:11010`
- `11010 -> 443`，那客户端填 `https://你的服务器IP:11010`

当前推荐是：

- 浏览器页面走 HTTPS
- 桌面客户端走 HTTP

也就是：

- 浏览器访问 `https://你的服务器IP` 或 `https://你的服务器IP:端口`
- 桌面客户端填写 `http://你的服务器IP` 或 `http://你的服务器IP:端口`

如果你把 `11010` 转发到了服务端 HTTPS 入口 `443`，就不能只填：

```text
192.168.40.223:11010
```

因为它会被客户端当成：

```text
http://192.168.40.223:11010
```

这时协议就错了，连接会失败。

### 2.1 Nginx 是根据什么转发的

当前 all-in-one 对外暴露的是 Nginx 的入口，不区分“网页端地址”和“客户端地址”；它是按请求路径转发：

- `/` -> 前端静态页面
- `/healthz` -> 转发到内部 `127.0.0.1:10010/healthz`
- `/readyz` -> 转发到内部 `127.0.0.1:10010/readyz`
- `/api/` -> 转发到内部 `127.0.0.1:10010`
- `/ws/` -> 转发到内部 `127.0.0.1:10010`
- `/uploads/` -> 转发到内部 `127.0.0.1:10010`
- `/downloads/files/` -> 直接读取下载目录

所以：

- 浏览器访问同一个站点时，打开的是网页页面
- 桌面客户端访问同一个站点时，请求的是 `/healthz`、`/api/admin/...` 这些接口路径
- 两者可以共用同一个对外地址和端口

### 2.2 客户端能不能直接访问当前暴露的网页地址

可以，但有一个前提：

- 客户端里填写的必须是“站点根地址（origin）”
- 不能填写具体页面路径

正确示例：

```text
http://192.168.40.223:11010
https://192.168.40.223:11010
https://192.168.40.223
```

错误示例：

```text
https://192.168.40.223:11010/downloads
https://192.168.40.223/login
```

原因是桌面客户端会在你填写的地址后面继续拼接：

- `/healthz`
- `/api/admin/auth/anonymous-login`
- `/api/admin/me`

如果你填的是页面地址，最终就会变成错误路径，例如：

```text
https://192.168.40.223:11010/downloads/healthz
```

这当然会失败。

### 3. 现在打出来的客户端包支持吗

支持。

当前桌面端代码已经支持：

- 手动输入带端口的服务器地址
- 不写前缀时自动补 `http://`
- 明确写 `https://` 时优先走 HTTPS
- 安装后在设置页随时修改服务器地址，不需要重新打包

默认情况下，如果你没有在打包时指定默认地址，桌面客户端会使用：

```text
http://127.0.0.1:10010
```

### 4. 自签名 HTTPS 的注意事项

如果桌面客户端要连：

```text
https://你的服务器IP:11010
```

并且服务端用的是自签名证书，那么客户端所在机器通常也需要信任这个证书；否则 HTTPS 握手可能失败。

如果你不准备给客户端机器导入证书，就不要让客户端走自签名 HTTPS 端口；这种情况下更适合：

- 单独开放一个 HTTP 内网端口给桌面客户端
- 或者直接使用当前默认提供的 HTTP 入口给桌面客户端

### 5. 调试包和发行包怎么指定默认服务器地址

桌面端调试和发行都支持通过环境变量 `VITE_DEFAULT_SERVER_URL` 指定默认服务器地址。

例如，调试开发时：

```bash
cd /home/lgt/asr/desktop
VITE_DEFAULT_SERVER_URL=http://192.168.40.223:11010 pnpm dev
```

例如，Windows 调试构建：

```bash
cd /home/lgt/asr/desktop
VITE_DEFAULT_SERVER_URL=http://192.168.40.223:11010 pnpm build:win:debug
```

例如，Windows 正式发行构建：

```bash
cd /home/lgt/asr/desktop
VITE_DEFAULT_SERVER_URL=https://192.168.40.223:11010 pnpm build:win
```

如果不传这个环境变量，发行包和调试包都会回退到默认值：

```text
http://127.0.0.1:10010
```

### 6. 推荐写法

如果你的 all-in-one 对外还是标准 HTTPS 入口：

```text
https://你的服务器IP
```

如果你做了自定义端口映射，例如：`11010 -> 443`，推荐桌面客户端填写：

```text
https://你的服务器IP:11010
```

如果你做的是 `11010 -> 80`，则填写：

```text
http://你的服务器IP:11010
```

## 自签名证书导入说明

安装脚本会打印证书 SAN 和浏览器导入提示。

证书文件位置：

```bash
runtime/certs/tls.crt
```

### Windows Chrome / Edge

1. 双击 `tls.crt`
2. 选择“安装证书”
3. 选择“本地计算机”
4. 选择“将所有的证书都放入下列存储”
5. 选择“受信任的根证书颁发机构”

### Firefox

1. 设置
2. 隐私与安全
3. 证书
4. 查看证书
5. 导入
6. 选择 `tls.crt`
7. 勾选信任此 CA 标识网站

导入后建议重启浏览器再访问 HTTPS 页面。

## 常用命令

### 查看容器状态

如果服务器支持新命令：

```bash
docker compose -f docker-compose.yml ps
```

如果服务器是老命令：

```bash
docker-compose -f docker-compose.yml ps
```

### 看日志

```bash
docker logs -f asr-all-in-one
```

### 手动重启

```bash
docker compose -f docker-compose.yml up -d --force-recreate
```

或者：

```bash
docker-compose -f docker-compose.yml up -d --force-recreate
```

### 手动停止

```bash
docker compose -f docker-compose.yml down
```

或者：

```bash
docker-compose -f docker-compose.yml down
```

## 常见问题

### 1. 浏览器打开页面了，但实时录音不能用

先确认：

- 你访问的是 `https://...`
- 不是 `http://...`
- 自签证书已经导入或至少已被浏览器接受

网页端实时语音采集使用浏览器麦克风接口，不在 HTTPS 安全上下文下会被浏览器直接拦截。

### 2. 下载页能打开，但看不到终端安装包

检查：

```bash
ls runtime/downloads
```

确认安装包是否真的放在这个目录里。

### 3. 安装或升级失败

先看容器日志：

```bash
docker logs asr-all-in-one --tail 200
```

再确认 `.env` 里的外部 ASR 地址是否能从服务器访问。

### 4. admin 账号密码没生效

先确认：

- 你是在第一次安装前就已经改好了 `.env` 里的 `ASR_BOOTSTRAP_ADMIN_USERNAME` 和 `ASR_BOOTSTRAP_ADMIN_PASSWORD`
- 当前数据库里不存在同名管理员账号

注意：

- 系统只会在“同名用户不存在”时自动创建 admin
- 如果同名 admin 已经存在，后续再次执行 `sh install.sh` 或 `sh install.sh upgrade` 不会覆盖旧密码

如果当前还没有正式数据，最简单的处理方式是：

```bash
cd /data/asr/asr-all-in-one
vi .env
sh uninstall.sh purge
sh install.sh
```

如果已经有正式数据，则应通过后台改密，或在数据库中手工重置该管理员账号。

### 5. 我想换成自己的证书

直接把证书和私钥放到：

```bash
runtime/certs/tls.crt
runtime/certs/tls.key
```

然后重新执行：

```bash
sh install.sh upgrade
```

## 相关文件

- 根目录打包入口: [Makefile](Makefile)
- 发布脚本: [deploy/all-in-one/scripts/build-release.sh](deploy/all-in-one/scripts/build-release.sh)
- 安装/升级脚本: [deploy/all-in-one/scripts/install.sh](deploy/all-in-one/scripts/install.sh)
- 卸载脚本: [deploy/all-in-one/scripts/uninstall.sh](deploy/all-in-one/scripts/uninstall.sh)
- 发布说明: [deploy/all-in-one/README.md](deploy/all-in-one/README.md)