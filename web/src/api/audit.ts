import apiClient from '@/api/client'
import type {
  PriceChangeListParams,
  PriceChangeListResult,
  SummaryParams,
  SummaryReport,
} from '@/types/api'

// Маппит camelCase-параметры в snake_case query, как ожидает бэкенд.
function toQuery(p: PriceChangeListParams | SummaryParams): Record<string, string | number> {
  const q: Record<string, string | number> = {}
  if ('page' in p && p.page) q.page = p.page
  if ('perPage' in p && p.perPage) q.per_page = p.perPage
  if (p.shopId) q.shop_id = p.shopId
  if (p.productId) q.product_id = p.productId
  if (p.externalSku) q.external_sku = p.externalSku
  if (p.status) q.status = p.status
  if (p.from) q.from = p.from
  if (p.to) q.to = p.to
  if ('sortDir' in p && p.sortDir) q.sort_dir = p.sortDir
  return q
}

export const auditApi = {
  listChanges: async (params: PriceChangeListParams = {}): Promise<PriceChangeListResult> => {
    const { data } = await apiClient.get<PriceChangeListResult>('/audit/price-changes', {
      params: toQuery(params),
    })
    return data
  },

  getSummary: async (params: SummaryParams = {}): Promise<SummaryReport> => {
    const { data } = await apiClient.get<SummaryReport>('/reports/summary', {
      params: toQuery(params),
    })
    return data
  },

  exportCsv: async (params: PriceChangeListParams = {}): Promise<void> => {
    const response = await apiClient.get('/audit/price-changes.csv', {
      params: toQuery(params),
      responseType: 'blob',
    })
    const url = URL.createObjectURL(new Blob([response.data], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `price-changes-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  },
}
