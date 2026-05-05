import { useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Eye, EyeOff, Mail } from 'lucide-react'
import { useAuth } from '@/context/AuthContext'
import { authApi } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

function VKIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M15.684 0H8.316C1.592 0 0 1.592 0 8.316v7.368C0 22.408 1.592 24 8.316 24h7.368C22.408 24 24 22.408 24 15.684V8.316C24 1.592 22.408 0 15.684 0zm3.692 17.123h-1.744c-.66 0-.862-.523-2.049-1.715-1.033-1.01-1.49-1.135-1.744-1.135-.356 0-.458.102-.458.593v1.566c0 .424-.135.678-1.253.678-1.846 0-3.896-1.118-5.335-3.202C5.21 11.6 4.001 8.94 4.001 8.48c0-.254.102-.491.593-.491h1.744c.44 0 .61.203.78.677.863 2.49 2.303 4.675 2.896 4.675.22 0 .322-.102.322-.66V9.72c-.068-1.186-.695-1.287-.695-1.71 0-.204.17-.407.44-.407h2.743c.373 0 .508.203.508.643v3.473c0 .372.17.508.271.508.22 0 .407-.136.813-.542 1.254-1.406 2.151-3.574 2.151-3.574.119-.254.322-.491.762-.491h1.744c.525 0 .643.271.525.643-.22 1.017-2.354 4.031-2.354 4.031-.186.305-.254.44 0 .78.186.254.796.779 1.203 1.253.745.847 1.32 1.558 1.473 2.05.17.49-.085.745-.576.745z" />
    </svg>
  )
}

function YandexIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M2.04 12c0-5.523 4.476-10 9.998-10C17.522 2 22 6.477 22 12s-4.478 10-9.962 10C6.516 22 2.04 17.523 2.04 12zm11.05 4.885V13.15h.927l2.132 3.735h1.785l-2.34-4.016c1.23-.404 1.97-1.38 1.97-2.747 0-1.88-1.196-2.972-3.307-2.972h-2.742v9.735h1.575zm0-8.342h1.028c1.196 0 1.858.562 1.858 1.591 0 1.057-.662 1.647-1.858 1.647h-1.028V8.543z" />
    </svg>
  )
}

