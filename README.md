# 赔率计算器 MVP

前后端分离实现，支持：

- 德州扑克：精确枚举胜率（`/holdem/odds`）
- 德州扑克全下 EV：主池/多级边池 + 可选抽水（`/holdem/allin-ev`）
- 四川麻将（血战到底 + 缺一门）：听牌/胡牌概率 + 两摸番数期望出牌建议（`/mahjong/analyze`）
- 简单登录（用户名密码 + JWT）
- 历史记录（按用户持久化到 SQLite）

## 项目结构

- `backend/`: Go REST API + 计算引擎 + SQLite 存储
- `frontend/`: React + TypeScript + Vite 页面

## 本地运行

### 1) 启动后端

```bash
cd backend
go test ./...
go run ./cmd/server
```

默认监听 `http://localhost:8080`。

可选环境变量：

- `PORT` (默认 `8080`)
- `DB_PATH` (默认 `./odds.db`)
- `JWT_SECRET` (默认 `dev-secret-change-me`)

### 2) 启动前端

```bash
cd frontend
npm install
npm run dev
```

默认开发地址 `http://localhost:5173`。

如果后端不是 `http://localhost:8080`，可设置：

```bash
VITE_API_BASE_URL=http://your-host:port npm run dev
```

## 输入格式说明

### 德州扑克玩家输入（前端）

每行：`id 牌1 牌2 投入金额 是否全下`

示例：

```txt
p1 As Ah 100 true
p2 Kc Kd 50 true
p3 Qc Qd 25 true
```

牌格式：`2-9/T/J/Q/K/A + s/h/d/c`，例如 `As`, `Td`, `7h`。

### 四川麻将牌格式

支持：`m1..m9`, `p1..p9`, `s1..s9`。

示例：`m1 m1 m2 m2 m3 m3 p4 p4 p5 p5 s6 s6 s7 s8`

## API 概览

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/holdem/odds`
- `POST /api/v1/holdem/allin-ev`
- `POST /api/v1/mahjong/analyze`
- `GET /api/v1/history?page=1&pageSize=20&gameType=`

## 测试

后端测试：

```bash
cd backend
go test ./...
```

前端构建检查：

```bash
cd frontend
npm run build
```
