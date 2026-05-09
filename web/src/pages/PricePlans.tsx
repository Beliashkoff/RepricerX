import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { AppLayout, PageHeader, EmptyState } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { pricingApi, type PricePlanSummary, type PricePlanItem } from '@/api/pricing'
import { formatPrice, formatDate } from '@/lib/utils'
import { ArrowLeft, ListChecks, Loader2, CheckCircle2, AlertTriangle, Ban } from 'lucide-react'

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { label: string; cls: string; icon: React.ReactNode }> = {
    pending:    { label: 'Ожидание',     cls: 'bg-gray-100 text-gray-700',     icon: <Loader2 className="h-3 w-3 animate-spin" /> },
    processing: { label: 'Обработка',    cls: 'bg-blue-100 text-blue-700',     icon: <Loader2 className="h-3 w-3 animate-spin" /> },
    applied:    { label: 'Готово',       cls: 'bg-green-100 text-green-700',   icon: <CheckCircle2 className="h-3 w-3" /> },
    failed:     { label: 'Ошибка',       cls: 'bg-red-100 text-red-700',       icon: <AlertTriangle className="h-3 w-3" /> },
    cancelled:  { label: 'Отменён',      cls: 'bg-gray-100 text-gray-500',     icon: <Ban className="h-3 w-3" /> },
    skipped:    { label: 'Пропущен',     cls: 'bg-orange-100 text-orange-700', icon: <AlertTriangle className="h-3 w-3" /> },
  }
  const info = map[status] || { label: status, cls: 'bg-gray-100 text-gray-700', icon: null }
  return (
    <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full font-medium ${info.cls}`}>
      {info.icon}{info.label}
    </span>
  )
}

function ConstraintBadge({ hit }: { hit: string }) {
  if (!hit) return <span className="text-[#aaa] text-xs">—</span>
  const map: Record<string, string> = {
    cost_price_floor: 'bg-blue-100 text-blue-700',
    max_change_pct:   'bg-yellow-100 text-yellow-700',
    max_price:        'bg-gray-100 text-gray-700',
    min_price:        'bg-gray-100 text-gray-700',
    min_profit_pct:   'bg-purple-100 text-purple-700',
    min_profit_abs:   'bg-purple-100 text-purple-700',
  }
  return <span className={`text-xs px-1.5 py-0.5 rounded ${map[hit] || 'bg-gray-100 text-gray-700'}`}>{hit}</span>
}

export function PricePlansList() {
  const navigate = useNavigate()
  const { data, isLoading } = useQuery({
    queryKey: ['price-plans'],
    queryFn: () => pricingApi.listPlans(),
    refetchInterval: 5000,
  })

  return (
    <AppLayout>
      <PageHeader title="Планы пересчёта цен" description="История запусков перерасчёта цен" />
      {isLoading ? (
        <div className="flex justify-center py-12"><Loader2 className="h-6 w-6 animate-spin text-[#ffcc00]" /></div>
      ) : !data || data.items.length === 0 ? (
        <EmptyState
          title="Нет планов"
          description="Запустите пересчёт цен на странице товаров"
          action={<Button onClick={() => navigate('/products')}>К товарам</Button>}
        />
      ) : (
        <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#f5f5f5] text-[#666] text-xs">
                <th className="text-left px-4 py-3 font-medium">Дата</th>
                <th className="text-left px-4 py-3 font-medium">Магазин</th>
                <th className="text-left px-4 py-3 font-medium">Статус</th>
                <th className="text-right px-4 py-3 font-medium">Товаров</th>
                <th className="w-20" />
              </tr>
            </thead>
            <tbody>
              {data.items.map((p: PricePlanSummary) => (
                <tr key={p.id} className="border-b border-[#f9f9f9] hover:bg-[#fafafa]">
                  <td className="px-4 py-3 text-[#111]">{formatDate(p.created_at)}</td>
                  <td className="px-4 py-3 text-[#666] text-xs font-mono">{p.shop_id.slice(0, 8)}…</td>
                  <td className="px-4 py-3"><StatusBadge status={p.status} /></td>
                  <td className="px-4 py-3 text-right">{p.total}</td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" size="sm" onClick={() => navigate(`/price-plans/${p.id}`)}>Открыть</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </AppLayout>
  )
}

export function PricePlanDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const { data, isLoading } = useQuery({
    queryKey: ['price-plan', id],
    queryFn: () => pricingApi.getPlan(id!),
    enabled: !!id,
    refetchInterval: (q) => {
      const s = q.state.data?.plan.status
      if (s === 'applied' || s === 'failed' || s === 'cancelled') return false
      return 2000
    },
  })

  return (
    <AppLayout>
      <div className="flex items-center gap-3 mb-6">
        <Button variant="ghost" size="sm" onClick={() => navigate('/price-plans')} className="gap-2">
          <ArrowLeft className="h-4 w-4" /> К списку
        </Button>
      </div>

      {isLoading || !data ? (
        <div className="flex justify-center py-12"><Loader2 className="h-6 w-6 animate-spin text-[#ffcc00]" /></div>
      ) : (
        <>
          <div className="bg-white rounded-2xl border border-[#e6e6e6] p-6 mb-5">
            <div className="flex items-center justify-between gap-4 flex-wrap">
              <div>
                <h2 className="text-lg font-semibold flex items-center gap-2">
                  <ListChecks className="h-5 w-5 text-[#ffcc00]" />
                  План #{data.plan.id.slice(0, 8)}
                </h2>
                <p className="text-xs text-[#888] mt-1">Создан {formatDate(data.plan.created_at)}</p>
              </div>
              <div className="flex items-center gap-3">
                <StatusBadge status={data.plan.status} />
                <span className="text-sm text-[#666]">{data.plan.total} позиций</span>
              </div>
            </div>
          </div>

          {data.items.length === 0 && data.plan.status === 'applied' ? (
            <EmptyState title="План пуст" description="Возможно, ни одному товару не назначена стратегия" />
          ) : (
            <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[#f5f5f5] text-[#666] text-xs">
                    <th className="text-left px-4 py-3 font-medium">Товар</th>
                    <th className="text-right px-4 py-3 font-medium">Текущая</th>
                    <th className="text-right px-4 py-3 font-medium">Цель</th>
                    <th className="text-right px-4 py-3 font-medium">Итог</th>
                    <th className="text-left px-4 py-3 font-medium">Ограничение</th>
                    <th className="text-left px-4 py-3 font-medium">Статус</th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((it: PricePlanItem) => (
                    <tr key={it.id} className="border-b border-[#f9f9f9] hover:bg-[#fafafa]">
                      <td className="px-4 py-3 text-[#111] truncate max-w-[280px]">{it.product_name}</td>
                      <td className="px-4 py-3 text-right text-[#666]">{formatPrice(it.current_price)}</td>
                      <td className="px-4 py-3 text-right text-[#666]">{it.target_price > 0 ? formatPrice(it.target_price) : '—'}</td>
                      <td className="px-4 py-3 text-right font-semibold text-[#111]">{formatPrice(it.final_price)}</td>
                      <td className="px-4 py-3"><ConstraintBadge hit={it.constraint_hit} /></td>
                      <td className="px-4 py-3"><StatusBadge status={it.status} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </AppLayout>
  )
}
