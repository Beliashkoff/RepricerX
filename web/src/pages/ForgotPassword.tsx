import { useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { MailCheck } from 'lucide-react'
import { authApi } from '@/api/auth'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isValidEmail } from '@/lib/authValidation'

export default function ForgotPassword() {
  const [email, setEmail] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [error, setError] = useState('')

  function validate() {
    if (!email) return 'Введите email'
    if (!isValidEmail(email)) return 'Некорректный email'
    return ''
  }

  async function handleSubmit(ev: React.FormEvent) {
    ev.preventDefault()
    const validationError = validate()
    setError(validationError)
    if (validationError) return

    setSubmitting(true)
    try {
      await authApi.forgotPassword(email)
      setSubmitted(true)
    } catch (err: unknown) {
      toast.error(err instanceof Error ? err.message : 'Не удалось отправить письмо')
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
          <h1 className="text-2xl font-bold text-[#111] mt-6 mb-1">Восстановить пароль</h1>
          <p className="text-sm text-[#666]">Укажите email аккаунта</p>
        </div>

        <div className="bg-white rounded-3xl border border-[#e6e6e6] p-8 shadow-sm">
          {submitted ? (
            <div className="text-center">
              <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-[#fff6cc] text-[#111]">
                <MailCheck className="h-6 w-6" />
              </div>
              <h2 className="text-lg font-semibold text-[#111]">Проверьте почту</h2>
              <p className="mt-2 text-sm leading-6 text-[#666]">
                Если аккаунт с таким email существует, мы отправим ссылку для сброса пароля.
              </p>
              <Button asChild className="mt-6 w-full" variant="secondary">
                <Link to="/login">Вернуться ко входу</Link>
              </Button>
            </div>
          ) : (
            <form onSubmit={handleSubmit} noValidate className="flex flex-col gap-4">
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
                {error && <p className="text-xs text-red-500 mt-1">{error}</p>}
              </div>

              <Button type="submit" className="w-full mt-1" disabled={submitting}>
                {submitting ? 'Отправляем...' : 'Отправить ссылку'}
              </Button>
            </form>
          )}
        </div>

        <p className="text-center text-sm text-[#666] mt-6">
          Вспомнили пароль?{' '}
          <Link to="/login" className="text-[#111] font-medium hover:underline">
            Войти
          </Link>
        </p>
      </div>
    </div>
  )
}
