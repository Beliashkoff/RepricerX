import apiClient from './client'
import type { Shop, CreateShopRequest } from '@/types/api'

export const shopsApi = {
  list: async (): Promise<Shop[]> => {
    const { data } = await apiClient.get<Shop[]>('/shops')
    return data
  },

  get: async (id: string): Promise<Shop> => {
    const { data } = await apiClient.get<Shop>(`/shops/${id}`)
    return data
  },

  create: async (payload: CreateShopRequest): Promise<Shop> => {
    const { data } = await apiClient.post<Shop>('/shops', payload)
    return data
  },

  update: async (id: string, payload: Partial<CreateShopRequest & { autoUpdateEnabled: boolean; scheduleCron: string }>): Promise<Shop> => {
    const { data } = await apiClient.patch<Shop>(`/shops/${id}`, payload)
    return data
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/shops/${id}`)
  },

  testConnection: async (id: string): Promise<{ status: string; message: string }> => {
    const { data } = await apiClient.post(`/shops/${id}/test`)
    return data
  },
}
