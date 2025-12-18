# sniping_engine (合规骨架)

这是一个 Go 后端骨架项目：
- SQLite 持久化（账号/目标清单/任务配置）
- TaskEngine 并发模型（多商品并行、账号轮询、全局/单账号限流）
- WebSocket 实时日志推送（Go 内部日志通过 Channel 广播到前端）
- Provider 插拔式设计（内置 `StandardProvider` 示例，Resty 调用链路指向 mock 地址）

说明：本项目不内置任何第三方站点的“抢购/扫货”实现细节，Provider 仅提供可替换模板。

## 快速开始

1) 启动 mock（可选，用于本地演示 Provider 调用链路）

```bash
go run ./cmd/mock
```

2) 启动服务

```bash
go run ./cmd/server -config ./config.yaml
```

3) WebSocket 日志

- WS 地址：`ws://127.0.0.1:8090/ws`
- 收到的消息为 JSON，`type=log` 或 `type=task_state`

## REST API（供前端调用）

- 账号：`GET/POST/DELETE /api/v1/accounts`
- 目标清单：`GET/POST/DELETE /api/v1/targets`
- 引擎：`POST /api/v1/engine/start`、`POST /api/v1/engine/stop`、`GET /api/v1/engine/state`
- 邮件通知：`GET/POST /api/v1/settings/email`、`POST /api/v1/settings/email/test`
- 上游 API 代理（保持原始 `/api/...` 路径与 payload，不在前端直连第三方）：
  - 任何非 `/api/v1/*` 的请求会由后端转发到 `provider.baseURL`。
  - 代理请求需要带 `Authorization: Bearer <token>`（或 `token/x-token`），后端用它匹配账号并保持 Cookie/UA/Proxy 一致。

## 目录结构

- `cmd/server`：HTTP + WS 服务入口
- `cmd/mock`：mock Provider 服务（本地演示）
- `internal/config`：配置读取
- `internal/store/sqlite`：SQLite 存储
- `internal/logbus`：日志总线（ring buffer + channel）
- `internal/ws`：WebSocket hub（多客户端广播）
- `internal/provider`：Provider 接口
- `internal/provider/standard`：Resty 模板 Provider（指向 mock）
- `internal/engine`：TaskEngine（并发/限流/任务执行）
- `internal/httpapi`：REST/WS 路由与处理器
