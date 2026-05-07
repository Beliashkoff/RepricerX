import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { productsApi } from '@/api/products'
import { shopsApi } from '@/api/shops'
import { strategiesApi } from '@/api/strategies'
import type { ImportStatus, Product, ProductListParams, ProductSortField, SortDir, Strategy } from '@/types/api'
import { formatPrice, formatDate } from '@/lib/utils'
import { ArrowUpDown, ArrowUp, ArrowDown, Download, Search, FileDown, Pencil, X } from 'lucide-react'

// ─── constants ───────────────────────────────────────────────────────────────

const terminalStatuses: ImportStatus['status'][] = ['succeeded', 'partial', 'failed', 'canceled']
const activeImportStatuses: ImportStatus['status'][] = ['pending', 'running']
const allShopsValue = '__all_shops__'
const allStatusesValue = '__all_statuses__'

// ─── sub-components ──────────────────────────────────────────────────────────

function StatusBadge({ status }: { status: Product['status'] }) {
  const variants = { active: 'success', archived: 'secondary', out_of_stock: 'warning' } as const
  const labels = { active: 'Активен', archived: 'В архиве', out_of_stock: 'Нет в наличии' }
  return <Badge variant={variants[status]}>{labels[status]}</Badge>
}

function SortIcon({ field, sortBy, sortDir }: { field: ProductSortField; sortBy: ProductSortField; sortDir: SortDir }) {
  if (sortBy !== field) return <ArrowUpDown className="h-3 w-3 ml-1 opacity-40" />
  return sortDir === 'asc'
    ? <ArrowUp className="h-3 w-3 ml-1 text-[#ffcc00]" />
    : <ArrowDown className="h-3 w-3 ml-1 text-[#ffcc00]" />
}

function Skeleton({ className = '' }: { className?: string }) {
  return <div className={`animate-pulse bg-[#f0f0f0] rounded ${className}`} />
}

// ─── EditProductModal ────────────────────────────────────────────────────────

interface EditModalProps {
  product: Product
  onClose: () => void
  onSaved: () => void
}

