<div align="center">

<img alt="DCC logo" width="120" height="120" src="web/static/favicon.svg" />

<h2>Docker Control Center</h2>

<p>单文件 Docker Web 管理面板 —— 容器、镜像、存储卷与 Compose 项目管理</p>

![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)
![TailwindCSS](https://img.shields.io/badge/TailwindCSS-3.4-38B2AC?logo=tailwindcss&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux-lightgrey?logo=debian&logoColor=white)
![License](https://img.shields.io/badge/license-LGPL--3.0-blue)

</div>

---

## 功能特性

- 🐳 **容器管理** —— 列表与详情、启动 / 停止 / 重启 / 删除，`latest` 容器可一键更新（拉取镜像后同配置重建，原容器自动备份）
- 📦 **镜像管理** —— 列表与详情、单个 / 多选批量删除、拉取新镜像；无容器使用时可更新（拉取新镜像后删除旧镜像）
- 💾 **存储卷管理** —— 列表与详情、单个 / 多选批量删除
- 🧩 **Compose 项目** —— 导入项目目录统一管理，支持启动 / 停止 / 重启 / 更新（pull → down → up -d，可选清理旧镜像）
- 🔐 **登录鉴权** —— 首次启动自动生成强随机密码，登录失败限流，全站 CSRF 防护
- 🌐 **多语言** —— 中文（默认）/ English 界面切换
- 🔄 **配置热重载** —— 修改 `config.yaml` 保存即生效，覆盖监听端口、SSL、凭据、日志开关
- 🔒 **HTTPS** —— 默认关闭，可选同端口保留 HTTP，证书支持绝对 / 相对路径，续期免重启
- 🪵 **系统日志** —— 默认开启，级别可配置（debug / info / warn / error，默认 warn；info 级别可记录操作日志），可在界面查看
- 📥 **一键安装 Docker** —— 未安装 Docker 的机器登录后可直接执行官方安装脚本（仅 Linux）
- 🪶 **单文件部署** —— 前端经 `go:embed` 打包进二进制，运行时第三方依赖仅 `gopkg.in/yaml.v3`

## 快速开始

### 环境要求

- Debian 11+ / Ubuntu 20.04+（生产环境）
- Docker Engine；Compose 项目管理需要 `docker compose`（v2）或 `docker-compose`（v1）
- 需要访问 `/var/run/docker.sock`，请以 root 或 `docker` 组用户运行

### 安装与运行

```bash
# 构建（见「开发」一节）或直接获取二进制后：
mkdir -p /opt/dcc && cp dcc /opt/dcc/ && cd /opt/dcc
chmod +x dcc
./dcc
```

首次启动会在程序目录生成 `config.yaml`。以前台交互方式运行时，控制台输出登录用户名与密码：

```
============================================================
 Docker Control Center (DCC) 首次启动
 已自动生成配置文件: /opt/dcc/config.yaml
 登录用户名: admin
 登录密码: xxxxxxxxxxxxxxxx
============================================================
```

浏览器访问 `http://<host>:8080` 登录即可。

> 以 `&`、`nohup` 或 systemd 等方式后台运行时程序静默运行、不输出控制台信息；启动信息与首次生成的密码仅写入 `config.yaml` 与 `logs/dcc.log`。

### systemd 部署

```bash
cp deploy/dcc.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now dcc
```

## 配置

`config.yaml` 于首次启动自动生成，修改保存后自动热重载。所有相对路径相对配置文件所在目录。

```yaml
version: 1              # 配置结构版本，旧版本自动迁移，勿手改

server:
  host: ""              # 监听地址，留空监听所有网卡
  port: 8080

auth:
  username: admin
  password: "xxxx"      # 首次启动随机生成，长度不小于 8

logging:
  enabled: true         # 日志开关
  level: warn           # 日志级别：debug / info / warn / error（默认 warn）
                        # info 级别可记录操作日志；首次生成的密码不受级别过滤影响
  file: logs/dcc.log
  max_size_mb: 5        # 单文件上限，超出轮转
  max_backups: 3

ssl:
  enabled: false        # HTTPS，默认关闭
  cert_dir: ""          # 证书目录（含 cert.pem / key.pem），绝对或相对路径
  cert_file: ""         # 或直接指定证书与私钥，优先于 cert_dir
  key_file: ""
  keep_http: false      # 启用 HTTPS 后是否在同端口保留 HTTP

docker:
  host: ""              # 留空为 unix:///var/run/docker.sock
                        # 远程守护进程示例：tcp://host:port

compose_projects:       # 也可通过界面导入 / 移除
  - name: myapp
    path: /opt/myapp
```

| 配置变化 | 热重载行为 |
| --- | --- |
| `auth.*` | 立即生效，下次登录使用新凭据 |
| `logging.*` | 立即生效 |
| `server.*` / `ssl.*` | 重建监听器，失败自动回滚旧配置 |
| `docker.host` | 重建 Docker 连接 |
| `compose_projects` | 立即生效 |

自签证书测试：

```bash
mkdir -p certs
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout certs/key.pem -out certs/cert.pem -subj "/CN=dcc.local"
```

> 生产环境请使用可信证书，并将 `keep_http` 设为 `false`。

## 更新行为

| 操作 | 执行内容 | 确认 |
| --- | --- | --- |
| 容器更新（仅 `latest`） | pull 镜像 → 停止 → 原容器备份重命名（`<名称>.dccbak-<时间戳>`）→ 同配置重建并启动 | 拉取前确认 |
| 镜像更新 | 校验无容器使用 → pull → 删除旧镜像（内容无变化则跳过） | 拉取前确认 |
| Compose 更新 | pull → down → up -d →（可选）删除未使用的旧镜像 | 询问是否删除旧镜像并确认 |

> 容器重建保留环境变量、命令与入口、标签、端口绑定、挂载、重启策略、资源限制、网络与静态 IP、健康检查等；重建失败自动回滚原容器名称。

## 开发

### 环境要求

- **Go** >= 1.22（建议使用最新补丁版本）
- **Node.js** >= 18
- **pnpm** >= 9

### 构建

```bash
make    # pnpm install + pnpm build + go build
```

> 交叉编译其他平台：`GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dcc .`

### 其他命令

```bash
go test ./...          # 全部测试
go vet ./...           # 静态检查
govulncheck ./...      # 漏洞扫描（需 go install golang.org/x/vuln/cmd/govulncheck@latest）
pnpm --dir web audit   # 前端依赖审计
```

### 项目结构

| 目录 | 说明 |
| --- | --- |
| `main.go` | 入口：配置生成、热重载、优雅退出 |
| `internal/config/` | config.yaml 加载、生成、校验、写回、热重载 |
| `internal/docker/` | Docker Engine API 客户端、Compose 执行器 |
| `internal/jobs/` | 异步任务与输出管理（拉取 / Compose / 安装） |
| `internal/logging/` | slog 日志，按大小轮转 |
| `internal/server/` | 路由、鉴权、CSRF、中间件、API、静态资源 |
| `web/` | 前端源码与构建配置（产物经 go:embed 打包） |
| `deploy/` `Makefile` | systemd 单元与构建 |

## API

响应格式：`{"ok":true,"data":...}` 或 `{"ok":false,"error":{"code":"err.xxx","message":"..."}}`。
写操作需携带会话 Cookie 与 `X-CSRF-Token`；长耗时操作返回 `jobId`，经 `GET /api/jobs/{id}` 轮询。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/session` | 会话状态、CSRF Token |
| POST | `/api/login`、`/api/logout` | 登录、退出 |
| GET | `/api/status` | 应用与 Docker 状态 |
| POST | `/api/docker/install` | 安装 Docker（任务） |
| GET | `/api/containers`、`/api/containers/{id}` | 容器列表、详情 |
| POST | `/api/containers/{id}/start\|stop\|restart\|update` | 容器操作 |
| DELETE | `/api/containers/{id}` | 删除容器 |
| GET | `/api/images`、`/api/images/detail?ref=` | 镜像列表、详情 |
| POST | `/api/images/pull`、`/api/images/update` | 拉取、更新镜像（任务） |
| DELETE | `/api/images` | 批量删除镜像 |
| GET | `/api/volumes`、`/api/volumes/detail?name=` | 存储卷列表、详情 |
| DELETE | `/api/volumes` | 批量删除存储卷 |
| GET / POST / DELETE | `/api/compose/projects` | 项目列表、导入、移除 |
| POST | `/api/compose/action` | Compose 操作（任务） |
| GET | `/api/jobs`、`/api/jobs/{id}` | 任务列表、详情 |
| POST | `/api/jobs/{id}/cancel` | 取消任务 |
| GET | `/api/system/logs` | 日志尾部 |
| GET | `/api/config/view` | 配置视图（脱敏） |
| POST | `/api/config/reload`、`/api/config/auth` | 重载配置、修改凭据 |

## 故障排查

| 现象 | 处理 |
| --- | --- |
| 提示未检测到可用的 Docker | 检查 `systemctl status docker`，或使用界面安装按钮；远程守护进程可在 config.yaml 配置 `docker.host` |
| 修改配置后未生效 | 查看 `logs/dcc.log` 中的热重载报错；校验失败时保持旧配置 |
| HTTPS 启动失败 | 确认 `cert_dir` 下存在 `cert.pem` / `key.pem` 且有读取权限 |
| 提示未检测到 docker compose | 安装 compose v2 插件或 `docker-compose` |
| 忘记密码 | 删除 `config.yaml` 后重启会生成新密码 |

## 致谢

- [gopkg.in/yaml.v3](https://gopkg.in/yaml.v3) —— 配置文件解析
- [Tailwind CSS](https://tailwindcss.com/) —— 前端样式工具链
- [Docker Engine API](https://docs.docker.com/engine/api/) / [Docker Compose](https://docs.docker.com/compose/) —— 容器编排能力来源
- [get.docker.com](https://get.docker.com) —— 官方便捷安装脚本

## 开源许可

本项目基于 [GNU Lesser General Public License v3.0 (LGPL-3.0)](https://www.gnu.org/licenses/lgpl-3.0.html) 许可开源，完整许可文本见 [LICENSE](./LICENSE)（LGPL-3.0）与 [LICENSE.GPL-3.0](./LICENSE.GPL-3.0)。

- **修改与分发：** 对本项目的修改与分发须同样基于 **LGPL-3.0** 许可，并提供完整源代码。
- **派生作品：** 须采用 **LGPL-3.0** 或兼容许可，并保留本项目的许可与版权信息。
- **链接使用：** 允许在专有软件中通过链接方式使用本项目，无须开放宿主软件源代码；但须保证用户可替换或重新链接本项目的库部分（静态链接时须随附目标文件或允许重新链接）。
- **商业用途：** 允许商业使用，前提是遵守上述条款并提供本项目源代码的获取途径。
- **免责：** 本软件按「现状」提供，不附带任何形式的担保，详见 LGPL-3.0 与 GPL-3.0 全文。

## 免责声明

本项目通过 root / `docker` 组权限访问 Docker 守护进程，可对容器、镜像与数据卷执行删除等不可逆操作。使用者须自行确认操作对象与影响范围，并确保使用行为符合相关法律法规及服务协议。对于因使用本项目产生的任何直接或间接后果，作者不承担任何责任。

本项目与 Docker, Inc. 不存在任何关联，亦未获得其认可或赞助。

This project is not affiliated with, endorsed by, or sponsored by Docker, Inc.
