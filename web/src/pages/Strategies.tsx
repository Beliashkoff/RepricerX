import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AppLayout, PageHeader, EmptyState } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { strategiesApi } from '@/api/strategies'
import type { Strategy, StrategyType } from '@/types/api'
import { Plus, Trash2 } from 'lucide-react'
import { formatDate } from '@/lib/utils'

const TYPE_LABELS: Record<StrategyType, string> = {
  below_median_pct: 'Ниже медианы на %',
  min_competitor_plus_step: 'Мин. конкурент + шаг',
  min_margin_pct: 'Минимальная маржа %',
  fixed: 'Фиксированная цена',
}

function StrategyTypeBadge({ type }: { type: StrategyType }) {
  return <Badge variant="secondary">{TYPE_LABELS[type]}</Badge>
}

function CreateStrategyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [type, setType] = useState<StrategyType>('below_median_pct')
  const [param, setParam] = useState('')

  const { mutate, isPending } = useMutation({
    mutationFn: () => strategiesApi.create({
      name,
      type,
      params: { pct: Number(param), step: Number(param), margin_pct: Number(param), price: Number(param) },
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['strategies'] })
      toast.success('Стратегия создана')
      onClose()
      setName('')
      setParam('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const paramLabel = {
    below_median_pct: 'Процент ниже медианы (0–20)',
    min_competitor_plus_step: 'Шаг от минимума (₽)',
    min_margin_pct: 'Минимальная маржа % (0–90)',
    fixed: 'Фиксированная цена (₽)',
  }[type]

  return (
    <Dialog open={open} onOpenChange={v => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Новая стратегия</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div>
            <Label htmlFor="st-name">Название</Label>
            <Input id="st-name" className="mt-1.5" placeholder="Моя стратегия" value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div>
            <Label>Тип стратегии</Label>
            <Select value={type} onValueChange={v => setType(v as StrategyType)}>
              <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
              <SelectContent>
                {(Object.keys(TYPE_LABELS) as StrategyType[]).map(t => (
                  <SelectItem key={t} value={t}>{TYPE_LABELS[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label htmlFor="st-param">{paramLabel}</Label>
            <Input id="st-param" type="number" className="mt-1.5" placeholder="5" value={param} onChange={e => setParam(e.target.value)} />
          </div>
          <div className="flex gap-3 pt-2">
            <Button variant="secondary" className="flex-1" onClick={onClose}>Отмена</Button>
            <Button className="flex-1" disabled={!name || !param || isPending} onClick={() => mutate()}>
              {isPending ? 'Создаём...' : 'Создать'}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export default function Strategies() {
  const [createOpen, setCreateOpen] = useState(false)
  const qc = useQueryClient()
  const { data: strategies = [], isLoading } = useQuery({ queryKey: ['strategies'], queryFn: strategiesApi.list })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => strategiesApi.delete(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['strategies'] }); toast.success('Стратегия удалена') },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <AppLayout>
      <PageHeader
        title="Стратегии"
        description="Правила автоматического изменения цен"
        action={<Button onClick={() => setCreateOpen(true)} className="gap-2"><Plus className="h-4 w-4" />Создать стратегию</Button>}
      />

      <div className="bg-[#fffae6] border border-[#ffcc00]/30 rounded-2xl px-5 py-3 text-xs text-[#7a6000] mb-5">
        ⚠ Управление стратегиями работает в режиме mock. Реальный бэкенд — в разработке (Этап 4).
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12"><div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" /></div>
      ) : strategies.length === 0 ? (
        <EmptyState title="Нет стратегий" description="Создайте первую стратегию для автообновления цен" action={<Button onClick={() => setCreateOpen(true)} className="gap-2"><Plus className="h-4 w-4" />Создать стратегию</Button>} />
      ) : (
        <div className="flex flex-col gap-3">
          {strategies.map((s: Strategy) => (
            <div key={s.id} className="bg-white rounded-2xl border border-[#e6e6e6] p-5 flex items-center justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2 mb-1">
                  <p className="font-semibold text-[#111] truncate">{s.name}</p>
                  {!s.enabled && <Badge variant="secondary">Отключена</Badge>}
                </div>
                <div className="flex items-center gap-2 flex-wrap">
                  <StrategyTypeBadge type={s.type} />
                  <span className="text-xs text-[#aaa]">Создана {formatDate(s.created_at)}</span>
                </div>
              </div>
              <Button variant="ghost" size="icon" onClick={() => deleteMutation.mutate(s.id)} disabled={deleteMutation.isPending}>
                <Trash2 className="h-4 w-4 text-[#aaa] hover:text-red-500 transition-colors" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <CreateStrategyDialog open={createOpen} onClose={() => setCreateOpen(false)} />
    </AppLayout>
  )
}
