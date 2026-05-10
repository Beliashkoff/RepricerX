import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ExternalLink, Link2, Unlink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { notificationsApi } from '@/api/notifications'

export function TelegramLinkDialog() {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [startURL, setStartURL] = useState('')

  const { data: status } = useQuery({
    queryKey: ['notifications', 'telegram-status'],
    queryFn: () => notificationsApi.telegramStatus(),
    refetchInterval: open && !startURL ? 3000 : false,
  })

  const issue = useMutation({
    mutationFn: () => notificationsApi.issueTelegramToken(),
    onSuccess: (res) => {
      setStartURL(res.start_url)
      setOpen(true)
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Telegram недоступен'),
  })

  const unlink = useMutation({
    mutationFn: () => notificationsApi.unlinkTelegram(),
    onSuccess: () => {
      toast.success('Telegram отвязан')
      qc.invalidateQueries({ queryKey: ['notifications', 'telegram-status'] })
      qc.invalidateQueries({ queryKey: ['notifications', 'preferences'] })
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось отвязать Telegram'),
  })

  useEffect(() => {
    if (!open || !startURL) return
    const t = window.setInterval(async () => {
      const next = await notificationsApi.telegramStatus().catch(() => null)
      if (next?.linked) {
        toast.success('Telegram привязан')
        setOpen(false)
        setStartURL('')
        qc.invalidateQueries({ queryKey: ['notifications', 'telegram-status'] })
        qc.invalidateQueries({ queryKey: ['notifications', 'preferences'] })
      }
    }, 3000)
    return () => window.clearInterval(t)
  }, [open, qc, startURL])

  return (
    <div className="rounded-xl border border-[#e6e6e6] p-4">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h4 className="text-sm font-semibold text-[#111]">Telegram</h4>
          <p className="text-xs text-[#666] mt-1">
            {status?.linked
              ? `Привязан${status.username ? `: @${status.username}` : ''}`
              : 'Быстрые уведомления в личный чат с ботом.'}
          </p>
        </div>
        {status?.linked ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => unlink.mutate()}
            disabled={unlink.isPending}
          >
            <Unlink className="h-3.5 w-3.5" />
            Отвязать
          </Button>
        ) : (
          <Button
            type="button"
            variant="secondary"
            size="sm"
            className="gap-1.5"
            onClick={() => issue.mutate()}
            disabled={issue.isPending}
          >
            <Link2 className="h-3.5 w-3.5" />
            Привязать
          </Button>
        )}
      </div>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Привязка Telegram</DialogTitle>
            <DialogDescription>
              Откройте ссылку и отправьте команду боту. Статус обновится автоматически.
            </DialogDescription>
          </DialogHeader>
          <div className="rounded-xl bg-[#f7f8fa] border border-[#e6e6e6] p-3 text-xs text-[#666] break-all">
            {startURL}
          </div>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>
              Закрыть
            </Button>
            <Button type="button" asChild>
              <a href={startURL} target="_blank" rel="noreferrer">
                <ExternalLink className="h-4 w-4" />
                Открыть Telegram
              </a>
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
