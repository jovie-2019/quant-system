export interface Exchange {
  id: number
  name: string
  venue: string
  status: string
  created_ms: number
  updated_ms: number
}

export interface APIKey {
  id: number
  exchange_id: number
  label: string
  api_key: string
  api_secret: string
  passphrase: string
  permissions: string
  status: string
  created_ms: number
  updated_ms: number
}

export interface StrategyConfig {
  id: number
  strategy_id: string
  strategy_type: string
  exchange_id: number
  api_key_id: number
  config: Record<string, unknown>
  status: string
  created_ms: number
  updated_ms: number
}

export interface Position {
  account_id: string
  symbol: string
  quantity: number
  avg_cost: number
  realized_pnl: number
  updated_ms: number
}

export interface Order {
  client_order_id: string
  venue_order_id: string
  symbol: string
  state: string
  filled_qty: number
  avg_price: number
  state_version: number
  updated_ms: number
}

export interface RiskConfig {
  max_order_qty: number
  max_order_amount: number
  allowed_symbols: string[]
}

export interface Overview {
  running_strategies: number
  total_strategies: number
  total_positions: number
  total_orders: number
  total_realized_pnl: number
  exchanges: Exchange[]
}

export interface LoginResponse {
  token: string
  expires_at: string
}

export interface ApiError {
  error: string
  message: string
}

// Request types

export interface CreateExchangeRequest {
  name: string
  venue: string
}

export interface UpdateExchangeRequest {
  name?: string
  venue?: string
  status?: string
}

export interface CreateAccountRequest {
  exchange_id: number
  label: string
  api_key: string
  api_secret: string
  passphrase?: string
}

export interface CreateStrategyRequest {
  strategy_id: string
  strategy_type: string
  exchange_id: number
  api_key_id: number
  config: Record<string, unknown>
}

export interface UpdateStrategyRequest {
  exchange_id?: number
  api_key_id?: number
  config?: Record<string, unknown>
}
