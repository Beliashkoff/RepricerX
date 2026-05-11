import { useState, useEffect } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { Eye, EyeOff, Link2 } from 'lucide-react'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

function providerLabel(p: string): string {
  switch (p) {
    case 'vk':
      return 'VK ID'
    case 'yandex':
      return 'Яндекс ID'
    default:
      return 'OAuth-провайдер'
  }
}

export default function LinkOAuth() {
  const { user, isLoading, refreshMe } = useAuth()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const linkToken = searchParams.get('token') ?? ''
  const email = searchParams.get('email') ?? ''
  const provider = searchParams.get('provider') ?? ''

  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!linkToken) {
      toast.error('Ссылка для привязки недействительна')
      navigate('/login', { replace: true })
    }
  }, [linkToken, navigate])

  if (!isLoading && user) return <Navigate to="/dashboard" replace />

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault()
    if (!password) {
      setError('Введите пароль')
      return
    }
    setError(null)
    setSubmitting(true)
    try {
      await authApi.linkOAuth(linkToken, password)
      await refreshMe()
      toast.success(`Аккаунт ${providerLabel(provider)} привязан`)
      navigate('/dashboard', { replace: true })
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Неверный пароль'
      setError(message)
      toast.error(message)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="min-h-screen bg-[#f7f8fa] flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <Link to="/" className="inline-flex items-center gap-2 font-bold text-xl text-[#111]">
            <img src="/logo.png" alt="RepricerX logo" className="w-9 h-9 rounded-lg object-contain pointer-events-none" draggable={false} />
            RepricerX
          </Link>
        </div>

        <div className="bg-white rounded-3xl border border-[#e6e6e6] p-8 shadow-sm">
          <div className="w-12 h-12 rounded-2xl bg-[#fff4d6] flex items-center justify-center mx-auto mb-4">
            <Link2 className="h-6 w-6 text-[#a16207]" />
          </div>
          <h1 className="text-xl font-bold text-[#111] text-center mb-2">
            Привязать {providerLabel(provider)}
          </h1>
          <p className="text-sm text-[#666] text-center mb-6">
            Адрес <strong className="text-[#111]">{email}</strong> уже зарегистрирован в RepricerX.
            Введите пароль от этого аккаунта, чтобы привязать к нему вход через {providerLabel(provider)}.
          </p>

          <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
            <div>
              <Label htmlFor="password">Пароль</Label>
              <div className="relative mt-1.5">
                <Input
                  id="password"
                  type={showPass ? 'text' : 'password'}
                  autoComplete="current-password"
                  placeholder="••••••••"
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  className="pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPass(v => !v)}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-[#aaa] hover:text-[#666]"
                >
                  {showPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              {error && <p className="text-xs text-red-500 mt-1">{error}</p>}
            </div>

            <Button type="submit" className="w-full mt-1" disabled={submitting}>
              {submitting ? 'Привязываем...' : 'Подтвердить и войти'}
            </Button>
          </form>

          <p className="text-center text-sm text-[#666] mt-6">
            <Link to="/login" className="text-[#111] font-medium hover:underline">
              Отмена
            </Link>
          </p>
        </div>
      </div>
    </div>
  )
}
