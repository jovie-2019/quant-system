# quant-system

现货量化交易系统 — Go 后端 + React 管理前端 + Docker Compose 本机部署。

支持 Binance / OKX 多交易所接入，内置动量突破策略框架，完整的风控、订单状态机、持仓管理和可观测性。

## 架构

```
┌─────────────┐    ┌──────────────┐    ┌──────────────┐
│ market-ingest│───▶│     NATS     │◀───│strategy-runner│
│  (行情接入)   │    │  JetStream   │    │  (策略执行)    │
└─────────────┘    └──────┬───────┘    └──────────────┘
                          │
                   ┌──────▼───────┐
                   │ engine-core  │
                   │ risk→exec→fsm│───▶ Binance/OKX REST
                   │  →position   │
                   └──────┬───────┘
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
           MySQL     Prometheus    admin-api
                      Grafana    (管理后台+Web UI)
```

## Quick Start

### 前置条件

- Docker + Docker Compose
- Go 1.26+（开发调试用）
- Node.js 20+（前端开发用）

### 一键启动

```bash
# 1. 复制配置文件
cp .env.example .env
# 编辑 .env，填入你的 AES_KEY、JWT_SECRET、ADMIN_PASSWORD_HASH 等

# 2. 启动所有服务
docker compose up -d

# 3. 检查状态
docker compose ps
```

### 访问

| 服务 | 地址 | 说明 |
|------|------|------|
| 管理后台 | http://localhost:8090 | 登录密码见 .env |
| Grafana | http://localhost:3001 | 默认 admin/admin |
| Prometheus | http://localhost:9090 | 指标查询 |
| engine-core | http://localhost:8080/metrics | 交易引擎指标 |
| strategy-runner | http://localhost:8081/metrics | 策略引擎指标 |
| market-ingest | http://localhost:8082/metrics | 行情接入指标 |

### 开发模式

```bash
# 后端
go build ./...
go test ./... -race

# 前端
cd web && npm install && npm run dev
# 访问 http://localhost:3000（自动代理 API 到 8090）
```

## 项目结构

```
cmd/
  engine-core/       交易引擎（风控→下单→状态机→持仓）
  strategy-runner/   策略执行器（消费行情→产生信号）
  market-ingest/     行情接入（Binance/OKX WebSocket→NATS）
  admin-api/         管理后台 API + Web UI 静态托管
internal/
  adapter/           交易所适配器（REST签名下单 + WebSocket行情）
  adminapi/          管理后台 API handlers
  adminstore/        管理数据持久化
  backtest/          回测引擎
  book/              本地订单簿
  bus/natsbus/       NATS JetStream 封装
  controlapi/        交易引擎控制 API
  crypto/            AES-256-GCM 加解密
  execution/         订单执行（重试、对账）
  hub/               行情分发
  normalizer/        数据标准化
  notify/            飞书通知
  obs/               可观测性（日志、指标、TTL缓存）
  orderfsm/          订单状态机
  pipeline/          交易管道编排
  position/          持仓管理
  risk/              风控引擎
  store/mysqlstore/  MySQL 持久化
  strategy/          策略接口 + 动量突破策略
  strategyrunner/    策略运行时循环
web/                 React + Ant Design 管理前端
deploy/              Docker Compose + Prometheus + Grafana
```

## 管理后台功能

- 交易所管理（Binance/OKX）
- API Key 管理（AES-256-GCM 加密存储）
- 策略配置与启停控制
- 实时持仓监控
- 历史订单查询
- 风控参数管理
- Grafana 监控面板
- 飞书告警推送

## 文档

- [架构设计](docs/architecture-v1.md)
- [API 契约](docs/api-contract.md)
- [事件契约](docs/services/contracts/event-contracts-v1.md)
- [运维手册](docs/runbook-v1.md)
- [发布清单](docs/release-checklist-v1.md)

## 环境变量

详见 [.env.example](.env.example)

## License

Private — All rights reserved.
