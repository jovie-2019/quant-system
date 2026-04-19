// Regime API client + TanStack Query hooks.
//
// The backend classifies klines into a handful of labels (trend_up,
// trend_down, range, high_vol, low_liq, unknown). This module exposes
//   - computeRegime   POST /api/v1/regime/compute
//   - getRegimeHistory GET /api/v1/regime/history
//   - getRegimeMatrix  GET /api/v1/regime/matrix
//
// The matrix endpoint is the primary driver of the dashboard heatmap;
// history is used for the drill-down time series; compute is invoked
// from a CTA button when the user wants to (re)run classification.

import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

const axiosClient = (() => {
  const c = axios.create({ baseURL: import.meta.env.VITE_API_BASE_URL || '/' })
  c.interceptors.request.use((config) => {
    const token = localStorage.getItem('token')
    if (token) config.headers.Authorization = `Bearer ${token}`
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
// Types (mirror internal/regime + internal/adminapi/handler_regime.go)
// -----------------------------------------------------------------------------

export type RegimeLabel =
  | 'trend_up'
  | 'trend_down'
  | 'range'
  | 'high_vol'
  | 'low_liq'
  | 'unknown'

export type RegimeMethod = 'threshold' | 'gmm' | 'hmm'

export interface RegimeFeatures {
  ADX: number
  PlusDI: number
  MinusDI: number
  ATR: number
  ATRPercent: number
  BBW: number
  Hurst: number
  LastClose: number
  ReturnLast: number
  VolumeLast: number
  VolumeMean: number
}

export interface RegimeRecord {
  Venue: string
  Symbol: string
  Interval: string
  BarTime: number
  Method: RegimeMethod
  Regime: RegimeLabel
  Confidence: number
  Features: RegimeFeatures
}

export interface ComputeRegimeRequest {
  venue: string
  symbol: string
  interval: string
  start_ms: number
  end_ms: number
  adx_period?: number
  atr_period?: number
  bb_period?: number
  bb_stddev?: number
  hurst_lookback?: number
  hurst_min_n?: number
  thresholds?: {
    adx_trend?: number
    adx_range?: number
    hurst_persistent?: number
    hurst_mean_revert?: number
    atr_percent_high?: number
    volume_ratio_low?: number
  }
}

export interface ComputeRegimeResponse {
  venue: string
  symbol: string
  interval: string
  bars_fetched: number
  records_stored: number
  latest?: RegimeRecord
  tail: RegimeRecord[]
  method: RegimeMethod
}

export interface RegimeHistoryResponse {
  items: RegimeRecord[]
  count: number
}

export interface RegimeMatrixRow {
  venue: string
  symbol: string
  interval: string
  method: RegimeMethod
  regime: RegimeLabel
  confidence: number
  bar_time: number
  adx: number
  hurst: number
  bbw: number
  atr: number
}

export interface RegimeMatrixResponse {
  rows: RegimeMatrixRow[]
}

// -----------------------------------------------------------------------------
// Raw fetchers
// -----------------------------------------------------------------------------

export async function computeRegime(req: ComputeRegimeRequest): Promise<ComputeRegimeResponse> {
  const { data } = await axiosClient.post<ComputeRegimeResponse>('/api/v1/regime/compute', req)
  return data
}

export async function getRegimeHistory(params: {
  venue?: string
  symbol: string
  interval: string
  method?: RegimeMethod
  start_ms?: number
  end_ms?: number
  limit?: number
}): Promise<RegimeHistoryResponse> {
  const q = new URLSearchParams()
  q.set('symbol', params.symbol)
  q.set('interval', params.interval)
  if (params.venue) q.set('venue', params.venue)
  if (params.method) q.set('method', params.method)
  if (params.start_ms) q.set('start_ms', String(params.start_ms))
  if (params.end_ms) q.set('end_ms', String(params.end_ms))
  if (params.limit) q.set('limit', String(params.limit))
  const { data } = await axiosClient.get<RegimeHistoryResponse>(`/api/v1/regime/history?${q}`)
  return data
}

export async function getRegimeMatrix(params: {
  venue?: string
  symbols: string[]
  intervals: string[]
  method?: RegimeMethod
}): Promise<RegimeMatrixResponse> {
  const q = new URLSearchParams()
  q.set('symbols', params.symbols.join(','))
  q.set('intervals', params.intervals.join(','))
  if (params.venue) q.set('venue', params.venue)
  if (params.method) q.set('method', params.method)
  const { data } = await axiosClient.get<RegimeMatrixResponse>(`/api/v1/regime/matrix?${q}`)
  return data
}

// -----------------------------------------------------------------------------
// TanStack hooks
// -----------------------------------------------------------------------------

const regimeKey = ['regime'] as const

export function useRegimeMatrix(params: {
  venue?: string
  symbols: string[]
  intervals: string[]
  method?: RegimeMethod
  refetchMs?: number
}) {
  return useQuery({
    queryKey: [...regimeKey, 'matrix', params.venue, params.symbols.join(','), params.intervals.join(','), params.method],
    queryFn: () => getRegimeMatrix(params),
    enabled: params.symbols.length > 0 && params.intervals.length > 0,
    refetchInterval: params.refetchMs,
  })
}

export function useRegimeHistory(params: {
  venue?: string
  symbol: string
  interval: string
  method?: RegimeMethod
  start_ms?: number
  end_ms?: number
  limit?: number
  enabled?: boolean
}) {
  return useQuery({
    queryKey: [
      ...regimeKey,
      'history',
      params.venue,
      params.symbol,
      params.interval,
      params.method,
      params.start_ms,
      params.end_ms,
      params.limit,
    ],
    queryFn: () => getRegimeHistory(params),
    enabled: (params.enabled ?? true) && !!params.symbol && !!params.interval,
  })
}

export function useComputeRegime() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: computeRegime,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: regimeKey })
    },
  })
}
