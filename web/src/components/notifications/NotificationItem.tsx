import { AlertCircle, AlertTriangle, Info } from 'lucide-react'
import { cn, formatDate } from '@/lib/utils'
import {
  EVENT_LABEL,
  SEVERITY_LABEL,
  type NotificationResponse,
} from '@/api/notifications'

export function notificationTarget(n: NotificationResponse): string {
  const deeplink = n.data?.deeplink
  if (typeof deeplink === 'string' && deeplink.startsWith('/')) return deeplink
  if (n.plan_id) return `/price-plans/${n.plan_id}`
  return `/notifications/${n.id}`
}

export function severityIcon(severity: NotificationResponse['severity']) {
  if (severity === 'error') return <AlertCircle className="h-4 w-4 text-red-600 shrink-0" />
  if (severity === 'warning') return <AlertTriangle className="h-4 w-4 text-amber-600 shrink-0" />
  return <Info className="h-4 w-4 text-blue-600 shrink-0" />
}

export function relativeTime(iso: string) {
  const t = new Date(iso).getTime()
  const diff = Math.max(0, Date.now() - t)
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'только что'
  if (mins < 60) return `${mins} мин назад`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs} ч назад`
  const days = Math.floor(hrs / 24)
  return `${days} дн назад`
}

export function NotificationItem({
  notification,
  compact = false,
  onClick,
}: {
  notification: NotificationResponse
  compact?: boolean
  onClick?: () => void
}) {
  const n = notification
  const content = (
    <>
      <div className="pt-0.5">{severityIcon(n.severity)}</div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between gap-2 mb-1">
          <span className="text-xs font-medium text-[#666]">
            {SEVERITY_LABEL[n.severity]} · {EVENT_LABEL[n.event_type] ?? n.event_type}
          </span>
          <span className="text-xs text-[#aaa] shrink-0">
            {compact ? relativeTime(n.created_at) : formatDate(n.created_at)}
          </span>
        </div>
        <div className={cn('text-sm font-semibold text-[#111]', compact && 'truncate')}>{n.title}</div>
        {n.body && (
          <div className={cn('text-xs text-[#666] mt-0.5', compact ? 'line-clamp-2' : 'line-clamp-3')}>
            {n.body}
          </div>
        )}
      </div>
      {!n.read_at && <span className="mt-1 h-2 w-2 rounded-full bg-blue-600 shrink-0" />}
    </>
  )

  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={cn(
          'w-full text-left px-4 py-3 border-b border-[#f0f0f0] hover:bg-[#f7f8fa] transition-colors flex gap-3',
          !n.read_at && 'bg-blue-50/50'
        )}
      >
        {content}
      </button>
    )
  }

  return (
    <div className={cn('px-4 py-3 border-b border-[#f0f0f0] flex gap-3', !n.read_at && 'bg-blue-50/50')}>
      {content}
    </div>
  )
}
