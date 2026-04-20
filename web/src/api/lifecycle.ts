// Strategy lifecycle API client.
//
// The lifecycle state machine lives on the backend; this module exposes
// typed wrappers around the four REST endpoints the Kanban board uses:
//
//   POST /api/v1/strategies/:id/lifecycle
//   GET  /api/v1/strategies/:id/lifecycle
//   GET  /api/v1/strategies/lifecycle-board
//   GET  /api/v1/strategies/:id/health

import axios from 'axios'
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'

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
// Types
// -----------------------------------------------------------------------------

export type LifecycleStage =
  | 'draft'
  | 'backtested'
  | 'paper'
  | 'canary'
  | 'live'
  | 'deprecated'

export type TransitionKind = 'promote' | 'demote' | 'deprecate'

export interface LifecycleTransitionRequest {
  to_stage: LifecycleStage
  reason?: string
  actor?: string
}

export interface LifecycleTransitionResponse {
  strategy_id: string
  from_stage: LifecycleStage
  to_stage: LifecycleStage
  kind: TransitionKind
  transitioned_ms: number
}

export interface LifecycleTransitionRow {
  id: number
  from_stage: LifecycleStage
  to_stage: LifecycleStage
  kind: TransitionKind
  actor: string
  reason?: string
  transitioned_ms: number
}

export interface StrategyLifecycleView {
  strategy_id: string
  stage: LifecycleStage
  transitions: LifecycleTransitionRow[]
}

export interface LifecycleBoardCard {
  id: number
  strategy_id: string
  strategy_type: string
  stage: LifecycleStage
  status: string
  config?: Record<string, unknown>
  updated_ms: number
  last_transition_ms?: number
}

export interface LifecycleBoardResponse {
  stages: LifecycleStage[]
  by_stage: Record<LifecycleStage, LifecycleBoardCard[] | null>
  total_count: number
}

export interface HealthResponse {
  strategy_id: string
  stage: LifecycleStage
  best_backtest_sharpe: number
  shadow_duration_ms: number
  shadow_virtual_pnl: number
  canary_duration_ms: number
  canary_live_sharpe: number
  sharpe_drift: number
  message?: string
}

// -----------------------------------------------------------------------------
// Fetchers
// -----------------------------------------------------------------------------

export async function proposeLifecycleTransition(
  id: number,
  req: LifecycleTransitionRequest,
): Promise<LifecycleTransitionResponse> {
  const { data } = await axiosClient.post<LifecycleTransitionResponse>(
    `/api/v1/strategies/${id}/lifecycle`,
    req,
  )
  return data
}

export async function getStrategyLifecycle(id: number): Promise<StrategyLifecycleView> {
  const { data } = await axiosClient.get<StrategyLifecycleView>(
    `/api/v1/strategies/${id}/lifecycle`,
  )
  return data
}

export async function getLifecycleBoard(): Promise<LifecycleBoardResponse> {
  const { data } = await axiosClient.get<LifecycleBoardResponse>(
    `/api/v1/strategies/lifecycle-board`,
  )
  return data
}

export async function getStrategyHealth(id: number): Promise<HealthResponse> {
  const { data } = await axiosClient.get<HealthResponse>(
    `/api/v1/strategies/${id}/health`,
  )
  return data
}

// -----------------------------------------------------------------------------
// TanStack hooks
// -----------------------------------------------------------------------------

const lifecycleKey = ['lifecycle'] as const

export function useLifecycleBoard(opts?: { refetchMs?: number }) {
  return useQuery({
    queryKey: [...lifecycleKey, 'board'],
    queryFn: getLifecycleBoard,
    refetchInterval: opts?.refetchMs,
  })
}

export function useStrategyLifecycle(id: number | null) {
  return useQuery({
    queryKey: [...lifecycleKey, 'strategy', id],
    queryFn: () => getStrategyLifecycle(id!),
    enabled: id !== null,
  })
}

export function useStrategyHealth(id: number | null) {
  return useQuery({
    queryKey: [...lifecycleKey, 'health', id],
    queryFn: () => getStrategyHealth(id!),
    enabled: id !== null,
  })
}

export function useProposeLifecycle(id: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: LifecycleTransitionRequest) => proposeLifecycleTransition(id!, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: lifecycleKey })
    },
  })
}
