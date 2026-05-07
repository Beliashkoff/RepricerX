import apiClient from '@/api/client'
import type { PriceChange, SummaryReport } from '@/types/api'

export const auditApi = {
  listChanges: async (): Promise<PriceChange[]> => {
    const { data } = await apiClient.get<PriceChange[]>('/audit/price-changes')
    return data
  },

  getSummary: async (): Promise<SummaryReport> => {
    const { data } = await apiClient.get<SummaryReport>('/audit/summary')
    return data
  },

  exportCsv: async (): Promise<void> => {
    const response = await apiClient.get('/audit/export', { responseType: 'blob' })
    const url = URL.createObjectURL(new Blob([response.data], { type: 'text/csv' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `price-changes-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  },
}
