# Admin API 契约文档 v1

Base URL: `http://localhost:8090`

## 认证

除 `/api/v1/health`、`/metrics`、`/api/v1/auth/login` 外，所有端点需要 JWT 认证。

```
Authorization: Bearer <token>
```

### POST /api/v1/auth/login

**Request:**
```json
{"password": "your-password"}
```

**Response 200:**
```json
{"token": "eyJ...", "expires_at": "2026-04-05T12:00:00Z"}
```

**Response 401:**
```json
{"error": "invalid_credentials", "message": "wrong password"}
```

---

## 交易所管理

### GET /api/v1/exchanges

**Response 200:**
```json
[
  {"id": 1, "name": "my-binance", "venue": "binance", "status": "active", "created_ms": 1712000000000, "updated_ms": 1712000000000}
]
```

### POST /api/v1/exchanges

**Request:**
```json
{"name": "my-binance", "venue": "binance"}
```

**Response 201:**
```json
{"id": 1, "name": "my-binance", "venue": "binance", "status": "active", "created_ms": 1712000000000, "updated_ms": 1712000000000}
```

### GET /api/v1/exchanges/{id}

**Response 200:** 同上单个对象
**Response 404:** `{"error": "not_found", "message": "exchange not found"}`

### PUT /api/v1/exchanges/{id}

**Request:**
```json
{"name": "my-binance-updated", "venue": "binance", "status": "disabled"}
```

### DELETE /api/v1/exchanges/{id}

**Response 200:** `{"status": "deleted"}`

---

## 账户 (API Key) 管理

### GET /api/v1/accounts

**Response 200:**
```json
[
  {
    "id": 1,
    "exchange_id": 1,
    "label": "main-key",
    "api_key": "****Ab1x",
    "api_secret": "********",
    "passphrase": "********",
    "permissions": "trade,read",
    "status": "active",
    "created_ms": 1712000000000,
    "updated_ms": 1712000000000
  }
]
```

### POST /api/v1/accounts

**Request（明文输入，服务端加密存储）:**
```json
{
  "exchange_id": 1,
  "label": "main-key",
  "api_key": "your-real-api-key",
  "api_secret": "your-real-api-secret",
  "passphrase": "your-passphrase-if-okx"
}
```

**Response 201:** 返回脱敏后的对象

### GET /api/v1/accounts/{id}

**Response 200:** 同上（脱敏）

### DELETE /api/v1/accounts/{id}

**Response 200:** `{"status": "deleted"}`

---

## 策略管理

### GET /api/v1/strategies

**Response 200:**
```json
[
  {
    "id": 1,
    "strategy_id": "momentum-btc",
    "strategy_type": "momentum",
    "exchange_id": 1,
    "api_key_id": 1,
    "config": {"symbol": "BTC-USDT", "window_size": 20, "breakout_threshold": 0.001, "order_qty": 0.01},
    "status": "stopped",
    "created_ms": 1712000000000,
    "updated_ms": 1712000000000
  }
]
```

### POST /api/v1/strategies

**Request:**
```json
{
  "strategy_id": "momentum-btc",
  "strategy_type": "momentum",
  "exchange_id": 1,
  "api_key_id": 1,
  "config": {"symbol": "BTC-USDT", "window_size": 20}
}
```

### GET /api/v1/strategies/{id}

### PUT /api/v1/strategies/{id}

**Request:**
```json
{"config": {"window_size": 30}, "exchange_id": 1, "api_key_id": 1}
```

### DELETE /api/v1/strategies/{id}

### POST /api/v1/strategies/{id}/start

**Response 200:** `{"status": "running", "strategy_id": "momentum-btc"}`

### POST /api/v1/strategies/{id}/stop

**Response 200:** `{"status": "stopped", "strategy_id": "momentum-btc"}`

---

## 持仓 & 订单查询

### GET /api/v1/positions

**Response 200:**
```json
[
  {"account_id": "default", "symbol": "BTC-USDT", "quantity": 0.5, "avg_cost": 60000, "realized_pnl": 123.45, "updated_ms": 1712000000000}
]
```

### GET /api/v1/orders

**Response 200:**
```json
[
  {"client_order_id": "cid-xxx", "venue_order_id": "vo-xxx", "symbol": "BTC-USDT", "state": "filled", "filled_qty": 0.1, "avg_price": 60000, "state_version": 3, "updated_ms": 1712000000000}
]
```

---

## 风控配置

### GET /api/v1/risk/config

**Response 200:**
```json
{"max_order_qty": 10, "max_order_amount": 1000000, "allowed_symbols": ["BTC-USDT", "ETH-USDT"]}
```

### PUT /api/v1/risk/config

**Request:**
```json
{"max_order_qty": 5, "max_order_amount": 500000, "allowed_symbols": ["BTC-USDT"]}
```

---

## 总览

### GET /api/v1/overview

**Response 200:**
```json
{
  "running_strategies": 2,
  "total_strategies": 5,
  "total_positions": 3,
  "total_orders": 150,
  "total_realized_pnl": 1234.56,
  "exchanges": [{"id": 1, "name": "my-binance", "status": "active"}]
}
```

---

## 通用错误格式

```json
{"error": "error_code", "message": "human readable message"}
```

| HTTP Status | error code | 场景 |
|-------------|-----------|------|
| 400 | bad_request | JSON 解析失败、缺少必填字段 |
| 401 | unauthorized | 未登录或 token 过期 |
| 404 | not_found | 资源不存在 |
| 409 | conflict | 状态冲突（如已启动的策略再次启动） |
| 500 | internal_error | 服务端错误 |
