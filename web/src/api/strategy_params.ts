// Strategy hot-reload API client.
//
// The Strategies page gets a per-row "参数" drawer that wraps these
// endpoints. An operator (or a promoted optimisation result) proposes
// a params update here; the admin-api forwards it to NATS and records
// an audit row whose status transitions from pending → accepted/failed
// when the runner acks.

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

export type StrategyControlType =
  | 'update_params'
  | 'pause'
  | 'resume'
  | 'shadow_on'
  | 'shadow_off'

export interface ProposeParamsRequest {
  type: StrategyControlType
  params?: Record<string, unknown>
  reason?: string
  actor?: string
}

export interface ProposeParamsResponse {
  strategy_id: string
  revision: number
  issued_ms: number
  status: string // "pending_ack"
}

export interface RevisionRow {
  id: number
  strategy_id: string
  revision: number
  command_type: StrategyControlType
  params_before?: string
  params_after?: string
  actor: string
  reason?: string
  issued_ms: number
  ack_received_ms?: number
  ack_accepted?: boolean
  ack_error?: string
}

export interface RevisionList {
  items: RevisionRow[]
  count: number
}

// -----------------------------------------------------------------------------
// Fetchers
// -----------------------------------------------------------------------------

export async function proposeStrategyParams(id: number, req: ProposeParamsRequest): Promise<ProposeParamsResponse> {
  const { data } = await axiosClient.post<ProposeParamsResponse>(`/api/v1/strategies/${id}/params`, req)
  return data
}

export async function listStrategyRevisions(id: number, limit = 50): Promise<RevisionList> {
  const { data } = await axiosClient.get<RevisionList>(`/api/v1/strategies/${id}/revisions?limit=${limit}`)
  return data
}

// -----------------------------------------------------------------------------
// TanStack hooks
// -----------------------------------------------------------------------------

export function useStrategyRevisions(id: number | null, opts?: { enabled?: boolean; refetchMs?: number }) {
  return useQuery({
    queryKey: ['strategy-revisions', id],
    queryFn: () => listStrategyRevisions(id!),
    enabled: id !== null && (opts?.enabled ?? true),
    refetchInterval: opts?.refetchMs,
  })
}

export function useProposeStrategyParams(id: number | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: ProposeParamsRequest) => proposeStrategyParams(id!, req),
    onSuccess: () => {
      if (id !== null) {
        qc.invalidateQueries({ queryKey: ['strategy-revisions', id] })
      }
    },
  })
}
