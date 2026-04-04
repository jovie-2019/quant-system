import axios from 'axios'
import type {
  Exchange,
  APIKey,
  StrategyConfig,
  Position,
  Order,
  RiskConfig,
  Overview,
  LoginResponse,
  CreateExchangeRequest,
  UpdateExchangeRequest,
  CreateAccountRequest,
  CreateStrategyRequest,
  UpdateStrategyRequest,
} from './types'

const client = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/',
})

// Attach JWT token to every request
client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// On 401, clear token and redirect to login
client.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  },
)

// ---- Auth ----

export async function login(password: string): Promise<LoginResponse> {
  const { data } = await client.post<LoginResponse>('/api/v1/auth/login', { password })
  return data
}

// ---- Exchanges ----

export async function getExchanges(): Promise<Exchange[]> {
  const { data } = await client.get<Exchange[]>('/api/v1/exchanges')
  return data
}

export async function getExchange(id: number): Promise<Exchange> {
  const { data } = await client.get<Exchange>(`/api/v1/exchanges/${id}`)
  return data
}

export async function createExchange(req: CreateExchangeRequest): Promise<Exchange> {
  const { data } = await client.post<Exchange>('/api/v1/exchanges', req)
  return data
}

export async function updateExchange(id: number, req: UpdateExchangeRequest): Promise<Exchange> {
  const { data } = await client.put<Exchange>(`/api/v1/exchanges/${id}`, req)
  return data
}

export async function deleteExchange(id: number): Promise<void> {
  await client.delete(`/api/v1/exchanges/${id}`)
}

// ---- Accounts (API Keys) ----

export async function getAccounts(): Promise<APIKey[]> {
  const { data } = await client.get<APIKey[]>('/api/v1/accounts')
  return data
}

export async function getAccount(id: number): Promise<APIKey> {
  const { data } = await client.get<APIKey>(`/api/v1/accounts/${id}`)
  return data
}

export async function createAccount(req: CreateAccountRequest): Promise<APIKey> {
  const { data } = await client.post<APIKey>('/api/v1/accounts', req)
  return data
}

export async function deleteAccount(id: number): Promise<void> {
  await client.delete(`/api/v1/accounts/${id}`)
}

// ---- Strategies ----

export async function getStrategies(): Promise<StrategyConfig[]> {
  const { data } = await client.get<StrategyConfig[]>('/api/v1/strategies')
  return data
}

export async function getStrategy(id: number): Promise<StrategyConfig> {
  const { data } = await client.get<StrategyConfig>(`/api/v1/strategies/${id}`)
  return data
}

export async function createStrategy(req: CreateStrategyRequest): Promise<StrategyConfig> {
  const { data } = await client.post<StrategyConfig>('/api/v1/strategies', req)
  return data
}

export async function updateStrategy(id: number, req: UpdateStrategyRequest): Promise<StrategyConfig> {
  const { data } = await client.put<StrategyConfig>(`/api/v1/strategies/${id}`, req)
  return data
}

export async function deleteStrategy(id: number): Promise<void> {
  await client.delete(`/api/v1/strategies/${id}`)
}

export async function startStrategy(id: number): Promise<{ status: string; strategy_id: string }> {
  const { data } = await client.post<{ status: string; strategy_id: string }>(`/api/v1/strategies/${id}/start`)
  return data
}

export async function stopStrategy(id: number): Promise<{ status: string; strategy_id: string }> {
  const { data } = await client.post<{ status: string; strategy_id: string }>(`/api/v1/strategies/${id}/stop`)
  return data
}

// ---- Positions ----

export async function getPositions(): Promise<Position[]> {
  const { data } = await client.get<Position[]>('/api/v1/positions')
  return data
}

// ---- Orders ----

export async function getOrders(): Promise<Order[]> {
  const { data } = await client.get<Order[]>('/api/v1/orders')
  return data
}

// ---- Risk Config ----

export async function getRiskConfig(): Promise<RiskConfig> {
  const { data } = await client.get<RiskConfig>('/api/v1/risk/config')
  return data
}

export async function updateRiskConfig(req: RiskConfig): Promise<RiskConfig> {
  const { data } = await client.put<RiskConfig>('/api/v1/risk/config', req)
  return data
}

// ---- Overview ----

export async function getOverview(): Promise<Overview> {
  const { data } = await client.get<Overview>('/api/v1/overview')
  return data
}

// Namespace export for pages that use `import api from '../api/client'`
const api = {
  login,
  getExchanges,
  getExchange,
  createExchange,
  updateExchange,
  deleteExchange,
  getAccounts,
  getAccount,
  createAccount,
  deleteAccount,
  getStrategies,
  getStrategy,
  createStrategy,
  updateStrategy,
  deleteStrategy,
  startStrategy,
  stopStrategy,
  getPositions,
  getOrders,
  getRiskConfig,
  updateRiskConfig,
  getOverview,
}

export default api
