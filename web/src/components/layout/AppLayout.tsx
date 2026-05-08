import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/context/AuthContext'
import {
  LayoutDashboard, Store, Package, TrendingUp, BarChart2,
  BookOpen, Settings, LogOut, ChevronRight, Bell,
} from 'lucide-react'
import { cn } from '@/lib/utils'

const NAV_ITEMS = [
  { path: '/dashboard', label: 'Дашборд', icon: LayoutDashboard },
  { path: '/shops', label: 'Магазины', icon: Store },
  { path: '/products', label: 'Товары', icon: Package },
  { path: '/strategies', label: 'Стратегии', icon: TrendingUp },
  { path: '/pricing', label: 'Симуляция', icon: BarChart2 },
  { path: '/audit', label: 'Журнал', icon: BookOpen },
  { path: '/settings', label: 'Настройки', icon: Settings },
]

function Logo() {
  return (
    <Link to="/dashboard" className="flex items-center gap-2 font-bold text-2xl text-[#111] select-none">
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

function Sidebar() {
  const location = useLocation()
  const { logout } = useAuth()
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/')
  }

  return (
    <aside className="w-60 shrink-0 h-screen sticky top-0 bg-[#f7f8fa] border-r border-[#e6e6e6] flex flex-col">
      <div className="px-5 py-5 border-b border-[#e6e6e6]">
        <Logo />
      </div>
      <nav className="flex-1 px-3 py-4 flex flex-col gap-0.5 overflow-y-auto">
        {NAV_ITEMS.map(({ path, label, icon: Icon }) => {
          const active = location.pathname === path || (path !== '/dashboard' && location.pathname.startsWith(path))
          return (
            <Link
              key={path}
              to={path}
              className={cn(
                'flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-colors',
                active
                  ? 'bg-[#ffcc00] text-[#111]'
                  : 'text-[#666] hover:bg-white hover:text-[#111]'
              )}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {label}
              {active && <ChevronRight className="h-3 w-3 ml-auto" />}
            </Link>
          )
        })}
      </nav>
      <div className="p-3 border-t border-[#e6e6e6]">
        <button
          onClick={handleLogout}
          className="flex items-center gap-3 px-3 py-2.5 w-full rounded-xl text-sm font-medium text-[#666] hover:bg-red-50 hover:text-red-600 transition-colors"
        >
          <LogOut className="h-4 w-4" />
          Выйти
        </button>
      </div>
    </aside>
  )
}

function TopBar() {
  const { user } = useAuth()
  const navigate = useNavigate()
  const initials = (user?.displayName ?? user?.email ?? '?')[0].toUpperCase()

  return (
    <header className="h-14 bg-white border-b border-[#e6e6e6] flex items-center justify-between px-6 gap-3">
      <Logo />
      <button className="w-9 h-9 rounded-xl bg-[#f5f5f5] flex items-center justify-center text-[#666] hover:bg-[#e8e9eb] transition-colors">
        <Bell className="h-4 w-4" />
      </button>
      <button
        onClick={() => navigate('/settings')}
        className="flex items-center gap-2 px-3 py-1.5 rounded-xl bg-[#f5f5f5] hover:bg-[#e8e9eb] transition-colors"
      >
        <div className="w-6 h-6 rounded-full bg-[#ffcc00] flex items-center justify-center text-xs font-bold text-[#111]">
          {initials}
        </div>
        <span className="text-sm font-medium text-[#111]">
          {user?.displayName ?? user?.email ?? 'Пользователь'}
        </span>
      </button>
    </header>
  )
}

export function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen bg-white overflow-hidden">
      <Sidebar />
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        <TopBar />
        <main className="flex-1 overflow-y-auto bg-[#f7f8fa] p-6">
          {children}
        </main>
      </div>
    </div>
  )
}

export function PageHeader({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between mb-6">
      <div>
        <h1 className="text-2xl font-bold text-[#111]">{title}</h1>
        {description && <p className="text-sm text-[#666] mt-1">{description}</p>}
      </div>
      {action && <div>{action}</div>}
    </div>
  )
}

export function StatCard({ label, value, sub, accent }: { label: string; value: string | number; sub?: string; accent?: boolean }) {
  return (
    <div className={cn('rounded-2xl p-6', accent ? 'bg-[#ffcc00]' : 'bg-white border border-[#e6e6e6]')}>
      <p className={cn('text-xs font-medium mb-1', accent ? 'text-[#111]' : 'text-[#666]')}>{label}</p>
      <p className="text-3xl font-bold text-[#111]">{value}</p>
      {sub && <p className={cn('text-xs mt-1', accent ? 'text-[#111]/60' : 'text-[#aaa]')}>{sub}</p>}
    </div>
  )
}

export function EmptyState({ title, description, action }: { title: string; description?: string; action?: React.ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center py-16 gap-3">
      <div className="w-12 h-12 rounded-2xl bg-[#f5f5f5] flex items-center justify-center mb-2">
        <Package className="w-6 h-6 text-[#aaa]" />
      </div>
      <p className="text-base font-semibold text-[#111]">{title}</p>
      {description && <p className="text-sm text-[#666] text-center max-w-sm">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}
