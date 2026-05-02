import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { productsApi } from '@/api/products'
import type { Product } from '@/types/api'
import { formatPrice, formatDate } from '@/lib/utils'
import { Download, Search, RefreshCw } from 'lucide-react'

function StatusBadge({ status }: { status: Product['status'] }) {
  const map = { active: 'success', archived: 'secondary', paused: 'warning' } as const
  const labels = { active: 'Активен', archived: 'В архиве', paused: 'На паузе' }
  return <Badge variant={map[status]}>{labels[status]}</Badge>
}

export default function Products() {
  const [search, setSearch] = useState('')
  const { data: products = [], isLoading, refetch } = useQuery({
    queryKey: ['products'],
    queryFn: () => productsApi.list(),
  })

  const importMutation = useMutation({
    mutationFn: () => productsApi.startImport(''),
    onSuccess: () => toast.success('Импорт запущен. Товары появятся через несколько минут.'),
    onError: (e: Error) => toast.error(e.message),
  })

  const filtered = products.filter(p =>
    p.name.toLowerCase().includes(search.toLowerCase()) ||
    p.external_sku.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <AppLayout>
      <PageHeader
        title="Товары"
        description="Каталог SKU из подключённых магазинов"
        action={
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" className="gap-1.5" onClick={() => refetch()}>
              <RefreshCw className="h-3.5 w-3.5" />
              Обновить
            </Button>
            <Button size="sm" className="gap-1.5" disabled={importMutation.isPending} onClick={() => importMutation.mutate()}>
              <Download className="h-3.5 w-3.5" />
              {importMutation.isPending ? 'Импорт...' : 'Импортировать'}
            </Button>
          </div>
        }
      />

      <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
        <div className="p-4 border-b border-[#e6e6e6]">
          <div className="relative max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[#aaa]" />
            <Input
              className="pl-9"
              placeholder="Поиск по названию или SKU..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-12">
            <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[#f5f5f5] text-[#666] text-xs">
                  <th className="text-left px-4 py-3 font-medium">Товар</th>
                  <th className="text-left px-4 py-3 font-medium">SKU</th>
                  <th className="text-right px-4 py-3 font-medium">Текущая цена</th>
                  <th className="text-right px-4 py-3 font-medium">Мин / Макс</th>
                  <th className="text-left px-4 py-3 font-medium">Статус</th>
                  <th className="text-left px-4 py-3 font-medium">Обновлено</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map(p => (
                  <tr key={p.id} className="border-b border-[#f9f9f9] hover:bg-[#fafafa] transition-colors">
                    <td className="px-4 py-3 font-medium text-[#111] max-w-[200px]">
                      <p className="truncate">{p.name}</p>
                    </td>
                    <td className="px-4 py-3 text-[#666] font-mono text-xs">{p.external_sku}</td>
                    <td className="px-4 py-3 text-right font-semibold text-[#111]">{formatPrice(p.current_price)}</td>
                    <td className="px-4 py-3 text-right text-[#666] text-xs">
                      {p.min_price ? formatPrice(p.min_price) : '—'} / {p.max_price ? formatPrice(p.max_price) : '—'}
                    </td>
                    <td className="px-4 py-3"><StatusBadge status={p.status} /></td>
                    <td className="px-4 py-3 text-[#aaa] text-xs">{formatDate(p.updated_at)}</td>
                  </tr>
                ))}
                {filtered.length === 0 && (
                  <tr>
                    <td colSpan={6} className="px-4 py-12 text-center text-[#aaa] text-sm">Товары не найдены</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        <div className="px-4 py-3 border-t border-[#f5f5f5] text-xs text-[#aaa]">
          Показано {filtered.length} из {products.length} товаров
          <span className="ml-2 text-[#ffcc00] font-medium">⚠ Данные mock — бэкенд в разработке</span>
        </div>
      </div>
    </AppLayout>
  )
}
