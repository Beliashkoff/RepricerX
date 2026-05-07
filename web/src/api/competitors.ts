import apiClient from '@/api/client'
import type { Competitor } from '@/types/api'

export const competitorsApi = {
  list: async (productId: string): Promise<Competitor[]> => {
    const { data } = await apiClient.get<Competitor[]>(`/products/${productId}/competitors`)
    return data
  },

  create: async (productId: string, target: string): Promise<Competitor> => {
    const { data } = await apiClient.post<Competitor>(`/products/${productId}/competitors`, { target })
    return data
  },

  update: async (competitorId: string, target: string): Promise<Competitor> => {
    const { data } = await apiClient.patch<Competitor>(`/competitors/${competitorId}`, { target })
    return data
  },

  remove: async (competitorId: string): Promise<void> => {
    await apiClient.delete(`/competitors/${competitorId}`)
  },

  refresh: async (competitorId: string): Promise<Competitor> => {
    const { data } = await apiClient.post<Competitor>(`/competitors/${competitorId}/refresh`)
    return data
  },
}
