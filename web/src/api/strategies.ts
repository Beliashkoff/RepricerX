import apiClient from '@/api/client'
import type { Strategy, StrategyDetail, CreateStrategyPayload, UpdateStrategyPayload } from '@/types/api'

export const strategiesApi = {
  list: async (): Promise<Strategy[]> => {
    const { data } = await apiClient.get<Strategy[]>('/strategies')
    return data
  },

  get: async (id: string): Promise<StrategyDetail> => {
    const { data } = await apiClient.get<StrategyDetail>(`/strategies/${id}`)
    return data
  },

  create: async (payload: CreateStrategyPayload): Promise<Strategy> => {
    const { data } = await apiClient.post<Strategy>('/strategies', payload)
    return data
  },

  update: async (id: string, patch: UpdateStrategyPayload): Promise<Strategy> => {
    const { data } = await apiClient.patch<Strategy>(`/strategies/${id}`, patch)
    return data
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/strategies/${id}`)
  },

  assign: async (strategyId: string, productIds: string[]): Promise<void> => {
    await apiClient.post(`/strategies/${strategyId}/assignments`, { productIds })
  },

  unassign: async (strategyId: string, productIds: string[]): Promise<void> => {
    await apiClient.delete(`/strategies/${strategyId}/assignments`, { data: { productIds } })
  },
}
