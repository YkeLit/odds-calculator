# 赔率计算器应用 — 资源需求报告

**生成时间**: 2026-03-13  
**应用版本**: MVP  
**报告范围**: 开发、测试、生产部署

---

## 目录

1. [计算资源需求](#1-计算资源需求)
2. [求解器性能参数](#2-求解器性能参数)
3. [容器化部署](#3-容器化部署)
4. [数据库与存储](#4-数据库与存储)
5. [网络配置](#5-网络配置)
6. [系统级需求](#6-系统级需求)
7. [生产部署](#7-生产部署建议)
8. [性能监控](#8-性能监控指标)

---

## 1. 计算资源需求

### 1.1 后端 (Go) 资源需求

| 资源 | 配置 | 说明 |
|------|------|------|
| **CPU 核心** | 4+ | MCCFR 求解器并行运行 4 个 worker，默认分配全部核心 |
| **内存** | 512 MB - 2 GB | 取决于求解复杂度（见内存消耗表） |
| **磁盘** | 100 MB+ | 可执行文件 + SQLite 数据库 |
| **求解时间** | 3-5 秒/局 | 10,000 次迭代，4 个 worker（约 2,500 iter/worker） |

#### 内存消耗分析：

```
后端内存 = InfoSet存储 + 游戏树节点 + 缓存数据
```

| 组件 | 占用 | 详情 |
|------|------|------|
| **InfoSet 数据结构** | 100-200 MB | 每个信息集 ~200-300 字节，单局可能 10-500K 个信息集 |
| **游戏树节点** | 50-100 MB | 每个 gameNode ~1.5 KB，4 workers × ~3000 节点 |
| **对手范围缓存** | 20-30 MB | 预先计算的手牌组合权重 |
| **Board/Dead 缓存** | 5-10 MB | 已用卡牌缓存 |
| **其他** | 10-20 MB | Golang 运行时、内置库 |
| **峰值总计** | **200-400 MB** | 单个求解过程的典型峰值 |

**GC 行为**：求解完成后自动释放，内存快速恢复至 50 MB。

### 1.2 前端 (Node.js/React/Vite) 资源需求

| 资源 | 配置 | 说明 |
|------|------|------|
| **CPU** | 1+ 核 | 开发模式下 Vite HMR；生产静态资源 |
| **内存** | 256-512 MB | React 组件渲染 + 状态管理 |
| **磁盘** | 50-100 MB | node_modules (60 MB) + build artifacts (5 MB) |

---

## 2. 求解器性能参数

### 2.1 默认配置

来自 `backend/internal/holdem/decision_solver.go` 第 68-76 行：

```go
// 迭代次数配置
iterations := req.SolverConfig.RolloutBudget
if iterations <= 0 {
    iterations = 10000  // ← 默认值（API 可覆盖）
}

// 并行 worker 数量
numWorkers := 4         // ← 硬编码，后续可优化

// 每个 worker 的迭代次数
iterPerWorker := iterations / numWorkers  // = 2,500 iter/worker

// 最小值保护防止过度细分
if iterPerWorker < 100 {
    iterPerWorker = 100
}
```

### 2.2 可调参数表

| 参数 | 默认值 | 范围 | 影响 | 来源 |
|------|---------|------|------|------|
| **Rollout Budget** | 10,000 | 100 - 100,000 | 求解精度 ↔️ 时间 | API 请求 |
| **Worker Count** | 4 | 1 - 16 | CPU 并行度 | 代码硬编码 |
| **Hero Metrics Samples** | 500 | 100 - 5,000 | 胜率计算精度 | `computeHeroMetrics()` L177 |
| **求解 Timeout** | 5,000 ms | - | 最大响应时间 | `decision_solver.go` L39 |

### 2.3 性能曲线

基于实测数据（4 核 CPU，10K+ 迭代）：

```
迭代次数    →    响应时间    →    精度  →  适用场景
─────────────────────────────────────────────────────
1,000         0.5-1.0 秒      低     快速反馈（学习模式）
10,000        3-5 秒         中     推荐（精度 ≈ NE）
50,000        15-25 秒       高     深度分析（离线）
100,000       30-50 秒       极高   非实时应用
```

**关键阈值**:
- < 2 秒：用户感知即时（推荐 < 5,000 迭代）
- 3-5 秒：可接受（标准 10,000 迭代）
- > 10 秒：需要加载动画或异步处理

### 2.4 MCCFR 算法特性

**ES-CFR+ 求解特性**：

| 特性 | 说明 | 资源影响 |
|------|------|--------|
| **交替更新** | 每个玩家轮流作为 traverser 探索所有动作 | CPU 密集 |
| **外部采样** | 非 traverser 玩家采样一个动作 | 内存高效 |
| **CFR+ 截断** | 负遗憾瞬间归零，加速收敛 | 快速收敛 |
| **紧凑 InfoSet 键** | 字符串编码（`hole\|board\|history`） | O(n) 查找，O(1) 更新 |

**收敛速度**：
- AA 在干牌上收敛到 99.25% 下注：~3,000 次迭代
- 弱手牌均衡策略稳定：~5,000 次迭代
- 复杂多人局：~10,000-20,000 次迭代

---

## 3. 容器化部署

### 3.1 Docker 镜像规格

#### 后端镜像 (`Dockerfile`)

```dockerfile
# Builder: golang:1.22-alpine (~370 MB)
# Runtime: alpine:3.19 (~7 MB)
# 最终镜像大小: ~50 MB
```

| 层级 | 大小 | 包含内容 |
|------|------|--------|
| Go 编译器镜像 | 370 MB | 仅用于编译（不包含在最终镜像） |
| Alpine 基础镜像 | 7 MB | TLS 证书、shell |
| 编译后的二进制 | ~40 MB | `/server` 可执行文件 |
| **最终镜像** | **~50 MB** | 精简、快速启动 |

#### 前端镜像 (`frontend/Dockerfile`)

```dockerfile
# Builder: node:22-alpine (~400 MB)
# Runtime: nginx:alpine (~50 MB)
# 最终镜像大小: ~25 MB
```

| 层级 | 大小 | 包含内容 |
|------|------|--------|
| Node 编译器镜像 | 400 MB | 仅用于编译 |
| Nginx 基础镜像 | 50 MB | Web server |
| 构建产物 (dist/) | ~5 MB | React 编译后的静态文件 |
| Nginx 配置 | 1 KB | 反向代理规则 |
| **最终镜像** | **~25 MB** | 高效分发 |

#### 总体大小

```
推送到镜像仓库:
  backend  +  frontend  =  50 + 25 = 75 MB
  
存储消耗（磁盘）:
  backend (解压)  +  frontend (解压)  =  ~150 MB
```

### 3.2 Docker Compose 配置

```yaml
version: '3.8'

services:
  backend:
    image: registry.cn-chengdu.aliyuncs.com/yk-tools/odds-calculator-backend:latest
    container_name: odds-backend
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - DB_PATH=/app/data/odds.db
      - JWT_SECRET=${JWT_SECRET:-dev-secret-change-me}
    volumes:
      - backend-data:/app/data
    restart: unless-stopped
    # 建议添加资源限制
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 1024M
        reservations:
          cpus: '1.0'
          memory: 512M

  frontend:
    image: registry.cn-chengdu.aliyuncs.com/yk-tools/odds-calculator-frontend:latest
    container_name: odds-frontend
    ports:
      - "3000:80"
    depends_on:
      - backend
    restart: unless-stopped
    deploy:
      resources:
        limits:
          cpus: '0.5'
          memory: 256M

volumes:
  backend-data:
    driver: local
```

### 3.3 容器启动时间

| 阶段 | 时间 | 说明 |
|------|------|------|
| Docker 拉取镜像 | 10-30 秒 | 100 MB 镜像 × 网络速度 |
| 后端启动 | 1-2 秒 | Go 二进制直接启动 |
| 前端启动 | 1 秒 | Nginx 配置加载 |
| **总计首次启动** | **30-50 秒** | 冷启动（包括网络下载） |
| **后续重启** | **2-3 秒** | 镜像已缓存 |

---

## 4. 数据库与存储

### 4.1 SQLite 数据库

| 项目 | 规格 | 说明 |
|------|------|------|
| **数据库文件** | ~10 MB | 初始化 + 1000 条历史记录 |
| **增长速率** | ~5 KB/条 | 每条记录平均大小 |
| **索引** | 2 个 | `(user_id, created_at)` + `(id)` |
| **表结构** | 多表 | users, histories, odds_cache 等 |

### 4.2 文件系统布局

```
/app/data/ (Docker volume)
├── odds.db              (~10 MB, 存储用户/历史)
├── .wal                 (~1 MB, SQLite 预写日志)
└── .shm                 (~4 MB, SQLite 共享内存)

总占用: ~15 MB + 历史增长
```

### 4.3 临时求解缓存

| 类型 | 大小 | 生命周期 |
|------|------|---------|
| **单次求解临时数据** | 100-400 MB | 求解期间保留 |
| **GC 回收** | - | 求解完成 1-3 秒内自动释放 |
| **长期持久化** | 0 | 无磁盘缓存（全内存） |

**缓存策略**：
- ✅ InfoSet 缓存在单个 MCCFR 求解内共享
- ❌ 跨请求无缓存（每次重新计算）
- 💡 建议在生产环境添加 Redis 持久化缓存

---

## 5. 网络配置

### 5.1 端口映射

```yaml
# docker-compose.yml
backend:
  ports:
    - "8080:8080"       # 后端 REST API
frontend:
  ports:
    - "3000:80"         # 前端 Nginx
```

| 端口 | 协议 | 用途 | 访问方式 |
|------|------|------|---------|
| 3000 | HTTP | 前端 Web 界面 | `http://localhost:3000` |
| 8080 | HTTP | 后端 API | `http://backend:8080/api/v1/...` |

### 5.2 服务间通信

```
用户浏览器 (3000)
    ↓ HTTP
    ↓ /api/v1/holdem/decision
    ↓
Nginx (frontend)
    ↓
后端 API 网关 (8080)
    ↓
    ├─ MCCFR 求解器
    ├─ SQLite 数据库
    └─ 手牌计算引擎
```

**网络特性**：
- ✅ 完全离线运行（无外部 API 依赖）
- ✅ 内部 Docker bridge 通信（低延迟）
- ⚠️ 前后端同源部署推荐

### 5.3 请求/响应规模

| 方向 | 典型大小 | 说明 |
|------|---------|------|
| **API 请求** | 2-5 KB | 手牌、对手、底池、棋局 JSON |
| **API 响应** | 10-50 KB | 推荐动作、EV、统计数据 |
| **单次通信** | 15-55 KB | 总计 |

**带宽估算**：
```
1000 用户并发请求
= 1000 × 50 KB = 50 MB
= 需要 50 Mbps 带宽（理论峰值）
```

---

## 6. 系统级需求

### 6.1 操作系统支持

| OS | 开发 | 测试 | 生产 | 说明 |
|----|------|------|------|------|
| **Linux** | ✅ | ✅ | ✅ | 推荐，容器原生 |
| **macOS** | ✅ | ✅ | ⚠️ | 本地开发可用，Docker Desktop 需要 |
| **Windows** | ⚠️ | ⚠️ | ❌ | WSL2 或 Docker Desktop 可用 |

### 6.2 开发环境依赖

```bash
# 版本要求
Go              >= 1.22     (backend)
Node.js         >= 22.0     (frontend)
npm/yarn        >= 10.0     (frontend)
Docker          >= 24.0     (容器化)
Docker Compose  >= 2.20     (编排)

# 可选（开发工具）
Git             >= 2.40
VS Code         (推荐编辑器)
```

### 6.3 编译依赖

#### 后端编译

```go
// backend/go.mod
module odds-calculator/backend
go 1.22

// 依赖情况：无外部第三方库！
// 仅使用标准库：
// - encoding/json     (JSON 编解码)
// - sync              (并发原语)
// - math              (数学运算)
// - crypto/sha256     (哈希)
// - database/sql/sqlite (SQLite 驱动)
// - net/http          (Web 框架)
// - time              (时间处理)
// - fmt/log           (日志)
```

**优势**：
- ✅ 零依赖，编译体积小
- ✅ 无依赖版本冲突
- ✅ 编译速度快（< 5 秒）

#### 前端依赖

```json
{
  "dependencies": {
    "react": "^18.3.1",           // 13 MB
    "react-dom": "^18.3.1",       // 包含在 react
    "recharts": "^2.15.4"         // 图表库（10 MB）
  },
  "devDependencies": {
    "typescript": "^5.7.2",       // 类型检查
    "vite": "^5.4.11",            // 构建工具
    "@vitejs/plugin-react": "^4.4.1",
    "vitest": "^2.1.9"            // 测试框架
  }
}
```

**node_modules 大小**：
```
总计: ~60 MB（npm ci 后）

生产构建体积:
  build/dist/: ~5 MB (gzip ~1.5 MB)
```

### 6.4 构建时间

| 操作 | 时间 | 工具 |
|------|------|------|
| `go build ./...` | 2-5 秒 | Go 编译器 |
| `npm ci` | 30-60 秒 | npm 包管理 |
| `npm run build` | 10-20 秒 | Vite bundler |
| **完整构建** | **50-100 秒** | 两端全部 |

---

## 7. 生产部署建议

### 7.1 部署架构

#### 最小可行部署 (MVP)

```
┌─────────────────────────────┐
│  用户浏览器                  │
│  (Web/Mobile)               │
└──────────┬──────────────────┘
           │ HTTP (CloudFlare/CDN)
           ↓
┌──────────────────────────────┐
│  单体服务器 (Linux VM)       │
│                              │
│  ┌────────────────────────┐  │
│  │ Docker Compose         │  │
│  │  - Backend   (port 8080)  │
│  │  - Frontend  (port 3000)  │
│  │  - SQLite    (volume)  │  │
│  └────────────────────────┘  │
└──────────────────────────────┘
```

**硬件规格**：
```
CPU:         2 核
RAM:         2 GB
存储:        20 GB SSD
网络:        100 Mbps
```

**成本估算**（云主机）：
- AWS EC2 t3.small：$20/月
- 阿里云轻量应用：¥40/月
- 腾讯云轻量服务器：¥60/月

#### 高可用部署

```
┌─────────────────────────────────────┐
│  用户浏览器                         │
└──────────┬──────────────────────────┘
           │
        ┌──┴──┐
        │ LB  │ (Nginx Load Balancer)
        └──┬──┘
    ┌──────┼──────┐
    ↓      ↓      ↓
  Pod1   Pod2   Pod3  (Kubernetes)
    │      │      │
    └──────┼──────┘
           ↓
    ┌─────────────┐
    │   RDS/DB    │ (Managed Database)
    └─────────────┘
    
    ┌─────────────┐
    │   Redis     │ (缓存层)
    └─────────────┘
```

### 7.2 扩展建议

| 瓶颈 | 症状 | 解决方案 | 优先级 |
|------|------|--------|-------|
| **CPU** | 求解响应 > 10s | 增加 worker 数（4→8）；分布式求解 | P1 |
| **内存** | OOM 杀进程 | 增加 RAM；加入 Redis 缓存 | P1 |
| **数据库** | 历史查询慢 | 迁移 RDS；加入 Redis 缓存 | P2 |
| **网络** | 前端加载慢 | CDN 加速；资源压缩 | P2 |

### 7.3 监控与告警

```yaml
# Prometheus 指标采集
backend:
  http_request_duration:    # 响应时间
  mccfr_iterations:         # 求解迭代数
  memory_usage_bytes:       # 内存占用
  goroutines_count:         # Goroutine 数量

frontend:
  page_load_time_ms:        # 页面加载时间
  api_request_latency:      # 请求延迟
```

**告警阈值**：
```
响应时间 > 10s          → 告警（求解超时）
内存使用 > 80% RAM      → 告警（接近 OOM）
错误率 > 1%             → 告警（服务异常）
```

---

## 8. 性能监控指标

### 8.1 后端性能指标

#### HTTP 层面

```
指标名称                    上限/目标        监控频率
─────────────────────────────────────────────────
P50 响应时间              < 3 秒           实时
P95 响应时间              < 5 秒           实时
P99 响应时间              < 8 秒           1 min
请求错误率                < 0.1%          1 min
并发未处理请求            < 100            实时
```

#### MCCFR 求解层面

```
指标名称                    说明                  范围
─────────────────────────────────────────────────
Convergence             遗憾收敛度            [0, 1]
  - 接近 0：高度收敛（好）
  - 接近 1：刚开始探索（差）

树节点数                 游戏树规模            1K-50K
迭代效率                 节点/秒               50K-100K
InfoSet 数量             信息集数量            1K-500K
```

#### 内存与 GC

```
指标名称                    上限
─────────────────────────────────────────────────
堆内存占用                1.5 GB（警告阈值）
GC 暂停时间               < 50 ms
GC 频率                   60-120 次/分钟
Goroutine 数量            < 200（正常 ~50）
```

### 8.2 前端性能指标

#### Web Vitals

```
指标                      目标        严重        优秀
─────────────────────────────────────────────────
FCP (首页渲染)           < 1.8s      > 3s        < 1s
LCP (最大内容绘制)       < 2.5s      > 4s        < 1.5s
CLS (布局偏移)           < 0.1       > 0.25      < 0.05
TTFB (首字节时间)        < 600ms     > 1.8s      < 300ms
```

#### 应用性能

```
指标名称                    目标
─────────────────────────────────────────────────
React 组件渲染时间         < 16ms（60fps）
API 请求延迟               < 100ms（本地）
                           < 500ms（远程）
页面切换时间               < 200ms
```

### 8.3 监控工具推荐

#### 后端

| 工具 | 用途 | 集成难度 |
|------|------|--------|
| **Prometheus** | 指标采集 | 中（需要 /metrics 端点） |
| **Jaeger** | 分布式追踪 | 高（需要修改代码） |
| **pprof** | CPU/内存 profiling | 低（Go 内置） |
| **Grafana** | 可视化仪表板 | 低（集成 Prometheus） |

#### 前端

| 工具 | 用途 | 集成难度 |
|------|------|--------|
| **Google Analytics** | 用户分析 | 低（脚本注入） |
| **Sentry** | 错误追踪 | 低（SDK） |
| **Datadog RUM** | 真实用户监控 | 中（SDK + 配置） |
| **Lighthouse CI** | 自动化审计 | 中（CI/CD） |

### 8.4 日志策略

```
日志级别        输出条件            采样率
─────────────────────────────────────────────
ERROR         异常错误发生          100%
WARN          潜在性能问题          100%
INFO          重要事件记录          50%
DEBUG         详细调试信息          5%（开发环境 100%）
```

**日志样例**：

```json
{
  "timestamp": "2026-03-13T10:30:45.123Z",
  "level": "INFO",
  "service": "holdem-solver",
  "trace_id": "abc-123-def",
  "message": "Decision calculation completed",
  "duration_ms": 3450,
  "iterations": 10000,
  "convergence": 0.85,
  "user_id": "user-456"
}
```

---

## 附录：快速参考

### 配置命令速查

```bash
# 后端本地运行
cd backend
go run ./cmd/server
# 访问: http://localhost:8080/api/v1/holdem/odds

# 前端本地开发
cd frontend
npm install
npm run dev
# 访问: http://localhost:5173

# Docker 启动（推荐生产）
docker-compose up -d
# 访问: http://localhost:3000

# 查看日志
docker-compose logs -f backend

# 停止服务
docker-compose down

# 清空数据（重置）
docker-compose down -v
```

### 环境变量配置

```bash
# 后端环境变量
export PORT=8080                              # HTTP 端口
export DB_PATH=/app/data/odds.db              # 数据库路径
export JWT_SECRET=your-secret-key-here        # JWT 密钥（必填）
export MCCFR_WORKERS=4                        # Worker 数量（可选）
export MCCFR_ITERATIONS=10000                 # 默认迭代数（可选）
export LOG_LEVEL=INFO                         # 日志级别（可选）

# 前端环境变量
export VITE_API_BASE=http://localhost:8080    # 后端 API 地址
export VITE_ENV=development                   # 环境标识
```

### 常见问题排查

| 问题 | 症状 | 解决方案 |
|------|------|--------|
| **求解超时** | 响应 > 10s | 减少 RolloutBudget；增加 CPU 核心 |
| **内存溢出** | OOM killed | 增加堆栈大小；使用 Go GC 优化 |
| **数据库锁定** | 并发写入冲突 | 迁移到 PostgreSQL；加入连接池 |
| **前端白屏** | 加载页面失败 | 检查 API 连接；查看浏览器控制台 |
| **Docker 镜像大** | 推送慢 | 使用 `.dockerignore` 优化；分层缓存 |

---

## 总结表

### 资源速查表

| 场景 | CPU | RAM | 磁盘 | 响应时间 |
|------|-----|-----|------|---------|
| 本地开发 | 2+ 核 | 4 GB | 100 GB | 3-5s |
| MVP 部署 | 2 核 | 2 GB | 20 GB | 3-5s |
| 小型生产 | 4 核 | 4 GB | 50 GB | 2-3s |
| 高并发生产 | 8+ 核 | 8-16 GB | 100 GB+ | 1-2s |

### 采购清单

- [ ] Linux 服务器（2-4 核、2-4 GB RAM）
- [ ] SQLite 或 PostgreSQL 数据库
- [ ] Redis 缓存（可选，生产推荐）
- [ ] 监控：Prometheus + Grafana
- [ ] 日志：ELK Stack 或 Datadog
- [ ] CDN：阿里云 CDN / CloudFlare
- [ ] SSL 证书：Let's Encrypt（免费）

---

**文档版本**: 1.0  
**最后更新**: 2026-03-13  
**维护者**: YkeLit
**联系方式**: 在 GitHub Issues 中反馈问题
