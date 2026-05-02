import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { pricingApi, type SimulateResult } from '@/api/pricing'
import { productsApi } from '@/api/products'
import { strategiesApi } from '@/api/strategies'
import { formatPrice } from '@/lib/utils'
import { ArrowDown, ArrowUp, TrendingDown, CheckCircle2, AlertCircle } from 'lucide-react'
import { toast } from 'sonner'

export default function Pricing() {
  const { data: products = [] } = useQuery({ queryKey: ['products'], queryFn: () => productsApi.list() })
  const { data: strategies = [] } = useQuery({ queryKey: ['strategies'], queryFn: strategiesApi.list })

  const [productId, setProductId] = useState('')
  const [strategyId, setStrategyId] = useState('')
  const [competitorPrice, setCompetitorPrice] = useState('')
  const [result, setResult] = useState<SimulateResult | null>(null)

  const selectedProduct = products.find(p => p.id === productId)

  const { mutate, isPending } = useMutation({
    mutationFn: () => pricingApi.simulate({
      product_id: productId,
      strategy_id: strategyId,
      current_price: selectedProduct?.current_price ?? 0,
      competitor_price: competitorPrice ? Number(competitorPrice) : undefined,
      cost_price: selectedProduct?.cost_price ?? undefined,
    }),
    onSuccess: (data) => setResult(data),
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <AppLayout>
      <PageHeader title="Симуляция цен" description="Рассчитайте итоговую цену без отправки в маркетплейс" />

      <div className="bg-[#fffae6] border border-[#ffcc00]/30 rounded-2xl px-5 py-3 text-xs text-[#7a6000] mb-5">
        ⚠ Симуляция работает с mock-данными. Реальный движок расчёта — Этап 5.
      </div>

      <div className="grid lg:grid-cols-2 gap-6">
        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-6">
          <h3 className="text-base font-semibold text-[#111] mb-5">Параметры симуляции</h3>
          <div className="flex flex-col gap-4">
            <div>
              <Label>Товар</Label>
              <Select value={productId} onValueChange={setProductId}>
                <SelectTrigger className="mt-1.5"><SelectValue placeholder="Выберите товар" /></SelectTrigger>
                <SelectContent>
                  {products.map(p => <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label>Стратегия</Label>
              <Select value={strategyId} onValueChange={setStrategyId}>
                <SelectTrigger className="mt-1.5"><SelectValue placeholder="Выберите стратегию" /></SelectTrigger>
                <SelectContent>
                  {strategies.map(s => <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            {selectedProduct && (
              <div className="bg-[#f7f8fa] rounded-xl p-4 text-sm">
                <p className="text-[#666] mb-1">Текущая цена</p>
                <p className="font-semibold text-[#111]">{formatPrice(selectedProduct.current_price)}</p>
              </div>
            )}
            <div>
              <Label htmlFor="comp-price">Цена конкурента (опционально)</Label>
              <Input id="comp-price" type="number" className="mt-1.5" placeholder="Например: 8500" value={competitorPrice} onChange={e => setCompetitorPrice(e.target.value)} />
            </div>
            <Button disabled={!productId || !strategyId || isPending} onClick={() => mutate()} className="mt-2">
              {isPending ? 'Считаем...' : 'Рассчитать'}
            </Button>
          </div>
        </div>

        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-6">
          <h3 className="text-base font-semibold text-[#111] mb-5">Результат</h3>
          {!result ? (
            <div className="flex flex-col items-center justify-center py-12 text-[#aaa]">
              <TrendingDown className="w-10 h-10 mb-3 text-[#e6e6e6]" />
              <p className="text-sm">Заполните параметры и нажмите «Рассчитать»</p>
            </div>
          ) : (
            <div className="flex flex-col gap-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-[#f7f8fa] rounded-2xl p-4">
                  <p className="text-xs text-[#666] mb-1">Целевая цена</p>
                  <p className="text-2xl font-bold text-[#111]">{formatPrice(result.target_price)}</p>
                </div>
                <div className="bg-[#ffcc00] rounded-2xl p-4">
                  <p className="text-xs text-[#111]/60 mb-1">Итоговая цена</p>
                  <p className="text-2xl font-bold text-[#111]">{formatPrice(result.final_price)}</p>
                </div>
              </div>

              <div className="flex items-center gap-2 text-sm">
                {result.change_pct < 0 ? (
                  <><ArrowDown className="h-4 w-4 text-green-500" /><span className="text-green-600 font-medium">{result.change_pct.toFixed(1)}%</span><span className="text-[#666]">от текущей цены</span></>
                ) : (
                  <><ArrowUp className="h-4 w-4 text-red-500" /><span className="text-red-500 font-medium">+{result.change_pct.toFixed(1)}%</span><span className="text-[#666]">от текущей цены</span></>
                )}
              </div>

              <div className="bg-[#f7f8fa] rounded-xl p-4">
                <p className="text-xs text-[#666] mb-1">Причина</p>
                <p className="text-sm text-[#333]">{result.reason}</p>
              </div>

              {result.constraint_hit ? (
                <div className="flex items-start gap-2 bg-yellow-50 rounded-xl p-3 text-xs text-yellow-700">
                  <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
                  Сработало ограничение: {result.constraint_hit}
                </div>
              ) : (
                <div className="flex items-center gap-2 bg-green-50 rounded-xl p-3 text-xs text-green-700">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  Ограничений не сработало
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </AppLayout>
  )
}
