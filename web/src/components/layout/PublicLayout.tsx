import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'
import { Button } from '@/components/ui/button'

function Logo() {
  return (
    <Link to="/" className="flex items-center gap-2 font-bold text-xl text-[#111]">
      <span className="w-8 h-8 rounded-lg bg-[#ffcc00] flex items-center justify-center text-[#111] font-bold text-sm">R</span>
      RepricerX
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
          <div className="grid grid-cols-2 gap-8 text-sm">
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
          </div>
        </div>
        <div className="max-w-6xl mx-auto px-6 mt-8 pt-8 border-t border-[#e6e6e6] text-sm text-[#666]">
          © 2026 RepricerX. Учебный проект НИУ ВШЭ.
        </div>
      </footer>
    </div>
  )
}
