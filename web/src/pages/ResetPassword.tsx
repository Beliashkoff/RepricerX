import { useLayoutEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Eye, EyeOff, ShieldCheck } from 'lucide-react'
import { authApi } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { passwordRequirementText, validatePasswordBasics } from '@/lib/authValidation'

type ResetStatus = 'checking' | 'ready' | 'loading' | 'success' | 'error' | 'invalid-token'

function readTokenFromFragment() {
  const hash = window.location.hash.startsWith('#') ? window.location.hash.slice(1) : window.location.hash
  const params = new URLSearchParams(hash)
  return params.get('token')?.trim() ?? ''
}

function clearTokenFromAddressBar() {
  const params = new URLSearchParams(window.location.search)
  params.delete('token')
  const search = params.toString()
  const nextUrl = `${window.location.pathname}${search ? `?${search}` : ''}`

  if (!window.location.hash && nextUrl === `${window.location.pathname}${window.location.search}`) return
  window.history.replaceState(null, document.title, nextUrl)
}

function isTokenError(message: string) {
  return /token|expired|invalid|истек|истёк|недейств/i.test(message)
}

export default function ResetPassword() {
  const [token, setToken] = useState('')
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirmation, setShowConfirmation] = useState(false)
  const [status, setStatus] = useState<ResetStatus>('checking')
  const [errors, setErrors] = useState<{ password?: string; confirmation?: string; submit?: string }>({})

  useLayoutEffect(() => {
    const parsedToken = readTokenFromFragment()
    clearTokenFromAddressBar()
    setToken(parsedToken)
    setStatus(parsedToken ? 'ready' : 'invalid-token')
  }, [])

  function validate() {
    const nextErrors: typeof errors = {}
    if (!password) nextErrors.password = 'Введите новый пароль'
    else nextErrors.password = validatePasswordBasics(password)

    if (!confirmation) nextErrors.confirmation = 'Повторите пароль'
    else if (password !== confirmation) nextErrors.confirmation = 'Пароли не совпадают'

    return Object.fromEntries(Object.entries(nextErrors).filter(([, value]) => value))
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault()
    if (!token) {
      setStatus('invalid-token')
      return
    }

    const validationErrors = validate()
    setErrors(validationErrors)
    if (Object.keys(validationErrors).length) return

    setStatus('loading')
    try {
      await authApi.resetPassword(token, password, confirmation)
      setStatus('success')
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Не удалось обновить пароль'
      if (isTokenError(message)) {
        setStatus('invalid-token')
        return
      }
      setErrors({ submit: message })
      setStatus('error')
    }
  }

  return (
    <div className="min-h-screen bg-[#f7f8fa] flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <Link to="/" className="inline-flex items-center gap-2 font-bold text-xl text-[#111]">
            <span className="w-9 h-9 rounded-xl bg-[#ffcc00] flex items-center justify-center text-[#111] font-bold">R</span>
            RepricerX
          </Link>
          <h1 className="text-2xl font-bold text-[#111] mt-6 mb-1">Новый пароль</h1>
          <p className="text-sm text-[#666]">Ссылка одноразовая и ограничена по времени</p>
        </div>

        <div className="bg-white rounded-3xl border border-[#e6e6e6] p-8 shadow-sm">
          {status === 'checking' && (
            <div className="flex flex-col items-center gap-4 py-6">
              <div className="w-8 h-8 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
              <p className="text-sm text-[#666]">Проверяем ссылку...</p>
            </div>
          )}

          {status === 'invalid-token' && (
            <div className="text-center">
              <h2 className="text-lg font-semibold text-[#111]">Ссылка недействительна</h2>
              <p className="mt-2 text-sm leading-6 text-[#666]">
                Запросите новую ссылку для сброса пароля.
              </p>
              <Button asChild className="mt-6 w-full">
                <Link to="/forgot-password">Запросить ссылку</Link>
              </Button>
            </div>
          )}

          {status === 'success' && (
            <div className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-[#fff6cc] text-[#111]">
                <ShieldCheck className="h-6 w-6" />
              </div>
              <h2 className="text-lg font-semibold text-[#111]">Пароль обновлён</h2>
              <p className="mt-2 text-sm leading-6 text-[#666]">
                Теперь можно войти с новым паролем.
              </p>
              <Button asChild className="mt-6 w-full">
                <Link to="/login">Войти</Link>
              </Button>
            </div>
          )}

          {(status === 'ready' || status === 'loading' || status === 'error') && (
            <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
              <div>
                <Label htmlFor="password">Новый пароль</Label>
                <div className="relative mt-1.5">
                  <Input
                    id="password"
                    type={showPassword ? 'text' : 'password'}
                    autoComplete="new-password"
                    placeholder="Минимум 12 символов"
                    value={password}
                    onChange={e => setPassword(e.target.value)}
                    className="pr-10"
                    disabled={status === 'loading'}
                  />
                  <button
                    type="button"
                    aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}
                    onClick={() => setShowPassword(v => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-[#aaa] hover:text-[#666]"
                    disabled={status === 'loading'}
                  >
                    {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                {errors.password && <p className="text-xs text-red-500 mt-1">{errors.password}</p>}
              </div>

              <div>
                <Label htmlFor="confirmation">Повторите пароль</Label>
                <div className="relative mt-1.5">
                  <Input
                    id="confirmation"
                    type={showConfirmation ? 'text' : 'password'}
                    autoComplete="new-password"
                    placeholder="Ещё раз"
                    value={confirmation}
                    onChange={e => setConfirmation(e.target.value)}
                    className="pr-10"
                    disabled={status === 'loading'}
                  />
                  <button
                    type="button"
                    aria-label={showConfirmation ? 'Скрыть повтор пароля' : 'Показать повтор пароля'}
                    onClick={() => setShowConfirmation(v => !v)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-[#aaa] hover:text-[#666]"
                    disabled={status === 'loading'}
                  >
                    {showConfirmation ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                {errors.confirmation && <p className="text-xs text-red-500 mt-1">{errors.confirmation}</p>}
              </div>

              <p className="text-xs leading-5 text-[#666]">{passwordRequirementText}.</p>
              {errors.submit && <p className="text-sm leading-5 text-red-500">{errors.submit}</p>}

              <Button type="submit" className="w-full mt-1" disabled={status === 'loading'}>
                {status === 'loading' ? 'Обновляем...' : 'Обновить пароль'}
              </Button>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}
