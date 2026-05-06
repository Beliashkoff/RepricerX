import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AppLayout, PageHeader, EmptyState } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { shopsApi } from '@/api/shops'
import type { Shop, Marketplace } from '@/types/api'
import { Plus, Trash2, RefreshCw, Clock, Eye, EyeOff } from 'lucide-react'
import { formatDate } from '@/lib/utils'

function ShopStatusBadge({ status }: { status: Shop['status'] }) {
  const config = {
    active: { label: 'Активен', variant: 'success' as const },
    error: { label: 'Ошибка', variant: 'destructive' as const },
    pending: { label: 'Проверка', variant: 'warning' as const },
    disabled: { label: 'Отключён', variant: 'secondary' as const },
  }
  const c = config[status] ?? { label: status, variant: 'secondary' as const }
  return <Badge variant={c.variant}>{c.label}</Badge>
}

function MarketplaceLabel({ mp }: { mp: Marketplace }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-[#555]">
      <span className="w-2 h-2 rounded-full bg-[#ffcc00]" />
      {mp === 'wb' ? 'Wildberries' : mp === 'ozon' ? 'Ozon' : mp}
    </span>
  )
}

interface CreateShopForm {
  name: string
  marketplace: Marketplace
  api_key: string
  client_id: string
}

function CreateShopDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState<CreateShopForm>({ name: '', marketplace: 'wb', api_key: '', client_id: '' })
  const [showKey, setShowKey] = useState(false)

  const { mutate, isPending } = useMutation({
    mutationFn: () => shopsApi.create({
      name: form.name,
      marketplace: form.marketplace,
      credentials: form.marketplace === 'wb'
        ? { api_key: form.api_key }
        : { client_id: form.client_id, api_key: form.api_key },
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['shops'] })
      toast.success('Магазин подключён')
      onClose()
      setForm({ name: '', marketplace: 'wb', api_key: '', client_id: '' })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <Dialog open={open} onOpenChange={v => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Подключить магазин</DialogTitle>
          <DialogDescription>Введите данные для подключения к маркетплейсу</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div>
            <Label htmlFor="sh-name">Название магазина</Label>
            <Input id="sh-name" className="mt-1.5" placeholder="Мой магазин WB" value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} />
          </div>
          <div>
            <Label>Маркетплейс</Label>
            <Select value={form.marketplace} onValueChange={v => setForm(f => ({ ...f, marketplace: v as Marketplace }))}>
              <SelectTrigger className="mt-1.5">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="wb">Wildberries</SelectItem>
                <SelectItem value="ozon">Ozon</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {form.marketplace === 'ozon' && (
            <div>
              <Label htmlFor="sh-clientid">Client ID</Label>
              <Input id="sh-clientid" className="mt-1.5" placeholder="123456" value={form.client_id} onChange={e => setForm(f => ({ ...f, client_id: e.target.value }))} />
            </div>
          )}
          <div>
            <Label htmlFor="sh-key">{form.marketplace === 'wb' ? 'API-ключ' : 'API Key'}</Label>
            <div className="relative mt-1.5">
              <Input
                id="sh-key"
                type={showKey ? 'text' : 'password'}
                placeholder={form.marketplace === 'wb' ? 'eyJhbGciOi...' : 'ваш API Key'}
                value={form.api_key}
                onChange={e => setForm(f => ({ ...f, api_key: e.target.value }))}
                className="pr-10"
              />
              <button type="button" onClick={() => setShowKey(v => !v)} className="absolute right-3 top-1/2 -translate-y-1/2 text-[#aaa] hover:text-[#666]">
                {showKey ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
            <p className="text-xs text-[#aaa] mt-1">Ключ хранится в зашифрованном виде</p>
          </div>
          <div className="flex gap-3 pt-2">
            <Button variant="secondary" className="flex-1" onClick={onClose}>Отмена</Button>
            <Button
              className="flex-1"
              disabled={!form.name || !form.api_key || (form.marketplace === 'ozon' && !form.client_id) || isPending}
              onClick={() => mutate()}
            >
              {isPending ? 'Подключаем...' : 'Подключить'}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

export default function Shops() {
  const [createOpen, setCreateOpen] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const qc = useQueryClient()

  const { data: shops = [], isLoading } = useQuery({ queryKey: ['shops'], queryFn: shopsApi.list })

  const testMutation = useMutation({
    mutationFn: (id: string) => shopsApi.testConnection(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['shops'] })
      toast.success(data.message || 'Подключение успешно')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => shopsApi.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['shops'] })
      toast.success('Магазин удалён')
      setDeletingId(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <AppLayout>
      <PageHeader
        title="Магазины"
        description="Управляйте подключёнными маркетплейсами"
        action={<Button onClick={() => setCreateOpen(true)} className="gap-2"><Plus className="h-4 w-4" />Подключить магазин</Button>}
      />

      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <div className="w-6 h-6 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
        </div>
      ) : shops.length === 0 ? (
        <EmptyState
          title="Нет подключённых магазинов"
          description="Добавьте первый магазин, чтобы начать управлять ценами"
          action={<Button onClick={() => setCreateOpen(true)} className="gap-2"><Plus className="h-4 w-4" />Подключить магазин</Button>}
        />
      ) : (
        <div className="grid sm:grid-cols-2 xl:grid-cols-3 gap-5">
          {shops.map(shop => (
            <div key={shop.id} className="bg-white rounded-2xl border border-[#e6e6e6] p-5 flex flex-col gap-4">
              <div className="flex items-start justify-between">
                <div>
                  <p className="font-semibold text-[#111]">{shop.name}</p>
                  <MarketplaceLabel mp={shop.marketplace} />
                </div>
                <ShopStatusBadge status={shop.status} />
              </div>
              {shop.lastCheckedAt && (
                <p className="text-xs text-[#aaa] flex items-center gap-1.5">
                  <Clock className="h-3 w-3" />
                  Проверено: {formatDate(shop.lastCheckedAt)}
                </p>
              )}
              <div className="flex gap-2 pt-1">
                <Button
                  variant="secondary"
                  size="sm"
                  className="flex-1 gap-1.5"
                  disabled={testMutation.isPending}
                  onClick={() => testMutation.mutate(shop.id)}
                >
                  <RefreshCw className="h-3.5 w-3.5" />
                  Проверить
                </Button>
                <Button
                  variant="destructive"
                  size="icon"
                  onClick={() => setDeletingId(shop.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <CreateShopDialog open={createOpen} onClose={() => setCreateOpen(false)} />

      {/* Delete confirmation */}
      <Dialog open={!!deletingId} onOpenChange={v => !v && setDeletingId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Удалить магазин?</DialogTitle>
            <DialogDescription>
              Это действие нельзя отменить. Все настройки магазина будут удалены.
            </DialogDescription>
          </DialogHeader>
          <div className="flex gap-3 pt-2">
            <Button variant="secondary" className="flex-1" onClick={() => setDeletingId(null)}>Отмена</Button>
            <Button
              variant="destructive"
              className="flex-1"
              disabled={deleteMutation.isPending}
              onClick={() => deletingId && deleteMutation.mutate(deletingId)}
            >
              {deleteMutation.isPending ? 'Удаляем...' : 'Удалить'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </AppLayout>
  )
}
