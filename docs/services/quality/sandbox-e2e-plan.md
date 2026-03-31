# Sandbox E2E 联调计划（Phase-13）

目标：在不影响默认本地测试的前提下，建立可重复的 sandbox 联调入口，验证真实行情接入与交易链路一致性。

## 1. 范围

1. 本阶段只覆盖 `spot`。
2. 先做行情链路联调：`exchange ws -> adapter -> normalizer`。
3. 再做交易链路联调：`strategy -> risk -> execution -> adapter(rest)`（需要 sandbox 凭据）。
4. 合约与生产实盘不在本阶段范围内。

## 2. 测试分层

1. `sandbox-market-ingress`
- 目标：确认 Binance/OKX 实时行情能被系统读取并标准化。
- 通过标准：每个启用交易所在超时窗口内至少收到 1 条可解析行情。

2. `sandbox-trade-smoke`（后续补齐）
- 目标：确认 sandbox 下单与回报路径可用。
- 当前实现：下单 ack + 撤单 ack + 订单状态机一致性（ack -> canceled）。
- 通过标准：下单请求有 ack，撤单成功，状态机最终为 `canceled`。

## 3. 运行开关

1. `RUN_SANDBOX_TESTS=1` 开启 sandbox 测试。
2. 默认关闭，避免影响本地常规开发速度。
3. 建议额外参数：
- `SANDBOX_SYMBOL`（默认 `BTC-USDT`）
- `SANDBOX_BINANCE_WS`（默认 `wss://stream.binance.com:9443/ws`）
- `SANDBOX_OKX_WS`（默认 `wss://ws.okx.com:8443/ws/v5/public`）

4. 交易烟测参数（需显式设置）：
- `RUN_SANDBOX_TRADE_TESTS=1`
- `SANDBOX_TRADE_VENUE`（`binance`/`okx`，默认 `binance`）
- `SANDBOX_TRADE_SYMBOL`（默认 `BTC-USDT`）
- `SANDBOX_TRADE_PRICE`（必填，正数）
- `SANDBOX_TRADE_QTY`（必填，正数）
- `SANDBOX_BINANCE_API_KEY` / `SANDBOX_BINANCE_API_SECRET`
- `SANDBOX_OKX_API_KEY` / `SANDBOX_OKX_API_SECRET` / `SANDBOX_OKX_PASSPHRASE`
- `SANDBOX_OKX_SIMULATED`（默认 `true`，会发送 `x-simulated-trading: 1`）

## 4. 审计输出

1. 测试命令与结果日志。
2. 每个 venue 的收包数量、首包耗时、是否标准化成功。
3. 失败时记录失败阶段（连接/订阅/收包/解析）。

## 5. 风险控制

1. sandbox 测试全量隔离在 `test/sandbox`，不影响默认门禁。
2. 对外网抖动设置明确超时，避免测试无限等待。
3. 无凭据时跳过交易链路测试，不阻断普通研发流。
