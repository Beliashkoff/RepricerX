import type { PriceChange, SummaryReport } from '@/types/api'

const MOCK_CHANGES: PriceChange[] = Array.from({ length: 20 }, (_, i) => ({
  id: String(i + 1),
  shop_id: 'shop1',
  product_id: String((i % 6) + 1),
  product_name: ['Кроссовки Nike Air Max 270', 'Футболка Adidas', 'Рюкзак городской 30л', 'Наушники Sony WH-1000XM5', 'Чехол iPhone 15 Pro', 'Зарядное USB-C 65W'][i % 6],
  strategy_id: String((i % 3) + 1),
  old_price: 1000 + i * 150,
  new_price: 950 + i * 150,
  target_price: 940 + i * 150,
  reason: ['below_median_pct', 'min_competitor_plus_step', 'min_margin_pct'][i % 3],
  constraint_hit: i % 5 === 0 ? 'min_price' : null,
  status: (i % 7 === 0 ? 'failed' : i % 3 === 0 ? 'skipped' : 'success') as PriceChange['status'],
  created_at: new Date(Date.now() - i * 3600000).toISOString(),
}))

export const auditApi = {
  listChanges: async (): Promise<PriceChange[]> => {
    await delay(400)
    return MOCK_CHANGES
  },

  getSummary: async (): Promise<SummaryReport> => {
    await delay(300)
    return {
      total_updates: 147,
      successful_updates: 132,
      failed_updates: 15,
      avg_change_pct: -3.2,
      period_start: new Date(Date.now() - 30 * 86400000).toISOString(),
      period_end: new Date().toISOString(),
    }
  },

  exportCsv: (): void => {
    const header = 'Дата,Товар,Старая цена,Новая цена,Статус\n'
    const rows = MOCK_CHANGES.map(c =>
      `${new Date(c.created_at).toLocaleString('ru-RU')},${c.product_name},${c.old_price},${c.new_price},${c.status}`
    ).join('\n')
    const blob = new Blob(['﻿' + header + rows], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'price-changes.csv'
    a.click()
    URL.revokeObjectURL(url)
  },
}

function delay(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}