function EditProductModal({ product, onClose, onSaved }: EditModalProps) {
  const [minPrice, setMinPrice] = useState(product.min_price?.toString() ?? '')
  const [maxPrice, setMaxPrice] = useState(product.max_price?.toString() ?? '')
  const [costPrice, setCostPrice] = useState(product.cost_price?.toString() ?? '')
  const [busy, setBusy] = useState(false)

  const parseOpt = (s: string) => (s.trim() === '' ? null : Number(s))

  const handleSave = async () => {
    setBusy(true)
    try {
      await productsApi.update(product.id, {
        min_price: parseOpt(minPrice),
        max_price: parseOpt(maxPrice),
        cost_price: parseOpt(costPrice),
      })
      toast.success('Цены обновлены')
      onSaved()
      onClose()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Ошибка сохранения цен')
    } finally {
      setBusy(false)
    }
  }

  const handleArchive = async () => {
    setBusy(true)
    try {
      await productsApi.softDelete(product.id)
      toast.success('Товар архивирован')
      onSaved()
      onClose()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Ошибка архивирования')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={open => { if (!open) onClose() }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="truncate">{product.name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="text-xs text-[#888]">SKU: {product.external_sku}</div>
          <div className="grid gap-3">
            <div className="space-y-1">
              <Label className="text-xs">Мин. цена</Label>
              <Input type="number" min={0} step="0.01" placeholder="Без ограничения"
                value={minPrice} onChange={e => setMinPrice(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Макс. цена</Label>
              <Input type="number" min={0} step="0.01" placeholder="Без ограничения"
                value={maxPrice} onChange={e => setMaxPrice(e.target.value)} />
            </div>
            <div className="space-y-1">
              <Label className="text-xs">Себестоимость</Label>
              <Input type="number" min={0} step="0.01" placeholder="Не задана"
                value={costPrice} onChange={e => setCostPrice(e.target.value)} />
            </div>
          </div>
        </div>
        <DialogFooter className="gap-2 flex-row justify-between">
          <Button variant="destructive" size="sm" onClick={handleArchive} disabled={busy}>
            Архивировать
          </Button>
          <div className="flex gap-2">
            <Button variant="secondary" size="sm" onClick={onClose} disabled={busy}>Отмена</Button>
            <Button size="sm" onClick={handleSave} disabled={busy}>Сохранить</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ─── ImportErrorsDialog ──────────────────────────────────────────────────────

function ImportErrorsDialog({ importId, onClose }: { importId: string; onClose: () => void }) {
  const [errPage, setErrPage] = useState(1)
  const perPage = 20

  const { data, isLoading } = useQuery({
    queryKey: ['import-errors', importId, errPage],
    queryFn: () => productsApi.getImportErrors(importId, errPage, perPage),
  })

  const totalPages = data ? Math.ceil(data.total / perPage) : 1

  return (
    <Dialog open onOpenChange={open => { if (!open) onClose() }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Ошибки импорта</DialogTitle>
        </DialogHeader>
        {isLoading ? (
          <div className="space-y-2 py-4">
            {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-8" />)}
          </div>
        ) : (
          <>
            <div className="overflow-auto max-h-80">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-xs text-[#666] border-b border-[#f0f0f0]">
                    <th className="text-left py-2 px-2 font-medium">SKU</th>
                    <th className="text-left py-2 px-2 font-medium">Код</th>
                    <th className="text-left py-2 px-2 font-medium">Сообщение</th>
                  </tr>
                </thead>
                <tbody>
                  {data?.items.map((e, i) => (
                    <tr key={i} className="border-b border-[#f9f9f9] text-xs">
                      <td className="py-2 px-2 font-mono text-[#666]">{e.externalSku || '—'}</td>
                      <td className="py-2 px-2 text-[#888]">{e.code}</td>
                      <td className="py-2 px-2">{e.message}</td>
                    </tr>
                  ))}
                  {data?.items.length === 0 && (
                    <tr><td colSpan={3} className="py-8 text-center text-[#aaa]">Ошибок нет</td></tr>
                  )}
                </tbody>
              </table>
            </div>
            {totalPages > 1 && (
              <div className="flex items-center justify-between pt-2 text-xs text-[#666]">
                <span>Всего: {data?.total}</span>
                <div className="flex gap-1">
                  <button className="px-2 py-1 rounded border border-[#e6e6e6] disabled:opacity-40"
                    disabled={errPage <= 1} onClick={() => setErrPage(p => p - 1)}>←</button>
                  <span className="px-2 py-1">{errPage} / {totalPages}</span>
                  <button className="px-2 py-1 rounded border border-[#e6e6e6] disabled:opacity-40"
                    disabled={errPage >= totalPages} onClick={() => setErrPage(p => p + 1)}>→</button>
                </div>
              </div>
            )}
          </>
        )}
        <DialogFooter>
          <Button variant="secondary" size="sm" onClick={onClose}>Закрыть</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ─── main component ──────────────────────────────────────────────────────────

export default function Products() {
  const queryClient = useQueryClient()

  // filter state
  const [page, setPage] = useState(1)
  const [perPage] = useState(50)
  const [sortBy, setSortBy] = useState<ProductSortField>('updated_at')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [shopFilter, setShopFilter] = useState<string>('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [priceFromStr, setPriceFromStr] = useState('')
  const [priceToStr, setPriceToStr] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchQ, setSearchQ] = useState('')

  // ui state
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [editProduct, setEditProduct] = useState<Product | null>(null)
  const [errorImportId, setErrorImportId] = useState<string | null>(null)
  const [activeImportId, setActiveImportId] = useState<string | null>(null)
  const [lastImportId, setLastImportId] = useState<string | null>(null)
  const [lastImportStatus, setLastImportStatus] = useState<ImportStatus | null>(null)

  // debounce search
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const handleSearchChange = useCallback((value: string) => {
    setSearchInput(value)
    if (searchTimer.current) clearTimeout(searchTimer.current)
    searchTimer.current = setTimeout(() => {
      setSearchQ(value)
      setPage(1)
    }, 300)
  }, [])

  // current filter params
  const listParams: ProductListParams = {
    page,
    perPage,
    sortBy,
    sortDir,
    shopId: shopFilter || undefined,
    status: (statusFilter as Product['status']) || undefined,
    q: searchQ || undefined,
    priceFrom: priceFromStr ? Number(priceFromStr) : undefined,
    priceTo: priceToStr ? Number(priceToStr) : undefined,
  }

  // data
  const { data: shops = [] } = useQuery({
    queryKey: ['shops'],
    queryFn: shopsApi.list,
  })

  const { data: listResult, isLoading } = useQuery({
    queryKey: ['products', listParams],
    queryFn: () => productsApi.list(listParams),
    placeholderData: prev => prev,
  })

  const products = listResult?.items ?? []
  const pagination = listResult?.pagination ?? { page: 1, perPage, total: 0 }
  const totalPages = Math.max(1, Math.ceil(pagination.total / perPage))

  // import polling
  const importStatusQuery = useQuery({
    queryKey: ['product-import', activeImportId],
    queryFn: () => productsApi.getImport(activeImportId as string),
    enabled: Boolean(activeImportId),
    refetchInterval: q => {
      const status = q.state.data?.status
      return status && terminalStatuses.includes(status) ? false : 3000
    },
  })

  useEffect(() => {
    const data = importStatusQuery.data
    const status = data?.status
    if (!status || !terminalStatuses.includes(status)) return
    if (activeImportId) {
      setLastImportId(activeImportId)
      setLastImportStatus(data)
    }
    if (status === 'succeeded') toast.success('Импорт завершён')
    if (status === 'partial') toast.warning('Импорт завершён с пропущенными SKU')
    if (status === 'failed') toast.error('Импорт завершился с ошибкой')
    if (status === 'canceled') toast.info('Импорт отменён')
    queryClient.invalidateQueries({ queryKey: ['products'] })
    setActiveImportId(null)
  }, [activeImportId, importStatusQuery.data, queryClient])

  // mutations
  const importMutation = useMutation({
    mutationFn: () => {
      if (!shopFilter) throw new Error('Выберите магазин для импорта')
      return productsApi.startImport(shopFilter)
    },
    onSuccess: data => {
      setActiveImportId(data.importId)
      setLastImportId(null)
      setLastImportStatus(null)
      toast.success('Импорт поставлен в очередь')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const cancelImportMutation = useMutation({
    mutationFn: (id: string) => productsApi.cancelImport(id),
    onSuccess: () => {
      toast.info('Запрос на отмену отправлен')
      queryClient.invalidateQueries({ queryKey: ['product-import', activeImportId] })
    },
    onError: () => toast.error('Не удалось отменить импорт'),
  })

  // sort toggle
  const handleSort = (field: ProductSortField) => {
    if (sortBy === field) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortBy(field)
      setSortDir('desc')
    }
    setPage(1)
    setSelectedIds(new Set())
  }

  // select logic
  const allPageSelected = products.length > 0 && products.every(p => selectedIds.has(p.id))
  const toggleAll = () => {
    if (allPageSelected) {
      setSelectedIds(prev => {
        const next = new Set(prev)
        products.forEach(p => next.delete(p.id))
        return next
      })
    } else {
      setSelectedIds(prev => {
        const next = new Set(prev)
        products.forEach(p => next.add(p.id))
        return next
      })
    }
  }
  const toggleOne = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const bulkArchiveMutation = useMutation({
    mutationFn: async () => {
      const ids = Array.from(selectedIds)
      await Promise.all(ids.map(id => productsApi.softDelete(id)))
    },
    onSuccess: () => {
      toast.success(`Архивировано: ${selectedIds.size}`)
      setSelectedIds(new Set())
      queryClient.invalidateQueries({ queryKey: ['products'] })
    },
    onError: () => toast.error('Ошибка архивирования'),
  })

  // ── Assign strategy ──────────────────────────────────────────────────────
  const [assignOpen, setAssignOpen] = useState(false)
  const [assignStrategyId, setAssignStrategyId] = useState<string>('')

  const { data: allStrategies = [] } = useQuery({
    queryKey: ['strategies'],
    queryFn: strategiesApi.list,
    enabled: assignOpen,
  })

  const assignMutation = useMutation({
    mutationFn: () => strategiesApi.assign(assignStrategyId, Array.from(selectedIds)),
    onSuccess: () => {
      toast.success(`Стратегия назначена на ${selectedIds.size} товар${selectedIds.size === 1 ? '' : 'а'}`)
      setAssignOpen(false)
      setAssignStrategyId('')
      setSelectedIds(new Set())
      queryClient.invalidateQueries({ queryKey: ['products'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const visibleImportId = activeImportId ?? lastImportId
  const importStatus = activeImportId ? importStatusQuery.data : lastImportStatus
  const isImportActive = Boolean(activeImportId && importStatus && activeImportStatuses.includes(importStatus.status))
  const isImportTerminal = Boolean(importStatus && terminalStatuses.includes(importStatus.status))

  return (
    <AppLayout>
      <PageHeader
        title="Товары"
        description="Каталог SKU из подключённых магазинов"
        action={
          <div className="flex gap-2 flex-wrap">
            <Button
              variant="secondary" size="sm" className="gap-1.5"
              onClick={() => productsApi.exportCsv(listParams).catch(() => toast.error('Ошибка экспорта'))}
            >
              <FileDown className="h-3.5 w-3.5" />
              Экспорт CSV
            </Button>
            <Button
              size="sm" className="gap-1.5"
              disabled={importMutation.isPending || Boolean(activeImportId) || !shopFilter}
              onClick={() => importMutation.mutate()}
            >
              <Download className="h-3.5 w-3.5" />
              {activeImportId ? 'Импорт...' : 'Импортировать'}
            </Button>
          </div>
        }
      />

      {/* ── Import status bar ── */}
      {visibleImportId && importStatus && (
        <div className="mb-4 bg-[#fffbe6] border border-[#ffcc00] rounded-xl px-4 py-3 flex items-center justify-between gap-4 text-sm">
          <div className="flex items-center gap-3">
            {isImportActive && (
              <div className="w-4 h-4 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
            )}
            <span className="font-medium">
              Импорт: {importStatus.status}
            </span>
            {importStatus.total > 0 && (
              <span className="text-[#666] text-xs">
                +{importStatus.added} добавлено · ~{importStatus.updated} обновлено · {importStatus.skipped} пропущено · {importStatus.failed} ошибок
              </span>
            )}
          </div>
          <div className="flex gap-2 shrink-0">
            {isImportActive && (
              <Button
                variant="secondary" size="sm"
                onClick={() => activeImportId && cancelImportMutation.mutate(activeImportId)}
                disabled={cancelImportMutation.isPending}
              >
                Отменить
              </Button>
            )}
            {isImportTerminal && importStatus.failed > 0 && (
              <Button
                variant="secondary" size="sm"
                onClick={() => setErrorImportId(visibleImportId)}
              >
                Детали ошибок
              </Button>
            )}
            <button onClick={() => { setActiveImportId(null); setLastImportId(null); setLastImportStatus(null) }}>
              <X className="h-4 w-4 text-[#aaa] hover:text-[#666]" />
            </button>
          </div>
        </div>
      )}

      <div className="bg-white rounded-2xl border border-[#e6e6e6] overflow-hidden">
        {/* ── Filters ── */}
        <div className="p-4 border-b border-[#e6e6e6] flex flex-wrap gap-3 items-end">
          {/* search */}
          <div className="relative flex-1 min-w-[180px]">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[#aaa]" />
            <Input
              className="pl-9"
              placeholder="Поиск по названию или SKU..."
              value={searchInput}
              onChange={e => handleSearchChange(e.target.value)}
            />
          </div>

          {/* shop filter */}
          <Select value={shopFilter || allShopsValue} onValueChange={v => { setShopFilter(v === allShopsValue ? '' : v); setPage(1); setSelectedIds(new Set()) }}>
            <SelectTrigger className="w-40">
              <SelectValue placeholder="Все магазины" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={allShopsValue}>Все магазины</SelectItem>
              {shops.map(s => <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>)}
            </SelectContent>
          </Select>

          {/* status filter */}
          <Select value={statusFilter || allStatusesValue} onValueChange={v => { setStatusFilter(v === allStatusesValue ? '' : v); setPage(1); setSelectedIds(new Set()) }}>
            <SelectTrigger className="w-36">
              <SelectValue placeholder="Все статусы" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={allStatusesValue}>Все статусы</SelectItem>
              <SelectItem value="active">Активен</SelectItem>
              <SelectItem value="archived">Архив</SelectItem>
              <SelectItem value="out_of_stock">Нет в наличии</SelectItem>
            </SelectContent>
          </Select>

          {/* price range */}
          <div className="flex items-center gap-1">
            <Input className="w-24" type="number" min={0} placeholder="Цена от"
              value={priceFromStr} onChange={e => { setPriceFromStr(e.target.value); setPage(1) }} />
            <span className="text-[#aaa] text-xs">—</span>
            <Input className="w-24" type="number" min={0} placeholder="до"
              value={priceToStr} onChange={e => { setPriceToStr(e.target.value); setPage(1) }} />
          </div>
        </div>

        {/* ── Bulk actions bar ── */}
        {selectedIds.size > 0 && (
          <div className="px-4 py-2 bg-[#fffbe6] border-b border-[#ffcc00] flex items-center gap-3 text-sm">
            <span className="text-[#666]">Выбрано: <strong>{selectedIds.size}</strong></span>
            <Button
              variant="secondary" size="sm"
              onClick={() => bulkArchiveMutation.mutate()}
              disabled={bulkArchiveMutation.isPending}
            >
              Архивировать выбранные
            </Button>
            <Button
              variant="secondary" size="sm"
              onClick={() => setAssignOpen(true)}
            >
              Назначить стратегию
            </Button>
            <button className="text-xs text-[#999] hover:text-[#333] ml-auto"
              onClick={() => setSelectedIds(new Set())}>
              Снять выделение
            </button>
          </div>
        )}

        {/* ── Table ── */}
        {isLoading && products.length === 0 ? (
          <div className="p-4 space-y-2">
            {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-12" />)}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[#f5f5f5] text-[#666] text-xs">
                  <th className="px-4 py-3 w-8">
                    <input type="checkbox" checked={allPageSelected}
                      onChange={toggleAll}
                      className="accent-[#ffcc00] cursor-pointer" />
                  </th>
                  <th className="text-left px-4 py-3 font-medium">
                    <button className="flex items-center hover:text-[#111]" onClick={() => handleSort('name')}>
                      Название <SortIcon field="name" sortBy={sortBy} sortDir={sortDir} />
                    </button>
                  </th>
                  <th className="text-left px-4 py-3 font-medium">SKU</th>
                  <th className="text-right px-4 py-3 font-medium">
                    <button className="flex items-center ml-auto hover:text-[#111]" onClick={() => handleSort('current_price')}>
                      Цена <SortIcon field="current_price" sortBy={sortBy} sortDir={sortDir} />
                    </button>
                  </th>
                  <th className="text-right px-4 py-3 font-medium">Мин / Макс / Себест.</th>
                  <th className="text-left px-4 py-3 font-medium">Статус</th>
                  <th className="text-left px-4 py-3 font-medium">
                    <button className="flex items-center hover:text-[#111]" onClick={() => handleSort('updated_at')}>
                      Обновлено <SortIcon field="updated_at" sortBy={sortBy} sortDir={sortDir} />
                    </button>
                  </th>
                  <th className="w-8" />
                </tr>
              </thead>
              <tbody>
                {products.map(p => (
                  <tr key={p.id}
                    className={`border-b border-[#f9f9f9] hover:bg-[#fafafa] transition-colors ${selectedIds.has(p.id) ? 'bg-[#fffde8]' : ''}`}
                  >
                    <td className="px-4 py-3">
                      <input type="checkbox" checked={selectedIds.has(p.id)}
                        onChange={() => toggleOne(p.id)}
                        className="accent-[#ffcc00] cursor-pointer" />
                    </td>
                    <td className="px-4 py-3 font-medium text-[#111] max-w-[200px]">
                      <p className="truncate">{p.name}</p>
                    </td>
                    <td className="px-4 py-3 text-[#666] font-mono text-xs">{p.external_sku}</td>
                    <td className="px-4 py-3 text-right font-semibold text-[#111]">
                      {formatPrice(p.current_price)}
                    </td>
                    <td className="px-4 py-3 text-right text-[#666] text-xs">
                      {p.min_price != null ? formatPrice(p.min_price) : '—'}
                      {' / '}
                      {p.max_price != null ? formatPrice(p.max_price) : '—'}
                      {' / '}
                      {p.cost_price != null ? formatPrice(p.cost_price) : '—'}
                    </td>
                    <td className="px-4 py-3"><StatusBadge status={p.status} /></td>
                    <td className="px-4 py-3 text-[#aaa] text-xs">{formatDate(p.updated_at)}</td>
                    <td className="px-4 py-3">
                      <button
                        className="p-1 rounded hover:bg-[#f0f0f0] text-[#aaa] hover:text-[#333]"
                        onClick={() => setEditProduct(p)}
                        title="Редактировать"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
                {products.length === 0 && (
                  <tr>
                    <td colSpan={8} className="px-4 py-12 text-center text-[#aaa] text-sm">
                      Товары не найдены
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}

        {/* ── Pagination ── */}
        <div className="px-4 py-3 border-t border-[#f5f5f5] flex items-center justify-between text-xs text-[#666]">
          <span>Всего: {pagination.total}</span>
          {totalPages > 1 && (
            <div className="flex items-center gap-1">
              <button
                className="px-2 py-1 rounded border border-[#e6e6e6] disabled:opacity-40 hover:bg-[#fafafa]"
                disabled={page <= 1}
                onClick={() => setPage(p => p - 1)}
              >← Пред</button>
              {buildPageRange(page, totalPages).map((p, i) =>
                p === '…'
                  ? <span key={`e${i}`} className="px-2">…</span>
                  : (
                    <button key={p}
                      className={`px-2 py-1 rounded border ${page === p ? 'border-[#ffcc00] bg-[#fffbe6] font-semibold' : 'border-[#e6e6e6] hover:bg-[#fafafa]'}`}
                      onClick={() => setPage(p as number)}
                    >{p}</button>
                  )
              )}
              <button
                className="px-2 py-1 rounded border border-[#e6e6e6] disabled:opacity-40 hover:bg-[#fafafa]"
                disabled={page >= totalPages}
                onClick={() => setPage(p => p + 1)}
              >След →</button>
            </div>
          )}
        </div>
      </div>

      {/* ── Modals ── */}
      {editProduct && (
        <EditProductModal
          product={editProduct}
          onClose={() => setEditProduct(null)}
          onSaved={() => queryClient.invalidateQueries({ queryKey: ['products'] })}
        />
      )}
      {errorImportId && (
        <ImportErrorsDialog importId={errorImportId} onClose={() => setErrorImportId(null)} />
      )}

      {/* Assign strategy dialog */}
      <Dialog open={assignOpen} onOpenChange={v => { if (!v) { setAssignOpen(false); setAssignStrategyId('') } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Назначить стратегию</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-[#666]">Выбрано товаров: <strong>{selectedIds.size}</strong></p>
          <div>
            <Label>Стратегия</Label>
            <Select value={assignStrategyId} onValueChange={setAssignStrategyId}>
              <SelectTrigger className="mt-1.5">
                <SelectValue placeholder="Выберите стратегию" />
              </SelectTrigger>
              <SelectContent>
                {allStrategies.map((s: Strategy) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name} {!s.enabled && '(отключена)'}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button variant="secondary" onClick={() => { setAssignOpen(false); setAssignStrategyId('') }}>Отмена</Button>
            <Button disabled={!assignStrategyId || assignMutation.isPending} onClick={() => assignMutation.mutate()}>
              {assignMutation.isPending ? 'Назначаем...' : 'Назначить'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </AppLayout>
  )
}

// ─── helpers ─────────────────────────────────────────────────────────────────

function buildPageRange(current: number, total: number): (number | '…')[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | '…')[] = [1]
  if (current > 3) pages.push('…')
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push('…')
  pages.push(total)
  return pages
}
