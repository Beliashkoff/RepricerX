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
import type { Strategy, StrategyDetail, StrategyType, FallbackPolicy, CreateStrategyPayload, UpdateStrategyPayload, StrategyConstraints } from '@/types/api'
import { Plus, Trash2, ChevronDown, ChevronUp, ShieldCheck, Pencil } from 'lucide-react'
import { formatDate, formatPrice } from '@/lib/utils'

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
    case 'fixed': return 'Цена (₽)'
    case 'below_median_pct': return 'Процент ниже медианы'
    case 'min_competitor_plus_step': return 'Шаг от минимума конкурента (₽)'
    case 'min_margin_pct': return 'Минимальная маржа %'
  }
}

function paramValueFromStrategy(s: Strategy): string {
  const p = s.params as Record<string, number>
  switch (s.type) {
    case 'fixed': return p.value != null ? String(p.value) : ''
    case 'below_median_pct': return p.pct != null ? String(p.pct) : ''
    case 'min_competitor_plus_step': return p.step != null ? String(p.step) : ''
    case 'min_margin_pct': return p.margin_pct != null ? String(p.margin_pct) : ''
  }
}

function paramSummary(s: Strategy): string {
  const p = s.params as Record<string, number>
  switch (s.type) {
    case 'fixed': return `Цена: ${formatPrice(p.value)}`
    case 'below_median_pct': return `−${p.pct}% от медианы`
    case 'min_competitor_plus_step': return `Мин. конкурент +${p.step} ₽`
    case 'min_margin_pct': return `Маржа: ${p.margin_pct}%`
  }
}

function isCompetitorType(type: StrategyType): boolean {
  return type === 'below_median_pct' || type === 'min_competitor_plus_step'
}

// ─── ConstraintsSection ──────────────────────────────────────────────────────

