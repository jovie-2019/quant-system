# 策略开发指南

## 概述

quant-system 的策略是一个实现了 `Strategy` 接口的 Go 结构体。系统负责行情推送、风控、下单、持仓管理，你只需专注于**交易逻辑**。

## Strategy 接口

```go
type Strategy interface {
    ID() string
    OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent
}
```

- `ID()` — 返回策略实例的唯一标识（如 `"grid-BTC-USDT"`）
- `OnMarket(evt)` — 每收到一个 tick 调用一次，返回下单意图列表（空 = 不下单）

## 输入：MarketNormalizedEvent

```go
type MarketNormalizedEvent struct {
    Venue      Venue    // "binance" / "okx"
    Symbol     string   // "BTC-USDT"
    BidPX      float64  // 最优买价
    BidSZ      float64  // 最优买量
    AskPX      float64  // 最优卖价
    AskSZ      float64  // 最优卖量
    LastPX     float64  // 最新成交价
    Sequence   int64    // 序列号
    SourceTSMS int64    // 交易所时间戳（毫秒）
}
```

## 输出：OrderIntent

```go
type OrderIntent struct {
    IntentID    string   // 唯一 ID，建议用 "{策略名}-{symbol}-{时间戳}-{序号}"
    Symbol      string   // 交易对
    Side        string   // "buy" / "sell"
    Price       float64  // 价格
    Quantity    float64  // 数量
    TimeInForce string   // "IOC" / "GTC"
}
```

> StrategyID 由 runtime 自动填充，不需要你设置。

## 开发步骤

### 1. 复制模板

```bash
cp -r internal/strategy/template internal/strategy/mystrategy
```

### 2. 实现策略逻辑

编辑 `internal/strategy/mystrategy/mystrategy.go`：

```go
package mystrategy

import "quant-system/pkg/contracts"

type Config struct {
    Symbol   string  `json:"symbol"`
    // 你的参数...
}

type Strategy struct {
    cfg Config
    // 你的状态...
}

func New(cfg Config) *Strategy {
    return &Strategy{cfg: cfg}
}

func (s *Strategy) ID() string {
    return "mystrategy-" + s.cfg.Symbol
}

func (s *Strategy) OnMarket(evt contracts.MarketNormalizedEvent) []contracts.OrderIntent {
    if evt.Symbol != s.cfg.Symbol {
        return nil
    }

    // 你的交易逻辑...
    // 返回 []OrderIntent 下单，返回 nil 不下单

    return nil
}
```

### 3. 注册策略类型

创建 `internal/strategy/mystrategy/register.go`：

```go
package mystrategy

import (
    "encoding/json"
    "fmt"
    "quant-system/internal/strategy"
)

func init() {
    strategy.RegisterType("mystrategy", func(raw json.RawMessage) (strategy.Strategy, error) {
        var cfg Config
        if err := json.Unmarshal(raw, &cfg); err != nil {
            return nil, fmt.Errorf("mystrategy: %w", err)
        }
        return New(cfg), nil
    })
}
```

### 4. 导入注册

在 `cmd/strategy-runner/main.go` 中添加空导入：

```go
import _ "quant-system/internal/strategy/mystrategy"
```

### 5. 编译 & 部署

```bash
# 重新构建镜像
docker compose build strategy-runner

# 在后台页面创建策略配置：
#   策略类型: mystrategy
#   参数: {"symbol": "BTC-USDT", ...}
#   得到配置 ID（如 ID=2）

# 在 docker-compose.yml 添加：
#   strategy-mystrategy-btc:
#     build: .
#     entrypoint: ["/app/strategy-runner"]
#     environment:
#       STRATEGY_CONFIG_ID: "2"
#       MYSQL_DSN: ${MYSQL_DSN}
#       NATS_URL: ${NATS_URL}
#     depends_on:
#       mysql: { condition: service_healthy }
#       nats: { condition: service_healthy }
#     networks:
#       - quant-net

# 只启动新容器（不影响旧策略）
docker compose up -d strategy-mystrategy-btc
```

## 策略设计要点

### 状态管理
- `OnMarket` 会被高频调用，保持轻量
- 所有状态保存在 struct 字段中，Strategy 是有状态的
- 不需要考虑并发安全（同一策略实例是单线程调用的）

### IntentID 唯一性
- 必须全局唯一，否则风控会视为重复
- 推荐格式：`"{策略名}-{symbol}-{毫秒时间戳}-{序号}"`

### 风控参数
在策略配置的 JSON 中添加 `risk` 字段，由系统自动应用：
```json
{
    "symbol": "BTC-USDT",
    "risk": {
        "max_order_qty": 0.1,
        "max_position": 1.0,
        "daily_loss_limit": 500
    }
}
```

### 频率分类

| 类型 | tick 处理时间 | 适用场景 |
|------|:----------:|---------|
| 高频 | < 1ms | 盘口价差、微结构 |
| 中频 | < 10ms | 动量、均值回归 |
| 低频 | < 100ms | 趋势跟踪、网格 |

当前系统对三种频率都支持。高频策略注意避免在 `OnMarket` 中做 I/O 操作。

### 参数热更新

后台修改策略参数后，strategy-runner 会在 5 秒内检测到变更并重新创建策略实例。新实例的状态从零开始（如滑动窗口会重新填充）。

## 调试

```bash
# 查看策略容器日志
docker compose logs -f strategy-mystrategy-btc

# 查看策略产生的信号（NATS subject）
# strategy.intent.mystrategy-BTC-USDT

# Grafana 上查看 momentum 指标
# engine_strategy_momentum_signal_total
# engine_strategy_momentum_eval_total
```

## 示例：已有策略

- **动量突破（momentum）**：`internal/strategy/momentum/` — 滚动窗口 + 价格突破信号
