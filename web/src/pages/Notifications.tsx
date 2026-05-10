import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { CheckCheck, ChevronLeft, ChevronRight, Trash2 } from 'lucide-react'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ALL_EVENTS,
  CHANNEL_LABEL,
  EVENT_LABEL,
  SEVERITY_LABEL,
  notificationsApi,
  type NotificationListParams,
  type Severity,
} from '@/api/notifications'
import { NotificationItem, notificationTarget, severityIcon } from '@/components/notifications/NotificationItem'
import { formatDate } from '@/lib/utils'

const ANY = '__any__'
const PER_PAGE_OPTIONS = [20, 50, 100] as const
type PerPage = (typeof PER_PAGE_OPTIONS)[number]

function dateToISO(date: string, start: boolean): string | undefined {
  if (!date) return undefined
  const time = start ? 'T00:00:00.000Z' : 'T23:59:59.999Z'
  return new Date(`${date}${time}`).toISOString()
}

function parsePerPage(raw: string | null): PerPage {
  const n = Number(raw)
  return (PER_PAGE_OPTIONS as readonly number[]).includes(n) ? (n as PerPage) : 20
}

function isSeverity(v: string): v is Severity {
  return v === 'info' || v === 'warning' || v === 'error'
}

function statusVariant(status: string) {
  if (status === 'sent') return 'success' as const
  if (status === 'failed') return 'destructive' as const
  if (status === 'skipped') return 'outline' as const
  if (status.includes('digest')) return 'warning' as const
  return 'secondary' as const
}

