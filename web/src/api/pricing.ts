import apiClient from '@/api/client'

export interface SimulateRequest {
  product_id: string
  strategy_id: string
  current_price?: number
  competitor_price?: number
  competitor_prices?: number[]
  cost_price?: number
}

export interface SimulateResult {
  target_price: number
  final_price: number
  constraint_hit: string | null
  status: string
  reason: string
  change_pct: number
  competitor_price?: number
  competitor_source?: string
}

export interface RecalculateRequest {
  shop_id: string
  product_ids?: string[]
}

export type PricePlanStatus =
  | 'pending'
  | 'processing'
  | 'calculated'
  | 'dispatching'
  | 'applied'
  | 'partial'
  | 'failed'
  | 'cancelled'

export interface PricePlanSummary {
  id: string
  shop_id: string
  status: PricePlanStatus
  total: number
  created_at: string
  updated_at: string
}

export interface RecalculateResponse {
  plan: PricePlanSummary
  job_id: string
}

export type PricePlanItemStatus = 'pending' | 'applied' | 'dispatched' | 'skipped' | 'failed'

export interface PricePlanItem {
  id: string
  product_id: string
  product_name: string
  strategy_id?: string | null
  current_price: number
  target_price: number
  final_price: number
  constraint_hit: string
  status: PricePlanItemStatus
  error?: string
}

export interface PricePlanDetail {
  plan: PricePlanSummary
  items: PricePlanItem[]
}

export interface PricePlanList {
  items: PricePlanSummary[]
  total: number
  limit: number
  offset: number
}

export const pricingApi = {
  simulate: async (req: SimulateRequest): Promise<SimulateResult> => {
    const { data } = await apiClient.post<SimulateResult>('/pricing/simulate', req)
    return data
  },

  recalculate: async (req: RecalculateRequest): Promise<RecalculateResponse> => {
    const { data } = await apiClient.post<RecalculateResponse>('/pricing/recalculate', req)
    return data
  },

  listPlans: async (limit = 50, offset = 0): Promise<PricePlanList> => {
    const { data } = await apiClient.get<PricePlanList>('/price-plans', { params: { limit, offset } })
    return data
  },

  getPlan: async (id: string): Promise<PricePlanDetail> => {
    const { data } = await apiClient.get<PricePlanDetail>(`/price-plans/${id}`)
    return data
  },

  // Этап 6: manual dispatch уже рассчитанного плана в МП.
  dispatch: async (planID: string): Promise<{ job_id: string; plan_id: string }> => {
    const { data } = await apiClient.post(`/price-plans/${planID}/dispatch`)
    return data
  },

  // Этап 6: отмена плана в non-terminal статусе.
  cancelPlan: async (planID: string): Promise<void> => {
    await apiClient.post(`/price-plans/${planID}/cancel`)
  },
}