function SocialButton({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center justify-center gap-2.5 w-full px-4 py-2.5 rounded-xl border border-[#e6e6e6] bg-white text-sm font-medium text-[#333] hover:bg-[#f5f5f5] transition-colors"
    >
      {icon}
      {label}
    </button>
  )
}

export default function Register() {
  const { user, isLoading } = useAuth()
  const [displayName, setDisplayName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [errors, setErrors] = useState<{ displayName?: string; email?: string; password?: string }>({})
  const [registeredEmail, setRegisteredEmail] = useState<string | null>(null)
  const [resending, setResending] = useState(false)
  const [recentlySent, setRecentlySent] = useState(false)

  if (!isLoading && user) return <Navigate to="/dashboard" replace />

  function validate() {
    const e: typeof errors = {}
    if (!displayName.trim()) e.displayName = 'Введите имя'
    if (!email) e.email = 'Введите email'
    else if (!/\S+@\S+\.\S+/.test(email)) e.email = 'Некорректный email'
    if (!password) e.password = 'Введите пароль'
    else if (password.length < 8) e.password = 'Минимум 8 символов'
    return e
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault()
    const e = validate()
    setErrors(e)
    if (Object.keys(e).length) return

    setSubmitting(true)
    try {
      await authApi.register(email, password, displayName.trim())
      setRegisteredEmail(email)
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Ошибка регистрации')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleResend() {
    if (!registeredEmail || recentlySent) return
    setResending(true)
    try {
      await authApi.resendVerification(registeredEmail)
      toast.success('Письмо отправлено')
      setRecentlySent(true)
      setTimeout(() => setRecentlySent(false), 60_000)
    } catch {
      toast.error('Не удалось отправить письмо, попробуйте позже')
    } finally {
      setResending(false)
    }
  }

  function comingSoon() {
    toast.info('Скоро будет доступно')
  }

  if (registeredEmail) {
    return (
      <div className="min-h-screen bg-[#f7f8fa] flex items-center justify-center p-4">
        <div className="w-full max-w-sm">
          <div className="text-center mb-8">
            <Link to="/" className="inline-flex items-center gap-2 font-bold text-xl text-[#111]">
              <span className="w-9 h-9 rounded-xl bg-[#ffcc00] flex items-center justify-center text-[#111] font-bold">R</span>
              RepricerX
            </Link>
          </div>
          <div className="bg-white rounded-3xl border border-[#e6e6e6] p-8 shadow-sm text-center">
            <div className="w-14 h-14 rounded-full bg-[#eff6ff] flex items-center justify-center mx-auto mb-4">
              <Mail className="h-7 w-7 text-[#2563eb]" />
            </div>
            <h2 className="text-xl font-bold text-[#111] mb-2">Проверьте почту</h2>
            <p className="text-sm text-[#666] mb-6">
              Письмо со ссылкой для подтверждения отправлено на{' '}
              <strong className="text-[#111]">{registeredEmail}</strong>.
              Ссылка действует 24 часа.
            </p>
            <Button
              onClick={handleResend}
              disabled={resending || recentlySent}
              variant="outline"
              className="w-full"
            >
              {resending ? 'Отправляем...' : recentlySent ? 'Письмо отправлено' : 'Отправить письмо ещё раз'}
            </Button>
            <p className="text-sm text-[#666] mt-4">
              Уже подтвердили?{' '}
              <Link to="/login" className="text-[#111] font-medium hover:underline">
                Войти
              </Link>
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-[#f7f8fa] flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <Link to="/" className="inline-flex items-center gap-2 font-bold text-xl text-[#111]">
            <span className="w-9 h-9 rounded-xl bg-[#ffcc00] flex items-center justify-center text-[#111] font-bold">R</span>
            RepricerX
          </Link>
          <h1 className="text-2xl font-bold text-[#111] mt-6 mb-1">Создать аккаунт</h1>
          <p className="text-sm text-[#666]">Бесплатно, без карты</p>
        </div>

        <div className="bg-white rounded-3xl border border-[#e6e6e6] p-8 shadow-sm">
          <div className="flex flex-col gap-3 mb-6">
            <SocialButton icon={<VKIcon />} label="Зарегистрироваться через VK ID" onClick={comingSoon} />
            <SocialButton icon={<YandexIcon />} label="Зарегистрироваться через Яндекс" onClick={comingSoon} />
          </div>

          <div className="relative flex items-center gap-3 mb-6">
            <div className="flex-1 h-px bg-[#e6e6e6]" />
            <span className="text-xs text-[#aaa]">или по email</span>
            <div className="flex-1 h-px bg-[#e6e6e6]" />
          </div>

          <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
            <div>
              <Label htmlFor="displayName">Имя</Label>
              <Input
                id="displayName"
                type="text"
                autoComplete="name"
                className="mt-1.5"
                placeholder="Иван Иванов"
                value={displayName}
                onChange={e => setDisplayName(e.target.value)}
              />
              {errors.displayName && <p className="text-xs text-red-500 mt-1">{errors.displayName}</p>}
            </div>

            <div>
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                autoComplete="email"
                className="mt-1.5"
                placeholder="you@example.com"
                value={email}
                onChange={e => setEmail(e.target.value)}
              />
              {errors.email && <p className="text-xs text-red-500 mt-1">{errors.email}</p>}
            </div>

            <div>
              <Label htmlFor="password">Пароль</Label>
              <div className="relative mt-1.5">
                <Input
                  id="password"
                  type={showPass ? 'text' : 'password'}
                  autoComplete="new-password"
                  placeholder="Минимум 8 символов"
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
              {errors.password && <p className="text-xs text-red-500 mt-1">{errors.password}</p>}
            </div>

            <Button type="submit" className="w-full mt-1" disabled={submitting}>
              {submitting ? 'Создаём аккаунт...' : 'Создать аккаунт'}
            </Button>
          </form>
        </div>

        <p className="text-center text-sm text-[#666] mt-6">
          Уже есть аккаунт?{' '}
          <Link to="/login" className="text-[#111] font-medium hover:underline">
            Войти
          </Link>
        </p>
      </div>
    </div>
  )
}
