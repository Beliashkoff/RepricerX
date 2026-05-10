import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query'
import { Bell, CheckCheck } from 'lucide-react'
import { notificationsApi, type NotificationResponse } from '@/api/notifications'
import { NotificationItem, notificationTarget } from '@/components/notifications/NotificationItem'

export function NotificationBell() {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const qc = useQueryClient()

  const { data: unreadData } = useQuery({
    queryKey: ['notifications', 'unread-count'],
    queryFn: () => notificationsApi.unreadCount(),
    refetchInterval: 30_000,
  })
  const unread = unreadData ?? 0

  const { data: list } = useQuery({
    queryKey: ['notifications', 'preview'],
    queryFn: () => notificationsApi.list({ perPage: 8 }),
    enabled: open,
  })

  const markRead = useMutation({
    mutationFn: (id: string) => notificationsApi.markRead(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['notifications'] })
    },
  })
  const markAll = useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['notifications'] })
    },
  })

  // Закрытие по клику вне.
  useEffect(() => {
    if (!open) return
    function handler(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  function handleItemClick(n: NotificationResponse) {
    if (!n.read_at) {
      markRead.mutate(n.id)
    }
    setOpen(false)
    navigate(notificationTarget(n))
  }

  return (
    <div className="relative" ref={containerRef}>
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label="Уведомления"
        className="relative w-9 h-9 rounded-xl bg-[#f5f5f5] flex items-center justify-center text-[#666] hover:bg-[#e8e9eb] transition-colors"
      >
        <Bell className="h-4 w-4" />
        {unread > 0 && (
          <span className="absolute -top-1 -right-1 min-w-[18px] h-[18px] px-1 rounded-full bg-red-600 text-white text-[10px] font-bold flex items-center justify-center">
            {unread > 99 ? '99+' : unread}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-12 w-96 max-h-[70vh] bg-white border border-[#e6e6e6] rounded-xl shadow-xl z-50 flex flex-col overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-[#e6e6e6]">
            <span className="text-sm font-semibold text-[#111]">Уведомления</span>
            {unread > 0 && (
              <button
                onClick={() => markAll.mutate()}
                className="flex items-center gap-1 text-xs text-[#2563eb] hover:underline"
              >
                <CheckCheck className="h-3 w-3" />
                Прочитать все
              </button>
            )}
          </div>

          <div className="flex-1 overflow-y-auto">
            {!list && <div className="p-4 text-sm text-[#666]">Загрузка…</div>}
            {list && list.items.length === 0 && (
              <div className="p-6 text-sm text-[#666] text-center">Пока нет уведомлений</div>
            )}
            {list?.items.map((n) => (
              <NotificationItem
                key={n.id}
                notification={n}
                compact
                onClick={() => handleItemClick(n)}
              />
            ))}
          </div>

          <div className="p-3 border-t border-[#e6e6e6]">
            <Link
              to="/notifications"
              onClick={() => setOpen(false)}
              className="block w-full text-center text-sm font-medium text-[#2563eb] hover:underline"
            >
              Все уведомления
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}
