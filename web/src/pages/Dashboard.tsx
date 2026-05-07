import { useQuery } from '@tanstack/react-query'
import { AppLayout, PageHeader, StatCard } from '@/components/layout/AppLayout'
import { shopsApi } from '@/api/shops'
import { auditApi } from '@/api/audit'
import { formatPrice, formatDate } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { TrendingDown, TrendingUp } from 'lucide-react'
import type { ShopStatus } from '@/types/api'

function ShopStatusBadge({ status }: { status: ShopStatus }) {
  const map: Record<ShopStatus, 'success' | 'destructive' | 'warning' | 'secondary'> = {
    draft: 'warning',
    active: 'success',
    error: 'destructive',
    disabled: 'secondary',
  }
  const labels: Record<ShopStatus, string> = {
    draft: 'Черновик', active: 'Активен', error: 'Ошибка', disabled: 'Отключён',
  }
  return <Badge variant={map[status]}>{labels[status]}</Badge>
}

export default function Dashboard() {
  const { data: shops = [] } = useQuery({ queryKey: ['shops'], queryFn: shopsApi.list })
  const { data: summary } = useQuery({ queryKey: ['summary'], queryFn: auditApi.getSummary })
  const { data: changes = [] } = useQuery({ queryKey: ['audit'], queryFn: auditApi.listChanges })

  const activeShops = shops.filter(s => s.status === 'active').length
  const successPct = summary && summary.total_updates > 0
    ? `${Math.round(summary.successful_updates / summary.total_updates * 100)}%`
    : '—'

  return (
    <AppLayout>
      <PageHeader title="Дашборд" description="Общая статистика вашего аккаунта" />

      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatCard label="Магазины" value={shops.length} sub={`${activeShops} активных`} />
        <StatCard label="Обновлений за 30 дней" value={summary?.total_updates ?? '—'} sub="всего изменений" accent />
        <StatCard label="Успешных" value={successPct} sub="от общего числа" />
        <StatCard label="Среднее изменение" value={summary ? `${summary.avg_change_pct.toFixed(1)}%` : '—'} sub="за период" />
      </div>

      <div className="grid lg:grid-cols-2 gap-6">
        {/* Shops quick view */}
        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-5">
          <h3 className="text-sm font-semibold text-[#111] mb-4">Мои магазины</h3>
          {shops.length === 0 ? (
            <p className="text-sm text-[#aaa] py-4 text-center">Нет подключённых магазинов</p>
          ) : (
            <div className="flex flex-col gap-2">
              {shops.slice(0, 5).map(shop => (
                <div key={shop.id} className="flex items-center justify-between py-2 border-b border-[#f5f5f5] last:border-0">
                  <div>
                    <p className="text-sm font-medium text-[#111]">{shop.name}</p>
                    <p className="text-xs text-[#aaa]">{shop.marketplace.toUpperCase()}</p>
                  </div>
                  <ShopStatusBadge status={shop.status} />
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Recent changes */}
        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-5">
          <h3 className="text-sm font-semibold text-[#111] mb-4">Последние изменения цен</h3>
          <div className="flex flex-col gap-2">
            {changes.slice(0, 6).map(c => (
              <div key={c.id} className="flex items-center justify-between py-2 border-b border-[#f5f5f5] last:border-0">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-[#111] truncate">{c.product_name}</p>
                  <p className="text-xs text-[#aaa]">{formatDate(c.created_at)}</p>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className="text-xs text-[#aaa] line-through">{formatPrice(c.old_price)}</span>
                  <span className="text-sm font-semibold text-[#111]">{formatPrice(c.new_price)}</span>
                  {c.new_price < c.old_price ? (
                    <TrendingDown className="h-3 w-3 text-green-500" />
                  ) : (
                    <TrendingUp className="h-3 w-3 text-red-500" />
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </AppLayout>
  )
}
