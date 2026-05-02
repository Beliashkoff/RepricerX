export interface SimulateRequest {
  product_id: string
  strategy_id: string
  current_price: number
  competitor_price?: number
  cost_price?: number
}

export interface SimulateResult {
  target_price: number
  final_price: number
  constraint_hit: string | null
  reason: string
  change_pct: number
}

export const pricingApi = {
  simulate: async (req: SimulateRequest): Promise<SimulateResult> => {
    await delay(600)
    const delta = Math.random() * 0.1 - 0.05
    const target = Math.round(req.current_price * (1 + delta))
    const final = Math.max(target, 500)
    return {
      target_price: target,
      final_price: final,
      constraint_hit: final > target ? 'min_price' : null,
      reason: 'below_median_pct: рассчитано по медиане конкурентов',
      change_pct: ((final - req.current_price) / req.current_price) * 100,
    }
  },
}

function delay(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}
