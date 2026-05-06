// Типы, соответствующие swagger.yaml бэкенда

export type Marketplace = 'wb' | 'ozon'
export type ShopStatus = 'active' | 'error' | 'pending' | 'disabled'
export type ProductStatus = 'active' | 'archived' | 'out_of_stock'
export type ImportStatusValue = 'pending' | 'running' | 'succeeded' | 'partial' | 'failed' | 'canceled'
export type JobStatusValue = 'pending' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'retrying'
export type StrategyType = 'below_median_pct' | 'min_competitor_plus_step' | 'min_margin_pct' | 'fixed'
export type FallbackPolicy = 'keep_current' | 'set_fixed' | 'set_min'
export type PriceChangeStatus = 'success' | 'failed' | 'skipped'

// Auth
export interface User {
  id: string
  email: string
  display_name: string
  status: string
  created_at: string
}

// Shops
export interface Shop {
  id: string
  name: string
  marketplace: Marketplace
  status: ShopStatus
  lastCheckedAt: string | null
  createdAt: string
  autoUpdateEnabled: boolean
  scheduleCron: string
}

export interface CreateShopRequest {
  name: string
  marketplace: Marketplace
  credentials: WBCredentials | OzonCredentials
}

export interface WBCredentials {
  api_key: string
}

export interface OzonCredentials {
  client_id: string
  api_key: string
}

// Products
export type ProductSortField = 'name' | 'current_price' | 'updated_at'
export type SortDir = 'asc' | 'desc'

export interface Product {
  id: string
  shop_id: string
  external_sku: string
  name: string
  current_price: number
  currency: string
  status: ProductStatus
  min_price: number | null
  max_price: number | null
  cost_price: number | null
  stock_count: number
  rating: number | null
  reviews_count: number
  last_synced_at: string | null
  has_strategy: boolean
  created_at: string
  updated_at: string
}

export interface ProductListParams {
  page?: number
  perPage?: number
  shopId?: string
  q?: string
  status?: ProductStatus
  sortBy?: ProductSortField
  sortDir?: SortDir
  priceFrom?: number
  priceTo?: number
}

export interface ProductListResult {
  items: Product[]
  pagination: {
    page: number
    perPage: number
    total: number
  }
}

export interface BulkPatchItem {
  id: string
  minPrice?: number | null
  maxPrice?: number | null
  costPrice?: number | null
}

export interface ImportStart {
  importId: string
  jobId?: string
  shopId: string
  status: ImportStatusValue
  startedAt: string
  pollUrl: string
}

export interface ImportErrorItem {
  externalSku?: string
  code: string
  message: string
}

export interface ImportStatus {
  id: string
  jobId?: string
  shopId: string
  status: ImportStatusValue
  jobStatus?: JobStatusValue
  total: number
  added: number
  updated: number
  skipped: number
  failed: number
  errors: ImportErrorItem[]
  startedAt: string
  finishedAt: string | null
}

export interface ImportErrorsPage {
  items: ImportErrorItem[]
  total: number
  page: number
  perPage: number
}

// Strategies
export interface Strategy {
  id: string
  user_id: string
  name: string
  type: StrategyType
  params: Record<string, unknown>
  constraints: StrategyConstraints
  fallback_policy: FallbackPolicy
  priority: number
  enabled: boolean
  created_at: string
}

export interface StrategyConstraints {
  min_price?: number
  max_price?: number
  max_change_pct?: number
  min_interval_minutes?: number
}

// Price changes (audit log)
export interface PriceChange {
  id: string
  shop_id: string
  product_id: string
  product_name: string
  strategy_id: string | null
  old_price: number
  new_price: number
  target_price: number
  reason: string
  constraint_hit: string | null
  status: PriceChangeStatus
  created_at: string
}

// Reports
export interface SummaryReport {
  total_updates: number
  successful_updates: number
  failed_updates: number
  avg_change_pct: number
  period_start: string
  period_end: string
}

// API response wrappers
export interface ApiError {
  error: string
  message: string
}

export interface Pagination {
  page: number
  per_page: number
  total: number
}
