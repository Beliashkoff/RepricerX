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
import type { Strategy, StrategyType, FallbackPolicy, CreateStrategyPayload, StrategyConstraints } from '@/types/api'
import { Plus, Trash2, ChevronDown, ChevronUp, ShieldCheck } from 'lucide-react'
import { formatDate } from '@/lib/utils'

const TYPE_LABELS: Record<StrategyType, string> = {
  below_median_pct: 'Ниже медианы на %',
  min_competitor_plus_step: 'Мин. конкурент + шаг',
  min_margin_pct: 'Минимальная маржа %',
  fixed: 'Фиксированная цена',
}

const FALLBACK_LABELS: Record<FallbackPolicy, string> = {
  keep_current: 'Сохранить текущую цену',
  set_fixed: 'Установить фикс. цену',
  set_min: 'Установить минимальную цену',
}

function StrategyTypeBadge({ type }: { type: StrategyType }) {
  return <Badge variant="secondary">{TYPE_LABELS[type]}</Badge>
}

function paramsForType(type: StrategyType, value: string): Record<string, unknown> {
  const n = parseFloat(value)
  switch (type) {
    case 'fixed': return { value: n }
    case 'below_median_pct': return { pct: n }
    case 'min_competitor_plus_step': return { step: n }
    case 'min_margin_pct': return { margin_pct: n }
  }
}

function paramLabelForType(type: StrategyType): string {
  switch (type) {
    case 'fixed': return 'Цена (₽), >0'
    case 'below_median_pct': return 'Процент ниже медианы (0–20)'
    case 'min_competitor_plus_step': return 'Шаг от минимума конкурента (₽, 0–500)'
    case 'min_margin_pct': return 'Минимальная маржа % (0–90)'
  }
}

function isCompetitorType(type: StrategyType): boolean {
  return type === 'below_median_pct' || type === 'min_competitor_plus_step'
}

