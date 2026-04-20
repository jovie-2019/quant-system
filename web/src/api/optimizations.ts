// Parameter-optimisation API client.
//
// The optimiser endpoint wraps backtest.v2 in a loop over a parameter
// grid / random sample and scores each trial with a selected objective.
// This module exposes:
//
//   createOptimization  POST /api/v1/optimizations
//   getOptimization     GET  /api/v1/optimizations/:id
//   listOptimizations   GET  /api/v1/optimizations?limit=N
//
// plus the TanStack hooks the Parameter Optimization page uses directly.

import axios from 'axios'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import type { BacktestDatasetSpec, BacktestRiskSpec, Metrics } from './backtests'

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
// Types (mirror internal/optimizer + internal/adminapi/optimization_store.go)
// -----------------------------------------------------------------------------

export type OptimizationStatus = 'queued' | 'running' | 'done' | 'failed'
export type OptAlgorithm = 'grid' | 'random'
export type ObjectivePreset =
  | 'sharpe_penalty_dd'
  | 'total_return'
  | 'calmar'
  | 'profit_factor'
export type OptParamType = 'int' | 'float' | 'categorical'

export interface OptParamPayload {
  name: string
  type: OptParamType
  min?: number
  max?: number
  step?: number
  log_scale?: boolean
  choices?: unknown[]
}

export interface OptimizationRequest {
  strategy_type: string
  base_params: Record<string, unknown>
  params: OptParamPayload[]
  dataset: BacktestDatasetSpec
  start_equity: number
  slippage_bps?: number
  fee_bps?: number
  risk?: BacktestRiskSpec
  algorithm?: OptAlgorithm
  max_trials?: number
  seed?: number
  objective?: ObjectivePreset
  account_id?: string
}

export interface Trial {
  id: number
  params: Record<string, unknown>
  objective: number
  metrics: Metrics
  error?: string
  duration_ms: number
  build_error?: string
}

export interface OptimizationResult {
  trials: Trial[]
  best: Trial
  algorithm: OptAlgorithm
  objective?: ObjectivePreset
  importance: Record<string, number>
  stability: number
  started_at: string
  finished_at: string
  duration_ms: number
  strategy_type: string
  dataset_name: string
}

export interface OptimizationRecord {
  id: string
  status: OptimizationStatus
  error?: string
  request: OptimizationRequest
  result?: OptimizationResult
  created_at: string
  started_at?: string
  finished_at?: string
}

export interface OptimizationList {
  items: OptimizationRecord[]
  count: number
}

// -----------------------------------------------------------------------------
// Raw fetchers
// -----------------------------------------------------------------------------

export async function createOptimization(req: OptimizationRequest): Promise<OptimizationRecord> {
  const { data } = await axiosClient.post<OptimizationRecord>('/api/v1/optimizations', req)
  return data
}

export async function getOptimization(id: string): Promise<OptimizationRecord> {
  const { data } = await axiosClient.get<OptimizationRecord>(`/api/v1/optimizations/${id}`)
  return data
}

export async function listOptimizations(limit = 50): Promise<OptimizationList> {
  const { data } = await axiosClient.get<OptimizationList>(`/api/v1/optimizations?limit=${limit}`)
  return data
}

// -----------------------------------------------------------------------------
// TanStack Query hooks
// -----------------------------------------------------------------------------

const optKey = ['optimizations'] as const

export function useOptimizations(limit = 20) {
  return useQuery({
    queryKey: [...optKey, 'list', limit],
    queryFn: () => listOptimizations(limit),
  })
}

export function useOptimization(id: string | null) {
  return useQuery({
    queryKey: [...optKey, 'detail', id],
    queryFn: () => getOptimization(id!),
    enabled: !!id,
  })
}

export function useCreateOptimization() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createOptimization,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: optKey })
    },
  })
}
