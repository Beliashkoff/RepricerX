import apiClient from './client'
import type { ImportStart, ImportStatus, Product } from '@/types/api'

interface ProductResponse {
  id: string
  shopId: string
  externalSku: string
  name: string
  currentPrice: number
  currency: string
  status: Product['status']
  minPrice: number | null
  maxPrice: number | null
  costPrice: number | null
  updatedAt: string
}

interface ProductListResponse {
  items: ProductResponse[]
}

function toProduct(p: ProductResponse): Product {
  return {
    id: p.id,
    shop_id: p.shopId,
    external_sku: p.externalSku,
    name: p.name,
    current_price: p.currentPrice,
    currency: p.currency,
    status: p.status,
    min_price: p.minPrice,
    max_price: p.maxPrice,
    cost_price: p.costPrice,
    updated_at: p.updatedAt,
  }
}

export const productsApi = {
  list: async (params?: { shopId?: string; q?: string }): Promise<Product[]> => {
    const { data } = await apiClient.get<ProductListResponse>('/products', { params })
    return data.items.map(toProduct)
  },

  update: async (id: string, payload: { min_price?: number | null; max_price?: number | null; cost_price?: number | null }): Promise<Product> => {
    const { data } = await apiClient.patch<ProductResponse>(`/products/${id}`, {
      minPrice: payload.min_price,
      maxPrice: payload.max_price,
      costPrice: payload.cost_price,
    })
    return toProduct(data)
  },

  startImport: async (shopId: string): Promise<ImportStart> => {
    const { data } = await apiClient.post<ImportStart>(`/shops/${shopId}/products/import`)
    return data
  },

  getImport: async (importId: string): Promise<ImportStatus> => {
    const { data } = await apiClient.get<ImportStatus>(`/imports/${importId}`)
    return data
  },
}