function ConstraintsSection({
  constraints, onChange, fallbackPolicy,
}: {
  constraints: StrategyConstraints
  onChange: (c: StrategyConstraints) => void
  fallbackPolicy: FallbackPolicy
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
            <ConstraintField label="Мин. прибыль %" hint="от себестоимости" value={constraints.min_profit_pct} onChange={v => upd('min_profit_pct', v)} />
            <ConstraintField label="Мин. прибыль ₽" hint="над себестоимостью" value={constraints.min_profit_abs} onChange={v => upd('min_profit_abs', v)} />
            <ConstraintField label="Макс. изм. цены %" hint="за один пересчёт" value={constraints.max_change_pct} onChange={v => upd('max_change_pct', v)} />
            <ConstraintField label="Мин. интервал (мин)" hint="между пересчётами, 1–1440" value={constraints.min_interval_minutes} onChange={v => upd('min_interval_minutes', v)} />
          </div>

          {fallbackPolicy === 'set_fixed' && (
            <div>
              <Label className="text-xs text-[#666]">
                Резервная цена (₽)
                <span className="text-[#aaa] font-normal"> — применяется когда формула не работает</span>
              </Label>
              <Input
                type="number"
                className="mt-1 h-8 text-sm"
                placeholder="Например: 999"
                value={constraints.fallback_price ?? ''}
                onChange={e => upd('fallback_price', e.target.value)}
              />
            </div>
          )}
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

// ─── Shared strategy form fields ─────────────────────────────────────────────

interface StrategyFormState {
  name: string
  type: StrategyType
  paramValue: string
  fallbackPolicy: FallbackPolicy
  enabled: boolean
  constraints: StrategyConstraints
}

function useStrategyForm(initial?: Partial<StrategyFormState>): [StrategyFormState, React.Dispatch<React.SetStateAction<StrategyFormState>>, () => void] {
  const defaults: StrategyFormState = {
    name: '',
    type: 'fixed',
    paramValue: '',
    fallbackPolicy: 'keep_current',
    enabled: true,
    constraints: {},
    ...initial,
  }
  const [state, setState] = useState<StrategyFormState>(defaults)
  const reset = () => setState(defaults)
  return [state, setState, reset]
}

// ─── CreateStrategyDialog ────────────────────────────────────────────────────

function CreateStrategyDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm, resetForm] = useStrategyForm()

  const upd = <K extends keyof StrategyFormState>(k: K, v: StrategyFormState[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  const { mutate, isPending } = useMutation({
    mutationFn: () => {
      const payload: CreateStrategyPayload = {
        name: form.name,
        type: form.type,
        params: paramsForType(form.type, form.paramValue),
        fallbackPolicy: form.fallbackPolicy,
        enabled: form.enabled,
      }
      const c = Object.fromEntries(Object.entries(form.constraints).filter(([, v]) => v !== undefined))
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

  return (
    <Dialog open={open} onOpenChange={v => { if (!v) { onClose(); resetForm() } }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Новая стратегия</DialogTitle>
        </DialogHeader>
        <StrategyFormFields
          form={form}
          onUpdate={upd}
          onChangeConstraints={c => upd('constraints', c)}
          submitLabel={isPending ? 'Создаём...' : 'Создать'}
          submitDisabled={!form.name || !form.paramValue || isPending}
          onSubmit={() => mutate()}
          onCancel={() => { onClose(); resetForm() }}
        />
      </DialogContent>
    </Dialog>
  )
}

// ─── EditStrategyDialog ──────────────────────────────────────────────────────

function EditStrategyDialog({ strategyId, open, onClose }: { strategyId: string; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()

  const { data: detail, isLoading } = useQuery({
    queryKey: ['strategy', strategyId],
    queryFn: () => strategiesApi.get(strategyId),
    enabled: open && !!strategyId,
  })

  return (
    <Dialog open={open} onOpenChange={v => { if (!v) onClose() }}>
      <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Редактировать стратегию</DialogTitle>
        </DialogHeader>
        {isLoading || !detail ? (
          <div className="flex justify-center py-8">
            <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
          </div>
        ) : (
          <EditStrategyForm
            detail={detail}
            onClose={onClose}
            onSaved={() => {
              qc.invalidateQueries({ queryKey: ['strategies'] })
              qc.invalidateQueries({ queryKey: ['strategy', strategyId] })
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function EditStrategyForm({ detail, onClose, onSaved }: { detail: StrategyDetail; onClose: () => void; onSaved: () => void }) {
  const [form, setForm] = useState<StrategyFormState>({
    name: detail.name,
    type: detail.type,
    paramValue: paramValueFromStrategy(detail),
    fallbackPolicy: detail.fallbackPolicy,
    enabled: detail.enabled,
    constraints: detail.constraints ?? {},
  })

  const upd = <K extends keyof StrategyFormState>(k: K, v: StrategyFormState[K]) =>
    setForm(f => ({ ...f, [k]: v }))

  const { mutate, isPending } = useMutation({
    mutationFn: () => {
      const patch: UpdateStrategyPayload = {
        name: form.name,
        type: form.type,
        params: paramsForType(form.type, form.paramValue),
        fallbackPolicy: form.fallbackPolicy,
        enabled: form.enabled,
      }
      const c = Object.fromEntries(Object.entries(form.constraints).filter(([, v]) => v !== undefined))
      patch.constraints = c as StrategyConstraints
      return strategiesApi.update(detail.id, patch)
    },
    onSuccess: () => {
      toast.success('Стратегия обновлена')
      onSaved()
      onClose()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <StrategyFormFields
      form={form}
      onUpdate={upd}
      onChangeConstraints={c => upd('constraints', c)}
      submitLabel={isPending ? 'Сохраняем...' : 'Сохранить'}
      submitDisabled={!form.name || !form.paramValue || isPending}
      onSubmit={() => mutate()}
      onCancel={onClose}
    />
  )
}

// ─── Shared form fields component ────────────────────────────────────────────

function StrategyFormFields({
  form, onUpdate, onChangeConstraints,
  submitLabel, submitDisabled, onSubmit, onCancel,
}: {
  form: StrategyFormState
  onUpdate: <K extends keyof StrategyFormState>(k: K, v: StrategyFormState[K]) => void
  onChangeConstraints: (c: StrategyConstraints) => void
  submitLabel: string
  submitDisabled: boolean
  onSubmit: () => void
  onCancel: () => void
}) {
  return (
    <div className="flex flex-col gap-4">
      <div>
        <Label htmlFor="st-name">Название</Label>
        <Input
          id="st-name" className="mt-1.5" placeholder="Моя стратегия"
          value={form.name} onChange={e => onUpdate('name', e.target.value)}
        />
      </div>

      <div>
        <Label>Тип стратегии</Label>
        <Select value={form.type} onValueChange={v => { onUpdate('type', v as StrategyType); onUpdate('paramValue', '') }}>
          <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
          <SelectContent>
            {(Object.keys(TYPE_LABELS) as StrategyType[]).map(t => (
              <SelectItem key={t} value={t}>{TYPE_LABELS[t]}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {isCompetitorType(form.type) && (
          <p className="mt-1.5 text-xs text-[#888]">
            Для расчёта нужны конкуренты — добавьте их на странице товара.
          </p>
        )}
      </div>

      <div>
        <Label htmlFor="st-param">{paramLabelForType(form.type)}</Label>
        <Input
          id="st-param" type="number" className="mt-1.5"
          placeholder={form.type === 'fixed' ? '1000' : '5'}
          value={form.paramValue}
          onChange={e => onUpdate('paramValue', e.target.value)}
        />
      </div>

      <div>
        <Label>Резервная политика</Label>
        <Select value={form.fallbackPolicy} onValueChange={v => onUpdate('fallbackPolicy', v as FallbackPolicy)}>
          <SelectTrigger className="mt-1.5"><SelectValue /></SelectTrigger>
          <SelectContent>
            {(Object.keys(FALLBACK_LABELS) as FallbackPolicy[]).map(p => (
              <SelectItem key={p} value={p}>{FALLBACK_LABELS[p]}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {form.fallbackPolicy === 'set_fixed' && (
          <p className="mt-1.5 text-xs text-[#888]">
            Укажите резервную цену в разделе «Ограничения» ниже.
          </p>
        )}
        {form.fallbackPolicy === 'set_min' && (
          <p className="mt-1.5 text-xs text-[#888]">
            Устанавливает «Мин. цену» из раздела ограничений. Если мин. цена не задана — сохраняет текущую.
          </p>
        )}
      </div>

      <ConstraintsSection
        constraints={form.constraints}
        onChange={onChangeConstraints}
        fallbackPolicy={form.fallbackPolicy}
      />

      <div className="flex items-center gap-3">
        <input
          id="st-enabled"
          type="checkbox"
          className="h-4 w-4 rounded border-[#e6e6e6] accent-[#ffcc00]"
          checked={form.enabled}
          onChange={e => onUpdate('enabled', e.target.checked)}
        />
        <Label htmlFor="st-enabled">Включена</Label>
      </div>

      <div className="flex gap-3 pt-2">
        <Button variant="secondary" className="flex-1" onClick={onCancel}>Отмена</Button>
        <Button className="flex-1" disabled={submitDisabled} onClick={onSubmit}>
          {submitLabel}
        </Button>
      </div>
    </div>
  )
}

// ─── Main page ───────────────────────────────────────────────────────────────

export default function Strategies() {
  const [createOpen, setCreateOpen] = useState(false)
  const [editId, setEditId] = useState<string | null>(null)
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
                  <span className="text-xs text-[#666] font-medium">{paramSummary(s)}</span>
                  {s.assignedCount > 0 && (
                    <Badge variant="outline" className="text-xs">
                      {s.assignedCount} товар{s.assignedCount === 1 ? '' : s.assignedCount < 5 ? 'а' : 'ов'}
                    </Badge>
                  )}
                  <span className="text-xs text-[#aaa]">Создана {formatDate(s.createdAt)}</span>
                </div>
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <Button
                  variant="ghost" size="icon"
                  title="Редактировать стратегию"
                  onClick={() => setEditId(s.id)}
                >
                  <Pencil className="h-4 w-4 text-[#aaa] hover:text-[#555] transition-colors" />
                </Button>
                <Button
                  variant="ghost" size="icon"
                  onClick={() => deleteMutation.mutate(s.id)}
                  disabled={deleteMutation.isPending}
                  title="Удалить стратегию"
                >
                  <Trash2 className="h-4 w-4 text-[#aaa] hover:text-red-500 transition-colors" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <CreateStrategyDialog open={createOpen} onClose={() => setCreateOpen(false)} />
      {editId && (
        <EditStrategyDialog
          strategyId={editId}
          open={!!editId}
          onClose={() => setEditId(null)}
        />
      )}
    </AppLayout>
  )
}
