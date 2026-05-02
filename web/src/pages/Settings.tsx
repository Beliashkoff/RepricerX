import { useState } from 'react'
import { toast } from 'sonner'
import { AppLayout, PageHeader } from '@/components/layout/AppLayout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/api/auth'

export default function Settings() {
  const { user, refreshMe } = useAuth()
  const [displayName, setDisplayName] = useState(user?.displayName ?? '')
  const [saving, setSaving] = useState(false)

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
      <PageHeader title="Настройки" description="Управление профилем и безопасностью" />
      <div className="max-w-lg">
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
      </div>
    </AppLayout>
  )
}
