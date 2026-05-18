import apiClient from './client'
import type {
  ImportStart,
  ImportStatus,
  ImportErrorsPage,
  Product,
  ProductListParams,
  ProductListResult,
  BulkPatchItem,
} from '@/types/api'

interface ProductResponse {
  id: string
  shopId: string
  externalSku: string
  vendorCode?: string | null
  name: string
  currentPrice: number
  currency: string
  status: Product['status']
  minPrice: number | null
  maxPrice: number | null
  costPrice: number | null
  stockCount: number
  rating: number | null
  reviewsCount: number
  lastSyncedAt: string | null
  hasStrategy: boolean
  createdAt: string
  updatedAt: string
}

interface ProductListResponse {
  items: ProductResponse[]
  pagination: {
    page: number
    perPage: number
    total: number
  }
}

function toProduct(p: ProductResponse): Product {
  return {
    id: p.id,
    shop_id: p.shopId,
    external_sku: p.externalSku,
    vendor_code: p.vendorCode ?? null,
    name: p.name,
    current_price: p.currentPrice,
    currency: p.currency,
    status: p.status,
    min_price: p.minPrice,
    max_price: p.maxPrice,
    cost_price: p.costPrice,
    stock_count: p.stockCount,
    rating: p.rating,
    reviews_count: p.reviewsCount,
    last_synced_at: p.lastSyncedAt,
    has_strategy: p.hasStrategy,
    created_at: p.createdAt,
    updated_at: p.updatedAt,
  }
}

export const productsApi = {
  list: async (params?: ProductListParams): Promise<ProductListResult> => {
    const query: Record<string, string | number | undefined> = {}
    if (params?.page) query.page = params.page
    if (params?.perPage) query.perPage = params.perPage
    if (params?.shopId) query.shopId = params.shopId
    if (params?.q) query.q = params.q
    if (params?.status) query.status = params.status
    if (params?.sortBy) query.sortBy = params.sortBy
    if (params?.sortDir) query.sortDir = params.sortDir
    if (params?.priceFrom !== undefined) query.priceFrom = params.priceFrom
    if (params?.priceTo !== undefined) query.priceTo = params.priceTo
    const { data } = await apiClient.get<ProductListResponse>('/products', { params: query })
    return {
      items: data.items.map(toProduct),
      pagination: data.pagination,
    }
  },

  update: async (
    id: string,
    payload: { min_price?: number | null; max_price?: number | null; cost_price?: number | null },
  ): Promise<Product> => {
    const { data } = await apiClient.patch<ProductResponse>(`/products/${id}`, {
      minPrice: payload.min_price,
      maxPrice: payload.max_price,
      costPrice: payload.cost_price,
    })
    return toProduct(data)
  },

  softDelete: async (id: string): Promise<void> => {
    await apiClient.delete(`/products/${id}`)
  },

  bulkPatch: async (products: BulkPatchItem[]): Promise<{ updated: number }> => {
    const { data } = await apiClient.post<{ updated: number }>('/products/bulk-patch', { products })
    return data
  },

  exportCsv: async (params?: ProductListParams): Promise<void> => {
    const query: Record<string, string | number | undefined> = {}
    if (params?.shopId) query.shopId = params.shopId
    if (params?.q) query.q = params.q
    if (params?.status) query.status = params.status
    if (params?.sortBy) query.sortBy = params.sortBy
    if (params?.sortDir) query.sortDir = params.sortDir
    if (params?.priceFrom !== undefined) query.priceFrom = params.priceFrom
    if (params?.priceTo !== undefined) query.priceTo = params.priceTo
    const response = await apiClient.get('/products/export', {
      params: query,
      responseType: 'blob',
    })
    const url = URL.createObjectURL(new Blob([response.data], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `products-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  },

  startImport: async (shopId: string): Promise<ImportStart> => {
    const { data } = await apiClient.post<ImportStart>(`/shops/${shopId}/products/import`)
    return data
  },

  getImport: async (importId: string): Promise<ImportStatus> => {
    const { data } = await apiClient.get<ImportStatus>(`/imports/${importId}`)
    return data
  },

  cancelImport: async (importId: string): Promise<void> => {
    await apiClient.delete(`/imports/${importId}`)
  },

  getImportErrors: async (importId: string, page = 1, perPage = 20): Promise<ImportErrorsPage> => {
    const { data } = await apiClient.get<ImportErrorsPage>(`/imports/${importId}/errors`, {
      params: { page, perPage },
    })
    return data
  },
}
