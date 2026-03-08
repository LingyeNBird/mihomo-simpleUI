# Mihomo WebUI Proxy

一个最小可运行的 Docker 应用：

- Go + Gin 后端
- React + Ant Design WebUI
- SQLite 存储订阅与节点选择
- Mihomo 作为独立容器提供代理能力

## 功能

- 添加 / 编辑 / 删除订阅
- 拉取订阅并保存为本地 YAML 文件
- 生成 Mihomo 主配置并尝试热加载
- 展示可切换的代理组
- 选择代理组当前节点，并持久化选择结果

## 目录

- `backend/` Go API 和配置生成
- `frontend/` WebUI
- `deploy/` Dockerfile 与 compose
- `data/config/` Mihomo 配置与订阅文件
- `data/db/` SQLite 数据库

## 本地开发

### 前端

```bash
cd frontend
npm install
npm run dev
```

### 后端

```bash
cd backend
go run ./cmd/server
```

默认前端通过 Vite 代理访问 `http://localhost:8080/api`。

## Docker 启动

```bash
cd deploy
docker compose up --build
```

可通过环境变量覆盖默认端口：

```bash
APP_PORT=8080 MIHOMO_MIXED_PORT=7890 MIHOMO_CONTROLLER_PORT=9090 docker compose up --build
```

PowerShell:

```powershell
$env:APP_PORT="8080"
$env:MIHOMO_MIXED_PORT="7890"
$env:MIHOMO_CONTROLLER_PORT="9090"
docker compose up --build
```

启动后：

- WebUI: `http://localhost:${APP_PORT:-8080}`
- Mihomo mixed-port: `127.0.0.1:${MIHOMO_MIXED_PORT:-7890}`
- Mihomo controller: `http://localhost:${MIHOMO_CONTROLLER_PORT:-9090}`

## 注意

- 目前是 MVP：单用户、手动刷新订阅、基础配置生成。
- 如果某个订阅格式不合法，后端会拒绝保存该远程内容。
- 如果 Mihomo controller 尚未可用，配置会先落盘，后续可在 WebUI 再次点击“同步配置”。
