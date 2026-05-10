import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  ALL_CHANNELS,
  ALL_EVENTS,
  CHANNEL_LABEL,
  EVENT_LABEL,
  notificationsApi,
  type Channel,
  type PreferenceItem,
} from '@/api/notifications'

function keyOf(eventType: string, channel: Channel) {
  return `${eventType}:${channel}`
}

export function PreferencesMatrix({
  telegramLinked,
  webhookEnabled,
}: {
  telegramLinked: boolean
  webhookEnabled: boolean
}) {
  const qc = useQueryClient()
  const [items, setItems] = useState<Record<string, boolean>>({})

  const { data, isLoading } = useQuery({
    queryKey: ['notifications', 'preferences'],
    queryFn: () => notificationsApi.getPreferences(),
  })

  useEffect(() => {
    if (!data) return
    const next: Record<string, boolean> = {}
    for (const item of data.items) {
      next[keyOf(item.event_type, item.channel)] = item.enabled
    }
    setItems(next)
  }, [data])

  const visibleChannels = useMemo(() => {
    return ALL_CHANNELS.filter((ch) => {
      if (ch === 'telegram') return telegramLinked
      if (ch === 'webhook') return webhookEnabled
      return true
    })
  }, [telegramLinked, webhookEnabled])

  const save = useMutation({
    mutationFn: () => {
      const payload: PreferenceItem[] = []
      for (const eventType of ALL_EVENTS) {
        for (const channel of visibleChannels) {
          payload.push({
            event_type: eventType,
            channel,
            enabled: channel === 'in_app' ? true : !!items[keyOf(eventType, channel)],
          })
        }
      }
      return notificationsApi.updatePreferences(payload)
    },
    onSuccess: () => {
      toast.success('Настройки уведомлений сохранены')
      qc.invalidateQueries({ queryKey: ['notifications', 'preferences'] })
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось сохранить настройки'),
  })

  function checked(eventType: string, channel: Channel) {
    if (channel === 'in_app') return true
    return !!items[keyOf(eventType, channel)]
  }

  function toggle(eventType: string, channel: Channel) {
    if (channel === 'in_app') return
    setItems((prev) => {
      const k = keyOf(eventType, channel)
      return { ...prev, [k]: !prev[k] }
    })
  }

  if (isLoading) {
    return <div className="text-sm text-[#666] py-4">Загрузка настроек...</div>
  }

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto border border-[#e6e6e6] rounded-xl">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[#e6e6e6] bg-[#f7f8fa] text-xs text-[#666]">
              <th className="text-left px-4 py-3 font-medium min-w-[240px]">Событие</th>
              {visibleChannels.map((ch) => (
                <th key={ch} className="text-center px-3 py-3 font-medium whitespace-nowrap">
                  {CHANNEL_LABEL[ch]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {ALL_EVENTS.map((eventType) => (
              <tr key={eventType} className="border-b border-[#f5f5f5] last:border-0">
                <td className="px-4 py-3 text-[#111]">{EVENT_LABEL[eventType] ?? eventType}</td>
                {visibleChannels.map((ch) => (
                  <td key={ch} className="px-3 py-3 text-center">
                    <input
                      type="checkbox"
                      checked={checked(eventType, ch)}
                      disabled={ch === 'in_app'}
                      onChange={() => toggle(eventType, ch)}
                      className="h-4 w-4 accent-[#ffcc00]"
                      aria-label={`${EVENT_LABEL[eventType] ?? eventType}: ${CHANNEL_LABEL[ch]}`}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Button type="button" onClick={() => save.mutate()} disabled={save.isPending}>
        {save.isPending ? 'Сохраняем...' : 'Сохранить уведомления'}
      </Button>
    </div>
  )
}
