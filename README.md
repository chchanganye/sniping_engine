# sniping_engine (monorepo)

本仓库采用 Monorepo 结构：

- `frontend/`：Vue3 + Vite 前端
- `backend/`：Go 后端（HTTP + WebSocket + SQLite）

## 开发

### 前端

```bash
cd frontend
npm install
npm run dev
```

### 后端

```bash
cd backend
go run ./cmd/mock
go run ./cmd/server -config ./config.yaml
```

