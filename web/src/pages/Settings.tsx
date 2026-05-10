import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/api/auth'
import { notificationsApi, type Severity } from '@/api/notifications'
import { PreferencesMatrix } from '@/components/notifications/PreferencesMatrix'
import { TelegramLinkDialog } from '@/components/notifications/TelegramLinkDialog'
import { WebhooksManager } from '@/components/notifications/WebhooksManager'

const DIGEST_LABEL: Record<string, string> = {
  '0': 'Сразу',
  '15': 'Раз в 15 минут',
  '60': 'Раз в час',
  '240': 'Раз в 4 часа',
  '1440': 'Раз в сутки',
}

export default function Settings() {
  const { user, refreshMe } = useAuth()
  const qc = useQueryClient()
  const [displayName, setDisplayName] = useState(user?.displayName ?? '')
  const [saving, setSaving] = useState(false)
  const [quietEnabled, setQuietEnabled] = useState(false)

  const { data: telegram } = useQuery({
    queryKey: ['notifications', 'telegram-status'],
    queryFn: () => notificationsApi.telegramStatus(),
  })
  const { data: webhooks } = useQuery({
    queryKey: ['notifications', 'webhooks'],
    queryFn: () => notificationsApi.listWebhooks(),
  })
  const { data: channelSettings } = useQuery({
    queryKey: ['notifications', 'channel-settings'],
    queryFn: () => notificationsApi.listChannelSettings(),
  })

  const emailSettings = channelSettings?.items.find((s) => s.channel === 'email')
  const [digestWindow, setDigestWindow] = useState<string>('60')
  const [minSeverity, setMinSeverity] = useState<Severity>('info')
  const [quietStart, setQuietStart] = useState('22')
  const [quietEnd, setQuietEnd] = useState('8')

  useEffect(() => {
    if (!emailSettings) return
    setDigestWindow(String(emailSettings.digest_window_minutes))
    setMinSeverity(emailSettings.digest_min_severity)
    setQuietEnabled(emailSettings.quiet_hours_start != null && emailSettings.quiet_hours_end != null)
    if (emailSettings.quiet_hours_start != null) setQuietStart(String(emailSettings.quiet_hours_start))
    if (emailSettings.quiet_hours_end != null) setQuietEnd(String(emailSettings.quiet_hours_end))
  }, [emailSettings])

  const saveEmailSchedule = useMutation({
    mutationFn: () => notificationsApi.updateChannelSettings('email', {
      digest_window_minutes: Number(digestWindow),
      digest_min_severity: minSeverity,
      quiet_hours_start: quietEnabled ? Number(quietStart) : undefined,
      quiet_hours_end: quietEnabled ? Number(quietEnd) : undefined,
      clear_quiet_hours: !quietEnabled,
    }),
    onSuccess: () => {
      toast.success('Расписание email сохранено')
      qc.invalidateQueries({ queryKey: ['notifications', 'channel-settings'] })
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Не удалось сохранить расписание'),
  })

  async function handleSave(ev: React.FormEvent) {
    ev.preventDefault()
    if (!displayName.trim()) { toast.error('Имя не может быть пустым'); return }
    setSaving(true)
    try {
      await authApi.updateMe(displayName.trim())
      await refreshMe()
      toast.success('Профиль обновлён')
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Ошибка сохранения')
    } finally {
      setSaving(false)
    }
  }

  return (
    <AppLayout>
      <PageHeader title="Настройки" description="Управление профилем, безопасностью и уведомлениями" />
      <div className="max-w-5xl space-y-6">
        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-6">
          <h3 className="text-base font-semibold text-[#111] mb-4">Профиль</h3>
          <form onSubmit={handleSave} className="flex flex-col gap-4">
            <div>
              <Label htmlFor="settings-email">Email</Label>
              <Input
                id="settings-email"
                type="email"
                value={user?.email ?? ''}
                readOnly
                className="mt-1.5 bg-[#f7f8fa] text-[#666] cursor-default"
              />
              <p className="text-xs text-[#aaa] mt-1">Email изменить нельзя</p>
            </div>
            <div>
              <Label htmlFor="settings-name">Отображаемое имя</Label>
              <Input
                id="settings-name"
                type="text"
                className="mt-1.5"
                placeholder="Иван Иванов"
                value={displayName}
                onChange={e => setDisplayName(e.target.value)}
              />
            </div>
            <div className="pt-1">
              <Button type="submit" disabled={saving}>
                {saving ? 'Сохраняем...' : 'Сохранить'}
              </Button>
            </div>
          </form>
        </div>

        <div id="notifications" className="bg-white rounded-2xl border border-[#e6e6e6] p-6">
          <h3 className="text-base font-semibold text-[#111]">Уведомления</h3>
          <p className="text-sm text-[#666] mt-1 mb-5">Каналы доставки и события, которые должны доходить до пользователя.</p>

          <div className="space-y-4 mb-6">
            <TelegramLinkDialog />
            <WebhooksManager />
          </div>

          <PreferencesMatrix
            telegramLinked={!!telegram?.linked}
            webhookEnabled={(webhooks?.items.length ?? 0) > 0}
          />
        </div>

        <div className="bg-white rounded-2xl border border-[#e6e6e6] p-6">
          <h3 className="text-base font-semibold text-[#111]">Расписание email</h3>
          <p className="text-sm text-[#666] mt-1 mb-5">Дайджест группирует частые события и не влияет на in-app уведомления.</p>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
            <div>
              <Label className="text-xs text-[#666] mb-1.5 block">Частота</Label>
              <Select value={digestWindow} onValueChange={setDigestWindow}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {Object.entries(DIGEST_LABEL).map(([value, label]) => (
                    <SelectItem key={value} value={value}>{label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label className="text-xs text-[#666] mb-1.5 block">Минимальная важность</Label>
              <Select value={minSeverity} onValueChange={(v) => setMinSeverity(v as Severity)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="info">Информация</SelectItem>
                  <SelectItem value="warning">Внимание</SelectItem>
                  <SelectItem value="error">Ошибка</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <label className="flex items-center gap-2 text-sm text-[#111] pt-7">
              <input
                type="checkbox"
                checked={quietEnabled}
                onChange={(e) => setQuietEnabled(e.target.checked)}
                className="h-4 w-4 accent-[#ffcc00]"
              />
              Не беспокоить ночью
            </label>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <Label className="text-xs text-[#666] mb-1.5 block">С UTC</Label>
                <Input type="number" min={0} max={23} value={quietStart} disabled={!quietEnabled} onChange={(e) => setQuietStart(e.target.value)} />
              </div>
              <div>
                <Label className="text-xs text-[#666] mb-1.5 block">До UTC</Label>
                <Input type="number" min={0} max={23} value={quietEnd} disabled={!quietEnabled} onChange={(e) => setQuietEnd(e.target.value)} />
              </div>
            </div>
          </div>
          <div className="mt-5">
            <Button type="button" onClick={() => saveEmailSchedule.mutate()} disabled={saveEmailSchedule.isPending}>
              {saveEmailSchedule.isPending ? 'Сохраняем...' : 'Сохранить расписание'}
            </Button>
          </div>
        </div>
      </div>
    </AppLayout>
  )
}
