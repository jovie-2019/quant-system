// Candidate API client for Phase 7's self-optimisation queue.
//
// The admin-api stages pending ParamCandidates whenever the nightly
// ReoptimizeJob (or a manual /reoptimize/run-now trigger) finds a
// new parameter set that improves the strategy's Sharpe by more than
// MinImprovement on recent klines. This module wraps the four REST
// endpoints operators use to process that queue.

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

export type CandidateStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'applied'
  | 'expired'

export interface Candidate {
  id: number
  strategy_id: string
  origin: string
  proposed_params: string // JSON text
  baseline_params: string
  baseline_sharpe: number
  proposed_sharpe: number
  improvement: number
  status: CandidateStatus
  rejection_reason?: string
  created_ms: number
  reviewed_ms?: number
  reviewer?: string
}

export interface CandidateList {
  items: Candidate[]
  count: number
}

// -----------------------------------------------------------------------------
// Fetchers
// -----------------------------------------------------------------------------

export async function listCandidates(params?: {
  status?: CandidateStatus
  strategy_id?: string
  limit?: number
}): Promise<CandidateList> {
  const q = new URLSearchParams()
  if (params?.status) q.set('status', params.status)
  if (params?.strategy_id) q.set('strategy_id', params.strategy_id)
  if (params?.limit) q.set('limit', String(params.limit))
  const { data } = await axiosClient.get<CandidateList>(`/api/v1/param-candidates?${q}`)
  return data
}

export async function approveCandidate(id: number, reason?: string): Promise<void> {
  await axiosClient.post(`/api/v1/param-candidates/${id}/approve`, {
    reason: reason || '',
  })
}

export async function rejectCandidate(id: number, reason: string): Promise<void> {
  await axiosClient.post(`/api/v1/param-candidates/${id}/reject`, { reason })
}

export async function runReoptimizeNow(): Promise<{ status: string; elapsed_ms: number }> {
  const { data } = await axiosClient.post<{ status: string; elapsed_ms: number }>(
    `/api/v1/reoptimize/run-now`,
    {},
  )
  return data
}

// -----------------------------------------------------------------------------
// TanStack hooks
// -----------------------------------------------------------------------------

const candidatesKey = ['candidates'] as const

export function useCandidates(params?: {
  status?: CandidateStatus
  strategy_id?: string
  limit?: number
  refetchMs?: number
}) {
  return useQuery({
    queryKey: [...candidatesKey, 'list', params?.status, params?.strategy_id, params?.limit],
    queryFn: () => listCandidates(params),
    refetchInterval: params?.refetchMs,
  })
}

export function useApproveCandidate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: number; reason?: string }) =>
      approveCandidate(vars.id, vars.reason),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: candidatesKey })
    },
  })
}

export function useRejectCandidate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: number; reason: string }) => rejectCandidate(vars.id, vars.reason),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: candidatesKey })
    },
  })
}

export function useRunReoptimize() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: runReoptimizeNow,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: candidatesKey })
    },
  })
}
