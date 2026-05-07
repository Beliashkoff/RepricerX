import { useQuery } from '@tanstack/react-query'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { auditApi } from '@/api/audit'
import type { PriceChange } from '@/types/api'
import { formatPrice, formatDate } from '@/lib/utils'
import { Download } from 'lucide-react'

function StatusBadge({ status }: { status: PriceChange['status'] }) {
  const map = { success: 'success', failed: 'destructive', skipped: 'warning' } as const
  const labels = { success: 'Успешно', failed: 'Ошибка', skipped: 'Пропущено' }
  return <Badge variant={map[status]}>{labels[status]}</Badge>
}

export default function Audit() {
  const { data: changes = [], isLoading } = useQuery({ queryKey: ['audit'], queryFn: auditApi.listChanges })
  const { data: summary } = useQuery({ queryKey: ['summary'], queryFn: auditApi.getSummary })

  return (
    <AppLayout>
      <PageHeader
        title="Журнал изменений"
        description="История обновлений цен за последние 180 дней"
        action={
          <Button variant="secondary" size="sm" className="gap-1.5" onClick={() => auditApi.exportCsv()}>
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

      <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
        {isLoading ? (
          <div className="flex justify-center py-12"><div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" /></div>
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
                    <td className="px-4 py-3 text-[#aaa] text-xs whitespace-nowrap">{formatDate(c.created_at)}</td>
                    <td className="px-4 py-3 font-medium text-[#111] max-w-[180px]"><p className="truncate">{c.product_name}</p></td>
                    <td className="px-4 py-3 text-right text-[#aaa]">{formatPrice(c.old_price)}</td>
                    <td className="px-4 py-3 text-right font-semibold text-[#111]">{formatPrice(c.new_price)}</td>
                    <td className="px-4 py-3 text-[#666] text-xs">{c.reason}</td>
                    <td className="px-4 py-3"><StatusBadge status={c.status} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <div className="px-4 py-3 border-t border-[#f5f5f5] text-xs text-[#aaa]">
          Показано {changes.length} записей
        </div>
      </div>
    </AppLayout>
  )
}
