import apiClient from '@/api/client'

export interface SimulateRequest {
  product_id: string
  strategy_id: string
  current_price?: number
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
    const { data } = await apiClient.post<SimulateResult>('/pricing/simulate', req)
    return data
  },
}
