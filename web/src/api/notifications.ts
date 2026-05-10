import apiClient from '@/api/client'

export type Severity = 'info' | 'warning' | 'error'
export type Channel = 'in_app' | 'email' | 'telegram' | 'webhook'

export interface NotificationResponse {
  id: string
  event_type: string
  severity: Severity
  title: string
  body: string
  data: Record<string, unknown>
  shop_id?: string
  plan_id?: string
  correlation_id?: string
  read_at?: string | null
  created_at: string
}

export interface NotificationListParams {
  page?: number
  perPage?: number
  eventType?: string
  severity?: Severity
  unreadOnly?: boolean
  shopId?: string
  from?: string
  to?: string
}

export interface NotificationListResult {
  items: NotificationResponse[]
  pagination: { page: number; per_page: number; total: number }
  unread: number
}

export interface NotificationDeliveryResponse {
  id: string
  channel: Channel
  status: 'pending' | 'pending_digest' | 'queued_digest' | 'sent' | 'failed' | 'skipped'
  attempts: number
  last_error?: string
  sent_at?: string
  updated_at: string
}

export interface NotificationDetail {
  notification: NotificationResponse
  deliveries: NotificationDeliveryResponse[]
}

export interface PreferenceItem {
  event_type: string
  channel: Channel
  enabled: boolean
}

export interface PreferencesResponse {
  items: PreferenceItem[]
}

export interface ChannelSettings {
  channel: Channel
  digest_window_minutes: number
  digest_min_severity: Severity
  quiet_hours_start?: number
  quiet_hours_end?: number
  digest_sent_at?: string
}

export interface ChannelSettingsListResponse {
  items: ChannelSettings[]
}

export interface UpdateChannelSettingsRequest {
  digest_window_minutes?: number
  digest_min_severity?: Severity
  quiet_hours_start?: number
  quiet_hours_end?: number
  clear_quiet_hours?: boolean
}

export interface TelegramLinkTokenResponse {
  token: string
  expires_at: string
  start_url: string
}

export interface TelegramStatusResponse {
  linked: boolean
  username?: string
  chat_id?: number
  linked_at?: string
}

export interface WebhookResponse {
  id: string
  url: string
  secret?: string
  enabled: boolean
  description: string
  created_at: string
}

export interface WebhookListResponse {
  items: WebhookResponse[]
}

export interface WebhookTestResponse {
  http_status?: number
  body?: string
  error?: string
}

function toQuery(p: NotificationListParams): Record<string, string | number | boolean> {
  const q: Record<string, string | number | boolean> = {}
  if (p.page) q.page = p.page
  if (p.perPage) q.per_page = p.perPage
  if (p.eventType) q.event_type = p.eventType
  if (p.severity) q.severity = p.severity
  if (p.unreadOnly) q.unread_only = true
  if (p.shopId) q.shop_id = p.shopId
  if (p.from) q.from = p.from
  if (p.to) q.to = p.to
  return q
}

export const notificationsApi = {
  list: async (params: NotificationListParams = {}): Promise<NotificationListResult> => {
    const { data } = await apiClient.get<NotificationListResult>('/notifications', { params: toQuery(params) })
    return data
  },

  get: async (id: string): Promise<NotificationDetail> => {
    const { data } = await apiClient.get<NotificationDetail>(`/notifications/${id}`)
    return data
  },

  unreadCount: async (): Promise<number> => {
    const { data } = await apiClient.get<{ count: number }>('/notifications/unread-count')
    return data.count
  },

  markRead: async (id: string): Promise<void> => {
    await apiClient.patch(`/notifications/${id}/read`)
  },

  markAllRead: async (): Promise<{ updated: number }> => {
    const { data } = await apiClient.post<{ updated: number }>('/notifications/read-all')
    return data
  },

  delete: async (id: string): Promise<void> => {
    await apiClient.delete(`/notifications/${id}`)
  },

  // Preferences
  getPreferences: async (): Promise<PreferencesResponse> => {
    const { data } = await apiClient.get<PreferencesResponse>('/notifications/preferences')
    return data
  },

  updatePreferences: async (items: PreferenceItem[]): Promise<void> => {
    await apiClient.put('/notifications/preferences', { items })
  },

  // Channel settings
  listChannelSettings: async (): Promise<ChannelSettingsListResponse> => {
    const { data } = await apiClient.get<ChannelSettingsListResponse>('/notifications/channel-settings')
    return data
  },

  updateChannelSettings: async (channel: Channel, body: UpdateChannelSettingsRequest): Promise<ChannelSettings> => {
    const { data } = await apiClient.put<ChannelSettings>(`/notifications/channel-settings/${channel}`, body)
    return data
  },

  // Telegram
  issueTelegramToken: async (): Promise<TelegramLinkTokenResponse> => {
    const { data } = await apiClient.post<TelegramLinkTokenResponse>('/notifications/telegram/link-token')
    return data
  },

  telegramStatus: async (): Promise<TelegramStatusResponse> => {
    const { data } = await apiClient.get<TelegramStatusResponse>('/notifications/telegram/status')
    return data
  },

  unlinkTelegram: async (): Promise<void> => {
    await apiClient.delete('/notifications/telegram')
  },

  // Webhooks
  listWebhooks: async (): Promise<WebhookListResponse> => {
    const { data } = await apiClient.get<WebhookListResponse>('/notifications/webhooks')
    return data
  },

  createWebhook: async (url: string, description: string): Promise<WebhookResponse> => {
    const { data } = await apiClient.post<WebhookResponse>('/notifications/webhooks', { url, description })
    return data
  },

  deleteWebhook: async (id: string): Promise<void> => {
    await apiClient.delete(`/notifications/webhooks/${id}`)
  },

  testWebhook: async (id: string): Promise<WebhookTestResponse> => {
    const { data } = await apiClient.post<WebhookTestResponse>(`/notifications/webhooks/${id}/test`)
    return data
  },
}

// Лейблы и цвета для UI.
export const SEVERITY_LABEL: Record<Severity, string> = {
  info: 'Информация',
  warning: 'Внимание',
  error: 'Ошибка',
}

export const CHANNEL_LABEL: Record<Channel, string> = {
  in_app: 'В приложении',
  email: 'Email',
  telegram: 'Telegram',
  webhook: 'Webhook',
}

export const EVENT_LABEL: Record<string, string> = {
  dispatch_completed: 'Отправка цен в маркетплейс',
  recalc_completed: 'Расчёт цен',
  import_completed: 'Импорт SKU',
  integration_error: 'Ошибки интеграции',
  scheduled_job_failed: 'Сбои фоновых задач',
  constraint_hit: 'Срабатывание ограничений',
  business_warning_no_cost: 'Не указана себестоимость',
  business_warning_no_competitors: 'Нет данных о конкурентах',
  business_warning_price_drift: 'Расхождение цен с маркетплейсом',
  competitor_price_dropped: 'Конкурент снизил цену',
  competitor_appeared: 'Появился новый конкурент',
  median_shifted: 'Сдвиг медианы конкурентов',
}

export const ALL_EVENTS: string[] = Object.keys(EVENT_LABEL)
export const ALL_CHANNELS: Channel[] = ['in_app', 'email', 'telegram', 'webhook']
