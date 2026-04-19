// Backtest API client + TanStack Query hooks.
//
// Responsibilities split:
//   - Data transport lives in `client.ts` so auth interception stays central.
//   - This module exports typed fetchers for /api/v1/backtests plus the
//     TanStack hooks a page can drop in without thinking about keys.
//
// Every list key includes `['backtests']` so a successful create
// invalidates the list cache in one line.

import axios from 'axios'
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'

// Re-use the same shared axios instance as the rest of the app so the JWT
// interceptor applies uniformly.
import './client'

const axiosClient = (() => {
  // Re-construct the instance here so we do not depend on the default
  // export shape of client.ts (it exports functions, not the axios
  // instance). Interceptors configured on a separate instance would not
  // see the JWT, so we build the same baseURL + interceptors chain.
  const c = axios.create({
    baseURL: import.meta.env.VITE_API_BASE_URL || '/',
  })
  c.interceptors.request.use((config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  })
  c.interceptors.response.use(
    (r) => r,
    (error) => {
      if (error.response?.status === 401) {
        localStorage.removeItem('token')
        window.location.href = '/login'
      }
      return Promise.reject(error)
    },
  )
  return c
})()

// -----------------------------------------------------------------------------
// Types (mirror internal/adminapi/backtest_store.go)
// -----------------------------------------------------------------------------

export type BacktestStatus = 'queued' | 'running' | 'done' | 'failed'

export interface BacktestDatasetSpec {
  source: 'synthetic' | 'clickhouse'
  symbol: string
  num_events: number
  // Synthetic knobs
  seed?: number
  start_price?: number
  volatility_bps?: number
  trend_bps_per_step?: number
  spread_bps?: number
  step_ms?: number
  // Shared (synthetic uses as initial ts; clickhouse uses as lower bound)
  start_ts_ms?: number
  // ClickHouse-only
  end_ts_ms?: number
  venue?: string
  interval?: string
}

export interface BacktestRiskSpec {
  max_order_qty?: number
  max_order_amount?: number
  allowed_symbols?: string[]
}

export interface BacktestRequest {
  strategy_type: string
  strategy_params: Record<string, unknown>
  dataset: BacktestDatasetSpec
  start_equity: number
  slippage_bps?: number
  fee_bps?: number
  risk?: BacktestRiskSpec
  account_id?: string
}

export interface EquityPoint {
  TSMS: number
  Cash: number
  MarkToMarket: number
}

export interface Metrics {
  FinalEquity: number
  TotalReturn: number
  MaxDrawdown: number
  Sharpe: number
  Calmar: number
  WinRate: number
  ProfitFactor: number
  Turnover: number
  NumTrades: number
}

export interface BacktestResult {
  StrategyID: string
  Dataset: string
  Events: number
  Intents: number
  Rejects: number
  Fills: number
  StartedAt: string
  FinishedAt: string
  Duration: number
  Equity: EquityPoint[]
  Trades: unknown[]
  Decisions: unknown[]
  Metrics: Metrics
}

export interface BacktestRecord {
  id: string
  status: BacktestStatus
  error?: string
  request: BacktestRequest
  result?: BacktestResult
  created_at: string
  started_at?: string
  finished_at?: string
}

export interface BacktestList {
  items: BacktestRecord[]
  count: number
}

// -----------------------------------------------------------------------------
// Raw fetchers
// -----------------------------------------------------------------------------

export async function createBacktest(req: BacktestRequest): Promise<BacktestRecord> {
  const { data } = await axiosClient.post<BacktestRecord>('/api/v1/backtests', req)
  return data
}

export async function getBacktest(id: string): Promise<BacktestRecord> {
  const { data } = await axiosClient.get<BacktestRecord>(`/api/v1/backtests/${id}`)
  return data
}

export async function listBacktests(limit = 50): Promise<BacktestList> {
  const { data } = await axiosClient.get<BacktestList>(`/api/v1/backtests?limit=${limit}`)
  return data
}

// -----------------------------------------------------------------------------
// TanStack Query hooks
// -----------------------------------------------------------------------------

const backtestsKey = ['backtests'] as const

export function useBacktests(limit = 50) {
  return useQuery({
    queryKey: [...backtestsKey, 'list', limit],
    queryFn: () => listBacktests(limit),
  })
}

export function useBacktest(id: string | null) {
  return useQuery({
    queryKey: [...backtestsKey, 'detail', id],
    queryFn: () => getBacktest(id!),
    enabled: !!id,
  })
}

/**
 * useCreateBacktest returns a TanStack mutation that runs a backtest and
 * invalidates the list cache on success so the recent-runs panel updates.
 */
export function useCreateBacktest() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createBacktest,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: backtestsKey })
    },
  })
}