export default function Notifications() {
  const [searchParams, setSearchParams] = useSearchParams()
  const { id } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [selectedID, setSelectedID] = useState<string | null>(id ?? null)

  useEffect(() => setSelectedID(id ?? null), [id])

  const eventType = searchParams.get('event_type') ?? ''
  const severityRaw = searchParams.get('severity') ?? ''
  const severity = isSeverity(severityRaw) ? severityRaw : ''
  const unreadOnly = searchParams.get('unread_only') === '1'
  const from = searchParams.get('from') ?? ''
  const to = searchParams.get('to') ?? ''
  const page = Math.max(1, Number(searchParams.get('page') ?? '1') || 1)
  const perPage = parsePerPage(searchParams.get('per_page'))

  function updateParam(key: string, value: string, resetPage = true) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      if (resetPage) next.delete('page')
      return next
    })
  }

  const params: NotificationListParams = useMemo(() => ({
    page,
    perPage,
    eventType: eventType || undefined,
    severity: severity || undefined,
    unreadOnly,
    from: dateToISO(from, true),
    to: dateToISO(to, false),
  }), [eventType, from, page, perPage, severity, to, unreadOnly])

  const { data: list, isLoading } = useQuery({
    queryKey: ['notifications', 'list', params],
    queryFn: () => notificationsApi.list(params),
  })

  const { data: detail } = useQuery({
    queryKey: ['notifications', 'detail', selectedID],
    queryFn: () => notificationsApi.get(selectedID!),
    enabled: !!selectedID,
  })

  const markRead = useMutation({
    mutationFn: (notificationID: string) => notificationsApi.markRead(notificationID),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notifications'] }),
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось отметить уведомление'),
  })

  const markAll = useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => {
      toast.success('Все уведомления отмечены прочитанными')
      qc.invalidateQueries({ queryKey: ['notifications'] })
    },
  })

  const remove = useMutation({
    mutationFn: (notificationID: string) => notificationsApi.delete(notificationID),
    onSuccess: () => {
      toast.success('Уведомление удалено')
      qc.invalidateQueries({ queryKey: ['notifications'] })
      setSelectedID(null)
      if (id) navigate('/notifications')
    },
  })

  const items = list?.items ?? []
  const total = list?.pagination.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / perPage))

  return (
    <AppLayout>
      <PageHeader
        title="Уведомления"
        description="События импорта, расчёта, отправки цен и внешних интеграций"
        action={
          <Button variant="secondary" size="sm" className="gap-1.5" onClick={() => markAll.mutate()} disabled={markAll.isPending}>
            <CheckCheck className="h-3.5 w-3.5" />
            Отметить все
          </Button>
        }
      />

      <div className="bg-white rounded-2xl border border-[#e6e6e6] p-4 mb-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-3">
        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">Событие</Label>
          <Select value={eventType || ANY} onValueChange={(v) => updateParam('event_type', v === ANY ? '' : v)}>
            <SelectTrigger><SelectValue placeholder="Все события" /></SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>Все события</SelectItem>
              {ALL_EVENTS.map((ev) => <SelectItem key={ev} value={ev}>{EVENT_LABEL[ev]}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">Важность</Label>
          <Select value={severity || ANY} onValueChange={(v) => updateParam('severity', v === ANY ? '' : v)}>
            <SelectTrigger><SelectValue placeholder="Любая" /></SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>Любая</SelectItem>
              <SelectItem value="info">Информация</SelectItem>
              <SelectItem value="warning">Внимание</SelectItem>
              <SelectItem value="error">Ошибка</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">С</Label>
          <Input type="date" value={from} onChange={(e) => updateParam('from', e.target.value)} />
        </div>
        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">По</Label>
          <Input type="date" value={to} onChange={(e) => updateParam('to', e.target.value)} />
        </div>
        <label className="flex items-center gap-2 text-sm text-[#111] pt-6">
          <input
            type="checkbox"
            checked={unreadOnly}
            onChange={(e) => updateParam('unread_only', e.target.checked ? '1' : '')}
            className="h-4 w-4 accent-[#ffcc00]"
          />
          Только непрочитанные
        </label>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1fr)_380px] gap-4">
        <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
          {isLoading ? (
            <div className="flex justify-center py-12">
              <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
            </div>
          ) : items.length === 0 ? (
            <div className="text-center py-12 text-sm text-[#666]">Уведомлений не найдено</div>
          ) : (
            items.map((n) => (
              <NotificationItem
                key={n.id}
                notification={n}
                onClick={() => {
                  setSelectedID(n.id)
                  navigate(`/notifications/${n.id}`)
                  if (!n.read_at) markRead.mutate(n.id)
                }}
              />
            ))
          )}
          <div className="flex items-center justify-between px-4 py-3 border-t border-[#e6e6e6]">
            <div className="text-xs text-[#666]">Всего: {total}</div>
            <div className="flex items-center gap-2">
              <Select
                value={String(perPage)}
                onValueChange={(v) => updateParam('per_page', v, true)}
              >
                <SelectTrigger className="w-24 h-9"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {PER_PAGE_OPTIONS.map((n) => <SelectItem key={n} value={String(n)}>{n}</SelectItem>)}
                </SelectContent>
              </Select>
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => updateParam('page', String(page - 1), false)}>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="text-sm text-[#666] w-16 text-center">{page} / {totalPages}</span>
              <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => updateParam('page', String(page + 1), false)}>
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        <aside className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden h-fit">
          {!detail ? (
            <div className="p-6 text-sm text-[#666]">Выберите уведомление, чтобы посмотреть детали доставки.</div>
          ) : (
            <div>
              <div className="p-4 border-b border-[#e6e6e6]">
                <div className="flex items-start gap-2">
                  {severityIcon(detail.notification.severity)}
                  <div className="min-w-0">
                    <div className="text-xs text-[#666]">{SEVERITY_LABEL[detail.notification.severity]}</div>
                    <h3 className="font-semibold text-[#111] mt-1">{detail.notification.title}</h3>
                    <p className="text-xs text-[#666] mt-1">{formatDate(detail.notification.created_at)}</p>
                  </div>
                </div>
                {detail.notification.body && <p className="text-sm text-[#666] mt-3">{detail.notification.body}</p>}
                <div className="flex gap-2 mt-4">
                  <Button type="button" size="sm" variant="secondary" onClick={() => navigate(notificationTarget(detail.notification))}>
                    Открыть
                  </Button>
                  {!detail.notification.read_at && (
                    <Button type="button" size="sm" variant="outline" onClick={() => markRead.mutate(detail.notification.id)}>
                      Прочитано
                    </Button>
                  )}
                  <Button type="button" size="sm" variant="destructive" onClick={() => remove.mutate(detail.notification.id)}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              <div className="p-4">
                <h4 className="text-sm font-semibold text-[#111] mb-3">Доставка</h4>
                <div className="space-y-2">
                  {detail.deliveries.map((d) => (
                    <div key={d.id} className="flex items-center justify-between gap-3 rounded-xl bg-[#f7f8fa] px-3 py-2">
                      <div>
                        <div className="text-sm font-medium text-[#111]">{CHANNEL_LABEL[d.channel]}</div>
                        <div className="text-xs text-[#666]">Попыток: {d.attempts}</div>
                        {d.last_error && <div className="text-xs text-red-600 max-w-[260px] truncate">{d.last_error}</div>}
                      </div>
                      <Badge variant={statusVariant(d.status)}>{d.status}</Badge>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
        </aside>
      </div>
    </AppLayout>
  )
}
