import type { Product } from '@/types/api'

const MOCK_PRODUCTS: Product[] = [
  { id: '1', shop_id: 'shop1', external_sku: 'WB-12345678', name: 'Кроссовки Nike Air Max 270', current_price: 8990, currency: 'RUB', status: 'active', min_price: 7000, max_price: 12000, cost_price: 5500, updated_at: new Date().toISOString() },
  { id: '2', shop_id: 'shop1', external_sku: 'WB-87654321', name: 'Футболка Adidas классик', current_price: 1890, currency: 'RUB', status: 'active', min_price: 1500, max_price: 3000, cost_price: 900, updated_at: new Date(Date.now() - 3600000).toISOString() },
  { id: '3', shop_id: 'shop1', external_sku: 'WB-11223344', name: 'Рюкзак городской 30л', current_price: 3450, currency: 'RUB', status: 'active', min_price: 2800, max_price: 5000, cost_price: 2000, updated_at: new Date(Date.now() - 7200000).toISOString() },
  { id: '4', shop_id: 'shop1', external_sku: 'WB-44332211', name: 'Наушники Sony WH-1000XM5', current_price: 24990, currency: 'RUB', status: 'active', min_price: 20000, max_price: 32000, cost_price: 15000, updated_at: new Date(Date.now() - 86400000).toISOString() },
  { id: '5', shop_id: 'shop1', external_sku: 'OZ-55667788', name: 'Чехол iPhone 15 Pro силикон', current_price: 590, currency: 'RUB', status: 'active', min_price: 400, max_price: 900, cost_price: 200, updated_at: new Date(Date.now() - 172800000).toISOString() },
  { id: '6', shop_id: 'shop1', external_sku: 'OZ-88776655', name: 'Зарядное устройство USB-C 65W', current_price: 2190, currency: 'RUB', status: 'paused', min_price: 1800, max_price: 3500, cost_price: 1200, updated_at: new Date(Date.now() - 259200000).toISOString() },
]

export const productsApi = {
  list: async (_shopId?: string): Promise<Product[]> => {
    await delay(400)
    return MOCK_PRODUCTS
  },

  update: async (id: string, payload: { min_price?: number; max_price?: number; cost_price?: number }): Promise<Product> => {
    await delay(300)
    const p = MOCK_PRODUCTS.find(p => p.id === id)
    if (!p) throw new Error('Товар не найден')
    return { ...p, ...payload }
  },

  startImport: async (_shopId: string): Promise<{ job_id: string }> => {
    await delay(500)
    return { job_id: 'job_' + Math.random().toString(36).slice(2) }
  },
}

function delay(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}
