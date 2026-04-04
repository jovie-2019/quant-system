# quant-system 项目规范

## 项目概述
现货量化交易系统，Go 后端 + React 前端 + Docker Compose 本机部署。

## 技术栈
- 后端：Go 1.26, MySQL 8, NATS JetStream, Prometheus, Grafana
- 前端：React 18 + TypeScript + Vite + Ant Design 5
- 部署：Docker Compose（本机工作站，无云服务）
- CI：GitHub Actions
- 通知：飞书 Webhook

## 代码规范
- Go: `go fmt`, `go vet`, 无 lint 警告
- 所有导出函数必须有简短注释
- 错误使用 sentinel error（`var ErrXxx = errors.New(...)`）
- Config 结构体必须有合理默认值
- 模块间通过接口通信，禁止跨包引用具体类型
- 测试文件与源文件 1:1 对应
- 前端：ESLint + Prettier，组件使用函数式 + hooks

## 目录结构
```
cmd/
  engine-core/       # 交易引擎
  strategy-runner/   # 策略执行器
  market-ingest/     # 行情接入
  admin-api/         # 管理后台 API
internal/
  crypto/            # AES 加解密
  notify/            # 飞书通知
  adminstore/        # 管理数据持久化（exchanges, api_keys, strategy_configs）
  pipeline/          # 交易管道
  ...existing modules...
web/                 # React 前端项目
deploy/
  docker-compose.yml # 一键启动所有服务
  prometheus/        # Prometheus 配置
  grafana/           # Grafana provisioning
pkg/contracts/       # 共享类型定义
```

## 安全规范
- API Key 数据库中 AES-256-GCM 加密存储
- AES 密钥从 `.env` 文件读取，**不入 Git**
- 管理后台 JWT 认证，密码 bcrypt 哈希
- `.env` 在 `.gitignore` 中

## API 设计规范
- RESTful，JSON 请求/响应
- 统一错误格式：`{"error": "code", "message": "..."}`
- 分页：`?page=1&page_size=20`
- 认证：`Authorization: Bearer <jwt>`

## 测试规范
- `go test ./... -race` 必须全过
- 新模块必须有对应 _test.go
- 集成测试通过 build tag 或环境变量控制

## Git 规范
- 提交消息格式：`type: short description`
- type: feat/fix/refactor/test/docs/chore
- 不提交 .env、密钥、二进制文件
