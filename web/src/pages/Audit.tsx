import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
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
import { auditApi } from '@/api/audit'
import { shopsApi } from '@/api/shops'
import type { PriceChange, PriceChangeListParams, PriceChangeStatus } from '@/types/api'
import { formatPrice, formatDate } from '@/lib/utils'
import { ChevronLeft, ChevronRight, Download } from 'lucide-react'

const ANY = '__any__'
const PER_PAGE_OPTIONS = [50, 100, 200] as const
type PerPage = (typeof PER_PAGE_OPTIONS)[number]

// URL-параметры — единственный источник истины для фильтров (можно делиться ссылкой,
// reload не теряет состояние).
const PARAM = {
  shopId: 'shop_id',
  status: 'status',
  from: 'from',
  to: 'to',
  sku: 'sku',
  page: 'page',
  perPage: 'per_page',
} as const

function StatusBadge({ status }: { status: PriceChange['status'] }) {
  const variant = { success: 'success', failed: 'destructive', skipped: 'warning' } as const
  const label = { success: 'Успешно', failed: 'Ошибка', skipped: 'Пропущено' }
  return <Badge variant={variant[status]}>{label[status]}</Badge>
}

// Локальная дата (yyyy-mm-dd) → ISO RFC3339 в UTC. start=true даёт 00:00:00Z, иначе 23:59:59Z.
function dateToISO(date: string, start: boolean): string | undefined {
  if (!date) return undefined
  const time = start ? 'T00:00:00.000Z' : 'T23:59:59.999Z'
  return new Date(`${date}${time}`).toISOString()
}

function isStatus(value: string): value is PriceChangeStatus {
  return value === 'success' || value === 'failed' || value === 'skipped'
}

function parsePerPage(raw: string | null): PerPage {
  const n = Number(raw)
  return (PER_PAGE_OPTIONS as readonly number[]).includes(n) ? (n as PerPage) : 50
}

