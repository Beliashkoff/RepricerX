import type { Strategy } from '@/types/api'

const MOCK_STRATEGIES: Strategy[] = [
  {
    id: '1', user_id: 'u1', name: 'Ниже медианы на 5%', type: 'below_median_pct',
    params: { pct: 5 },
    constraints: { min_price: 500, max_change_pct: 15, min_interval_minutes: 60 },
    fallback_policy: 'keep_current', priority: 1, enabled: true,
    created_at: new Date(Date.now() - 7 * 86400000).toISOString(),
  },
  {
    id: '2', user_id: 'u1', name: 'Минимальная цена конкурента + 50₽', type: 'min_competitor_plus_step',
    params: { step: 50 },
    constraints: { min_price: 1000, max_change_pct: 20, min_interval_minutes: 30 },
    fallback_policy: 'keep_current', priority: 2, enabled: true,
    created_at: new Date(Date.now() - 3 * 86400000).toISOString(),
  },
  {
    id: '3', user_id: 'u1', name: 'Маржа не ниже 30%', type: 'min_margin_pct',
    params: { margin_pct: 30 },
    constraints: { max_change_pct: 25 },
    fallback_policy: 'set_min', priority: 3, enabled: false,
    created_at: new Date(Date.now() - 86400000).toISOString(),
  },
]

export const strategiesApi = {
  list: async (): Promise<Strategy[]> => {
    await delay(300)
    return MOCK_STRATEGIES
  },

  create: async (payload: Partial<Strategy>): Promise<Strategy> => {
    await delay(400)
    const s: Strategy = {
      id: Math.random().toString(36).slice(2),
      user_id: 'u1',
      name: payload.name ?? 'Новая стратегия',
      type: payload.type ?? 'fixed',
      params: payload.params ?? {},
      constraints: payload.constraints ?? {},
      fallback_policy: payload.fallback_policy ?? 'keep_current',
      priority: (MOCK_STRATEGIES.length + 1),
      enabled: true,
      created_at: new Date().toISOString(),
    }
    MOCK_STRATEGIES.push(s)
    return s
  },

  delete: async (id: string): Promise<void> => {
    await delay(300)
    const idx = MOCK_STRATEGIES.findIndex(s => s.id === id)
    if (idx >= 0) MOCK_STRATEGIES.splice(idx, 1)
  },
}

function delay(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}
