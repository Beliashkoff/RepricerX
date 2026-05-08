import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'
import { Button } from '@/components/ui/button'
import { useEffect, useState } from 'react'

function Logo() {
  return (
    <Link
      to="/"
      aria-label="RepricerX на главную"
      className="flex items-center gap-2 font-bold text-2xl text-[#111] select-none"
    >
      <img
        src="/logo.png"
        alt="RepricerX logo"
        className="w-16 h-16 rounded-lg object-contain pointer-events-none"
        draggable={false}
      />
      <span>
        Repricer
        <span className="text-[#f2c200]">X</span>
      </span>
    </Link>
  )
}

export function PublicHeader() {
  const { user } = useAuth()
  const navigate = useNavigate()

  return (
    <header className="sticky top-0 z-40 bg-white/95 backdrop-blur-sm border-b border-[#e6e6e6]">
      <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
        <Logo />
        <nav className="hidden md:flex items-center gap-6 text-sm text-[#666]">
          <a href="#features" className="hover:text-[#111] transition-colors">Возможности</a>
          <a href="#how-it-works" className="hover:text-[#111] transition-colors">Как работает</a>
          <a href="#faq" className="hover:text-[#111] transition-colors">FAQ</a>
        </nav>
        <div className="flex items-center gap-3">
          {user ? (
            <Button onClick={() => navigate('/dashboard')}>Перейти в кабинет</Button>
          ) : (
            <>
              <Button variant="secondary" onClick={() => navigate('/login')}>Войти</Button>
              <Button onClick={() => navigate('/register')}>Начать бесплатно</Button>
            </>
          )}
        </div>
      </div>
    </header>
  )
}

const cookieConsentKey = 'repricerx_cookie_consent_v1'

function CookieBanner() {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    setVisible(localStorage.getItem(cookieConsentKey) === null)
  }, [])

  function accept(value: 'necessary' | 'all') {
    localStorage.setItem(cookieConsentKey, value)
    setVisible(false)
  }

  if (!visible) return null

  return (
    <div className="fixed left-4 right-4 bottom-4 z-50 mx-auto max-w-3xl rounded-2xl border border-[#e6e6e6] bg-white p-4 shadow-lg">
      <div className="flex flex-col md:flex-row gap-4 md:items-center md:justify-between">
        <p className="text-sm text-[#555] leading-6">
          RepricerX использует необходимые cookies для входа и безопасности. Аналитические и маркетинговые cookies включаются только после согласия.
          {' '}
          <Link to="/legal/cookies" className="font-medium text-[#111] underline underline-offset-2">
            Подробнее
          </Link>
        </p>
        <div className="flex shrink-0 gap-2">
          <Button variant="secondary" size="sm" onClick={() => accept('necessary')}>
            Только необходимые
          </Button>
          <Button size="sm" onClick={() => accept('all')}>
            Принять все
          </Button>
        </div>
      </div>
    </div>
  )
}

export function PublicLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-white">
      <PublicHeader />
      <main>{children}</main>
      <footer className="border-t border-[#e6e6e6] py-12 mt-20">
        <div className="max-w-6xl mx-auto px-6 flex flex-col md:flex-row justify-between items-start gap-8">
          <div>
            <Logo />
            <p className="text-sm text-[#666] mt-2 max-w-xs">
              Автоматическое управление ценами на Wildberries и Ozon
            </p>
          </div>
          <div className="grid grid-cols-2 md:grid-cols-3 gap-8 text-sm">
            <div>
              <p className="font-medium text-[#111] mb-3">Продукт</p>
              <div className="flex flex-col gap-2 text-[#666]">
                <a href="#features" className="hover:text-[#111]">Возможности</a>
                <a href="#how-it-works" className="hover:text-[#111]">Как работает</a>
                <a href="#faq" className="hover:text-[#111]">FAQ</a>
              </div>
            </div>
            <div>
              <p className="font-medium text-[#111] mb-3">Аккаунт</p>
              <div className="flex flex-col gap-2 text-[#666]">
                <Link to="/login" className="hover:text-[#111]">Войти</Link>
                <Link to="/register" className="hover:text-[#111]">Регистрация</Link>
              </div>
            </div>
            <div>
              <p className="font-medium text-[#111] mb-3">Документы</p>
              <div className="flex flex-col gap-2 text-[#666]">
                <Link to="/legal/terms" className="hover:text-[#111]">Оферта</Link>
                <Link to="/legal/privacy" className="hover:text-[#111]">Персональные данные</Link>
                <Link to="/legal/cookies" className="hover:text-[#111]">Cookies</Link>
                <Link to="/legal/archive" className="hover:text-[#111]">Архив</Link>
              </div>
            </div>
          </div>
        </div>
        <div className="max-w-6xl mx-auto px-6 mt-8 pt-8 border-t border-[#e6e6e6] text-sm text-[#666]">
          © 2026 RepricerX.
        </div>
      </footer>
      <CookieBanner />
    </div>
  )
}