export default function Audit() {
  const [searchParams, setSearchParams] = useSearchParams()

  // Все фильтры — производные от URL.
  const shopId = searchParams.get(PARAM.shopId) ?? ''
  const statusRaw = searchParams.get(PARAM.status) ?? ''
  const status: PriceChangeStatus | '' = isStatus(statusRaw) ? statusRaw : ''
  const from = searchParams.get(PARAM.from) ?? ''
  const to = searchParams.get(PARAM.to) ?? ''
  const skuQuery = searchParams.get(PARAM.sku) ?? ''
  const page = Math.max(1, Number(searchParams.get(PARAM.page) ?? '1') || 1)
  const perPage = parsePerPage(searchParams.get(PARAM.perPage))

  // SKU — локальный input + debounce, только debounced значение пишется в URL.
  const [skuInput, setSkuInput] = useState(skuQuery)
  useEffect(() => {
    if (skuInput === skuQuery) return
    const t = setTimeout(() => updateParam(PARAM.sku, skuInput.trim(), true), 300)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [skuInput])

  // Если URL поменялся извне (back/forward) — синкаем локальный input.
  useEffect(() => {
    setSkuInput(skuQuery)
  }, [skuQuery])

  // updateParam — обновить query-параметр, сбросив page (если меняется не сам page/per_page).
  function updateParam(key: string, value: string, resetPage = true) {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      if (resetPage) next.delete(PARAM.page)
      return next
    })
  }

  function setPage(next: number) {
    updateParam(PARAM.page, next > 1 ? String(next) : '', false)
  }
  function setPerPage(next: PerPage) {
    setSearchParams(prev => {
      const sp = new URLSearchParams(prev)
      sp.set(PARAM.perPage, String(next))
      sp.delete(PARAM.page)
      return sp
    })
  }

  const baseParams: PriceChangeListParams = useMemo(
    () => ({
      shopId: shopId || undefined,
      status: status || undefined,
      from: dateToISO(from, true),
      to: dateToISO(to, false),
      externalSku: skuQuery || undefined,
    }),
    [shopId, status, from, to, skuQuery],
  )
  const listParams: PriceChangeListParams = { ...baseParams, page, perPage }

  const { data: shops = [] } = useQuery({ queryKey: ['shops'], queryFn: shopsApi.list })
  const { data: list, isLoading } = useQuery({
    queryKey: ['audit', listParams],
    queryFn: () => auditApi.listChanges(listParams),
  })
  const { data: summary } = useQuery({
    queryKey: ['summary', baseParams],
    queryFn: () => auditApi.getSummary(baseParams),
  })

  const changes = list?.items ?? []
  const total = list?.pagination.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / perPage))

  return (
    <AppLayout>
      <PageHeader
        title="Журнал изменений"
        description="История обновлений цен за последние 180 дней"
        action={
          <Button
            variant="secondary"
            size="sm"
            className="gap-1.5"
            onClick={() => auditApi.exportCsv(baseParams)}
          >
            <Download className="h-3.5 w-3.5" />
            Экспорт CSV
          </Button>
        }
      />

      {summary && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {[
            { label: 'Всего изменений', val: summary.total_updates },
            { label: 'Успешных', val: summary.successful_updates },
            { label: 'Ошибок', val: summary.failed_updates },
            { label: 'Среднее изменение', val: `${summary.avg_change_pct.toFixed(1)}%` },
          ].map(({ label, val }) => (
            <div key={label} className="bg-white rounded-2xl border border-[#e6e6e6] p-4">
              <p className="text-xs text-[#666] mb-1">{label}</p>
              <p className="text-2xl font-bold text-[#111]">{val}</p>
            </div>
          ))}
        </div>
      )}

      {/* фильтры */}
      <div className="bg-white rounded-2xl border border-[#e6e6e6] p-4 mb-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-3">
        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">Магазин</Label>
          <Select
            value={shopId || ANY}
            onValueChange={v => updateParam(PARAM.shopId, v === ANY ? '' : v)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Все магазины" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>Все магазины</SelectItem>
              {shops.map(s => (
                <SelectItem key={s.id} value={s.id}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">Статус</Label>
          <Select
            value={status || ANY}
            onValueChange={v => updateParam(PARAM.status, v === ANY ? '' : v)}
          >
            <SelectTrigger>
              <SelectValue placeholder="Все статусы" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ANY}>Все статусы</SelectItem>
              <SelectItem value="success">Успешно</SelectItem>
              <SelectItem value="failed">Ошибка</SelectItem>
              <SelectItem value="skipped">Пропущено</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">С</Label>
          <Input
            type="date"
            value={from}
            onChange={e => updateParam(PARAM.from, e.target.value)}
          />
        </div>

        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">По</Label>
          <Input
            type="date"
            value={to}
            onChange={e => updateParam(PARAM.to, e.target.value)}
          />
        </div>

        <div>
          <Label className="text-xs text-[#666] mb-1.5 block">Поиск по SKU</Label>
          <Input
            value={skuInput}
            onChange={e => setSkuInput(e.target.value)}
            placeholder="Подстрока external_sku"
          />
        </div>
      </div>

      <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
        {isLoading ? (
          <div className="flex justify-center py-12">
            <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
          </div>
        ) : changes.length === 0 ? (
          <div className="text-center py-12 text-sm text-[#666]">Записей не найдено</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[#f5f5f5] text-[#666] text-xs">
                  <th className="text-left px-4 py-3 font-medium">Дата</th>
                  <th className="text-left px-4 py-3 font-medium">Товар</th>
                  <th className="text-right px-4 py-3 font-medium">Было</th>
                  <th className="text-right px-4 py-3 font-medium">Стало</th>
                  <th className="text-left px-4 py-3 font-medium">Причина</th>
                  <th className="text-left px-4 py-3 font-medium">Статус</th>
                </tr>
              </thead>
              <tbody>
                {changes.map(c => (
                  <tr key={c.id} className="border-b border-[#f9f9f9] hover:bg-[#fafafa]">
                    <td className="px-4 py-3 text-[#aaa] text-xs whitespace-nowrap">
                      {formatDate(c.created_at)}
                    </td>
                    <td className="px-4 py-3 font-medium text-[#111] max-w-[180px]">
                      <p className="truncate">{c.product_name}</p>
                    </td>
                    <td className="px-4 py-3 text-right text-[#aaa]">{formatPrice(c.old_price)}</td>
                    <td className="px-4 py-3 text-right font-semibold text-[#111]">
                      {formatPrice(c.new_price)}
                    </td>
                    <td className="px-4 py-3 text-[#666] text-xs">{c.reason}</td>
                    <td className="px-4 py-3">
                      <StatusBadge status={c.status} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="px-4 py-3 border-t border-[#f5f5f5] flex flex-wrap items-center gap-3 text-xs text-[#666]">
          <span>Всего: {total}</span>
          <span className="text-[#aaa]">·</span>
          <span>
            Страница {Math.min(page, totalPages)} из {totalPages}
          </span>
          <div className="ml-auto flex items-center gap-2">
            <Select
              value={String(perPage)}
              onValueChange={v => setPerPage(Number(v) as PerPage)}
            >
              <SelectTrigger className="h-9 w-[88px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PER_PAGE_OPTIONS.map(n => (
                  <SelectItem key={n} value={String(n)}>
                    {n}/стр
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant="secondary"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="secondary"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </AppLayout>
  )
}