// ConstraintsSection — раскрывающийся блок ограничений
function ConstraintsSection({
  constraints, onChange,
}: {
  constraints: StrategyConstraints
  onChange: (c: StrategyConstraints) => void
}) {
  const [open, setOpen] = useState(false)

  const upd = (key: keyof StrategyConstraints, raw: string) => {
    const v = raw === '' ? undefined : Number(raw)
    onChange({ ...constraints, [key]: v })
  }

  return (
    <div className="border border-[#e6e6e6] rounded-xl overflow-hidden">
      <button
        type="button"
        className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-[#555] hover:bg-[#fafafa]"
        onClick={() => setOpen(o => !o)}
      >
        <span className="flex items-center gap-2">
          <ShieldCheck className="h-4 w-4 text-[#aaa]" />
          Ограничения (необязательно)
        </span>
        {open ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
      </button>
      {open && (
        <div className="px-4 pb-4 pt-2 flex flex-col gap-3 bg-[#fafafa]">
          <p className="text-xs text-[#888]">Все поля необязательны. Оставьте пустым — ограничение не применяется.</p>

          <div className="bg-[#fffae6] border border-[#ffcc00]/40 rounded-lg px-3 py-2 text-xs text-[#7a6000] flex items-start gap-2">
            <ShieldCheck className="h-3.5 w-3.5 mt-0.5 shrink-0" />
            <span>
              <b>Защита от убытков:</b> укажите «Мин. прибыль %» или «Мин. прибыль ₽»,
              чтобы цена никогда не опускалась ниже себестоимости + заданного порога.
              Если оба заданы — действует более строгое.
            </span>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <ConstraintField label="Мин. цена (₽)" value={constraints.min_price} onChange={v => upd('min_price', v)} />
            <ConstraintField label="Макс. цена (₽)" value={constraints.max_price} onChange={v => upd('max_price', v)} />
            <ConstraintField label="Мин. прибыль %" hint="от себестоимости, 0–90" value={constraints.min_profit_pct} onChange={v => upd('min_profit_pct', v)} />
            <ConstraintField label="Мин. прибыль ₽" hint="над себестоимостью" value={constraints.min_profit_abs} onChange={v => upd('min_profit_abs', v)} />
            <ConstraintField label="Макс. изм. цены %" hint="за один пересчёт, 0–50" value={constraints.max_change_pct} onChange={v => upd('max_change_pct', v)} />
            <ConstraintField label="Мин. интервал (мин)" hint="между пересчётами, 1–1440" value={constraints.min_interval_minutes} onChange={v => upd('min_interval_minutes', v)} />
          </div>
        </div>
      )}
    </div>
  )
}

function ConstraintField({
  label, hint, value, onChange,
}: {
  label: string
  hint?: string
  value: number | undefined
  onChange: (v: string) => void
}) {
  return (
    <div>
      <Label className="text-xs text-[#666]">{label}{hint && <span className="text-[#aaa] font-normal"> — {hint}</span>}</Label>
      <Input
        type="number"
        className="mt-1 h-8 text-sm"
        placeholder="—"
        value={value ?? ''}
        onChange={e => onChange(e.target.value)}
      />
    </div>
  )
}

// CreateStrategyDialog
function CreateStrategyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [type, setType] = useState<StrategyType>('fixed')
  const [paramValue, setParamValue] = useState('')
  const [fallbackPolicy, setFallbackPolicy] = useState<FallbackPolicy>('keep_current')
  const [enabled, setEnabled] = useState(true)
  const [constraints, setConstraints] = useState<StrategyConstraints>({})

  const { mutate, isPending } = useMutation({
    mutationFn: () => {
      const payload: CreateStrategyPayload = {
        name,
        type,
        params: paramsForType(type, paramValue),
        fallbackPolicy,
        enabled,
      }
      const c = Object.fromEntries(Object.entries(constraints).filter(([, v]) => v !== undefined))
      if (Object.keys(c).length > 0) payload.constraints = c as StrategyConstraints
      return strategiesApi.create(payload)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['strategies'] })
      toast.success('Стратегия создана')
      onClose()
      resetForm()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const resetForm = () => {
    setName(''); setParamValue(''); setType('fixed')
    setFallbackPolicy('keep_current'); setEnabled(true); setConstraints({})
  }

  return (
    <Dialog open={open} onOpenChange={v => { if (!v) { onClose(); resetForm() } }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
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
            <Select value={type} onValueChange={v => { setType(v as StrategyType); setParamValue('') }}>
              <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
              <SelectContent>
                {(Object.keys(TYPE_LABELS) as StrategyType[]).map(t => (
                  <SelectItem key={t} value={t}>{TYPE_LABELS[t]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {isCompetitorType(type) && (
              <p className="mt-1.5 text-xs text-[#888]">
                Для расчёта потребуется ввести цены конкурентов (Этап 5).
              </p>
            )}
          </div>

          <div>
            <Label htmlFor="st-param">{paramLabelForType(type)}</Label>
            <Input
              id="st-param" type="number" className="mt-1.5"
              placeholder={type === 'fixed' ? '1000' : '5'}
              value={paramValue}
              onChange={e => setParamValue(e.target.value)}
            />
          </div>

          <div>
            <Label>Резервная политика</Label>
            <Select value={fallbackPolicy} onValueChange={v => setFallbackPolicy(v as FallbackPolicy)}>
              <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
              <SelectContent>
                {(Object.keys(FALLBACK_LABELS) as FallbackPolicy[]).map(p => (
                  <SelectItem key={p} value={p}>{FALLBACK_LABELS[p]}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <ConstraintsSection constraints={constraints} onChange={setConstraints} />

          <div className="flex items-center gap-3">
            <input
              id="st-enabled"
              type="checkbox"
              className="h-4 w-4 rounded border-[#e6e6e6] accent-[#ffcc00]"
              checked={enabled}
              onChange={e => setEnabled(e.target.checked)}
            />
            <Label htmlFor="st-enabled">Включить сразу</Label>
          </div>

          <div className="flex gap-3 pt-2">
            <Button variant="secondary" className="flex-1" onClick={() => { onClose(); resetForm() }}>Отмена</Button>
            <Button className="flex-1" disabled={!name || !paramValue || isPending} onClick={() => mutate()}>
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
  const { data: strategies = [], isLoading } = useQuery({
    queryKey: ['strategies'],
    queryFn: strategiesApi.list,
  })

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

      {isLoading ? (
        <div className="flex justify-center py-12">
          <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
        </div>
      ) : strategies.length === 0 ? (
        <EmptyState
          title="Нет стратегий"
          description="Создайте первую стратегию для автообновления цен"
          action={<Button onClick={() => setCreateOpen(true)} className="gap-2"><Plus className="h-4 w-4" />Создать стратегию</Button>}
        />
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
                  {s.assignedCount > 0 && (
                    <Badge variant="outline" className="text-xs">
                      {s.assignedCount} товар{s.assignedCount === 1 ? '' : s.assignedCount < 5 ? 'а' : 'ов'}
                    </Badge>
                  )}
                  <span className="text-xs text-[#aaa]">Создана {formatDate(s.createdAt)}</span>
                </div>
              </div>
              <Button
                variant="ghost" size="icon"
                onClick={() => deleteMutation.mutate(s.id)}
                disabled={deleteMutation.isPending}
              >
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
