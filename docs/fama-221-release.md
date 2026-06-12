# 192.168.40.221 Fama 发布流程

本文只适用于 221 服务器上的 Fama 项目：`ubuntu@192.168.40.221:/data/ganttly/fama`。

## 目录约定

| 路径 | 用途 |
| --- | --- |
| `/data/ganttly/fama` | 服务器上的源码工作区，只用于同步代码和执行打包命令。 |
| `/data/ganttly/releases/fama/<版本号>` | 标准发布产物目录，存放 `.run` 和 `.run.partNNN`。 |
| `/data/ganttly/jusha-asr-business` | 当前业务服务安装目录，保存 `.env`、`runtime` 和 `backups`。 |

不要把 `OUTPUT_DIR` 指向 `/data/ganttly`、`/data/ganttly/fama` 或 `/data/ganttly/jusha-asr-business`。打包输出目录必须和安装运行目录分开。

## 代码同步规则

本机仓库 `/home/lgt/asr` 是源码准绳。凡是修改代码、脚本或文档，先在本机仓库完成，再把明确改过的文件同步到 221 的 `/data/ganttly/fama`。不要在服务器上直接做长期代码修改。

同步示例：

```bash
/home/lgt/.codex/skills/asr-server-sync/scripts/sync_to_server.sh \
  Makefile \
  docs/fama-221-release.md
```

## 打包

在 221 服务器执行：

```bash
cd /data/ganttly/fama
make release-fama-business VERSION=0.10.3 JUSHA_ASR_PART_SIZE=2g
```

等价完整命令：

```bash
cd /data/ganttly/fama
JUSHA_ASR_PART_SIZE=2g make release-jusha-business \
  VERSION=0.10.3 \
  OUTPUT_DIR=/data/ganttly/releases/fama/0.10.3 \
  SERVER_HOST=192.168.40.221
```

输出目录中必须同时存在：

```text
jusha-asr-business-0.10.3.run
jusha-asr-business-0.10.3.run.part001
...
```

## 覆盖安装

当前运行中的业务实例挂载目录是 `/data/ganttly/jusha-asr-business/runtime/...`，所以覆盖安装必须把目标父目录指定为 `/data/ganttly`。

推荐命令：

```bash
cd /data/ganttly/fama
make install-fama-business VERSION=0.10.3
```

等价完整命令：

```bash
cd /data/ganttly/releases/fama/0.10.3
ASR_RUN_TARGET_DIR=/data/ganttly sh ./jusha-asr-business-0.10.3.run
```

安装包会覆盖 `/data/ganttly/jusha-asr-business` 中的发布文件，并保留现有 `.env`、`runtime` 和 `backups`。安装脚本会在升级前备份配置、证书和 runtime 数据，加载新镜像，重启容器并等待健康检查。

## 禁止操作

- 不要使用 `OUTPUT_DIR=/data/ganttly` 或 `OUTPUT_DIR=../`。
- 不要把发布产物直接散放在 `/data/ganttly`。
- 不要手动 `cp -r` 覆盖 `/data/ganttly/jusha-asr-business`。
- 不要只移动 `.run`，必须保留同目录下所有 `.run.partNNN`。
- 不要在 `/data/ganttly/releases/fama/<版本号>` 中直接执行 `.run` 而不设置 `ASR_RUN_TARGET_DIR=/data/ganttly`。

## 验证

```bash
cat /data/ganttly/jusha-asr-business/.release-manifest
docker ps --filter name=jusha-asr-business
docker logs --tail 100 jusha-asr-business
```

如果健康检查失败，安装脚本会尝试回滚到上一版镜像。运行数据仍以 `/data/ganttly/jusha-asr-business/runtime` 为准。
