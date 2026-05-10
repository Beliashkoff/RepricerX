import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Send, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { notificationsApi } from '@/api/notifications'

export function WebhooksManager() {
  const qc = useQueryClient()
  const [url, setURL] = useState('')
  const [description, setDescription] = useState('')
  const [latestSecret, setLatestSecret] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['notifications', 'webhooks'],
    queryFn: () => notificationsApi.listWebhooks(),
  })
  const webhooks = data?.items ?? []

  const create = useMutation({
    mutationFn: () => notificationsApi.createWebhook(url.trim(), description.trim()),
    onSuccess: (w) => {
      setURL('')
      setDescription('')
      setLatestSecret(w.secret ?? '')
      toast.success('Webhook добавлен')
      qc.invalidateQueries({ queryKey: ['notifications', 'webhooks'] })
      qc.invalidateQueries({ queryKey: ['notifications', 'preferences'] })
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось добавить webhook'),
  })

  const remove = useMutation({
    mutationFn: (id: string) => notificationsApi.deleteWebhook(id),
    onSuccess: () => {
      toast.success('Webhook удалён')
      qc.invalidateQueries({ queryKey: ['notifications', 'webhooks'] })
      qc.invalidateQueries({ queryKey: ['notifications', 'preferences'] })
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось удалить webhook'),
  })

  const test = useMutation({
    mutationFn: (id: string) => notificationsApi.testWebhook(id),
    onSuccess: (res) => {
      if (res.error) toast.error(res.error)
      else toast.success(`Webhook ответил HTTP ${res.http_status}`)
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Тест webhook не прошёл'),
  })

  return (
    <div className="rounded-xl border border-[#e6e6e6] p-4">
      <h4 className="text-sm font-semibold text-[#111]">Webhooks</h4>
      <p className="text-xs text-[#666] mt-1">Outbound POST для внешних автоматизаций.</p>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_180px_auto] gap-3 mt-4 items-end">
        <div>
          <Label className="text-xs">URL</Label>
          <Input value={url} onChange={(e) => setURL(e.target.value)} placeholder="https://example.com/repricerx" />
        </div>
        <div>
          <Label className="text-xs">Описание</Label>
          <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="CRM" />
        </div>
        <Button
          type="button"
          onClick={() => create.mutate()}
          disabled={create.isPending || !url.trim()}
        >
          Добавить
        </Button>
      </div>

      {latestSecret && (
        <div className="mt-3 rounded-xl bg-yellow-50 border border-yellow-200 p-3 text-xs text-yellow-900 break-all">
          Секрет показан один раз: {latestSecret}
        </div>
      )}

      <div className="mt-4 divide-y divide-[#f0f0f0]">
        {isLoading && <div className="py-3 text-sm text-[#666]">Загрузка...</div>}
        {!isLoading && webhooks.length === 0 && (
          <div className="py-3 text-sm text-[#666]">Webhook-и не добавлены</div>
        )}
        {webhooks.map((w) => (
          <div key={w.id} className="py-3 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="text-sm font-medium text-[#111] truncate">{w.description || 'Webhook'}</div>
              <div className="text-xs text-[#666] truncate">{w.url}</div>
            </div>
            <div className="flex items-center gap-2 shrink-0">
              <Button type="button" variant="outline" size="sm" onClick={() => test.mutate(w.id)}>
                <Send className="h-3.5 w-3.5" />
                Тест
              </Button>
              <Button type="button" variant="destructive" size="sm" onClick={() => remove.mutate(w.id)}>
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
