import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { ArrowRight, CheckCircle2 } from 'lucide-react'

const FEATURES_BRIEF = [
  'Wildberries и Ozon из одного кабинета',
  'Стратегии по медиане конкурентов',
  'Журнал изменений с экспортом',
]

export function Hero() {
  const navigate = useNavigate()
  return (
    <section className="max-w-6xl mx-auto px-6 pt-20 pb-24">
      <div className="grid lg:grid-cols-2 gap-12 items-center">
        <div>
          <div className="inline-flex items-center gap-2 bg-[#fff9cc] text-[#7a6000] text-xs font-medium px-3 py-1.5 rounded-full mb-6">
            <span className="w-1.5 h-1.5 rounded-full bg-[#ffcc00]" />
            Wildberries & Ozon - в одном месте
          </div>
          <h1 className="text-[#111] mb-6">
            Автоматическое управление ценами на маркетплейсах
          </h1>
          <p className="text-lg text-[#555] mb-8 leading-relaxed max-w-lg">
            Подключите магазин, настройте стратегию - RepricerX будет отслеживать конкурентов
            и обновлять цены автоматически, пока вы занимаетесь бизнесом.
          </p>
          <div className="flex flex-col sm:flex-row gap-3 mb-10">
            <Button size="lg" onClick={() => navigate('/register')} className="gap-2">
              Начать бесплатно <ArrowRight className="h-4 w-4" />
            </Button>
            <Button size="lg" variant="secondary" onClick={() => navigate('/login')}>
              Войти в кабинет
            </Button>
          </div>
          <div className="flex flex-col gap-2">
            {FEATURES_BRIEF.map(f => (
              <div key={f} className="flex items-center gap-2 text-sm text-[#555]">
                <CheckCircle2 className="h-4 w-4 text-[#111] shrink-0" />
                {f}
              </div>
            ))}
          </div>
        </div>

        {/* Product mockup */}
        <div className="relative lg:block">
          <div className="bg-[#f5f5f5] rounded-3xl p-6 border border-[#e6e6e6]">
            <div className="bg-white rounded-2xl p-4 border border-[#e6e6e6] mb-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-xs font-medium text-[#666]">Последние изменения цен</span>
                <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full">Активно</span>
              </div>
              {[
                { name: 'Кроссовки Nike Air Max', old: '8 990', new: '8 450', change: '-6.0%', ok: true },
                { name: 'Рюкзак городской 30л', old: '3 450', new: '3 200', change: '-7.2%', ok: true },
                { name: 'Футболка Adidas классик', old: '1 890', new: '1 750', change: '-7.4%', ok: true },
              ].map(row => (
                <div key={row.name} className="flex items-center justify-between py-2 border-b border-[#f5f5f5] last:border-0">
                  <span className="text-xs text-[#333] font-medium truncate max-w-[140px]">{row.name}</span>
                  <div className="flex items-center gap-2 text-xs shrink-0">
                    <span className="text-[#aaa] line-through">{row.old} ₽</span>
                    <span className="font-semibold text-[#111]">{row.new} ₽</span>
                    <span className="text-green-600">{row.change}</span>
                  </div>
                </div>
              ))}
            </div>
            <div className="grid grid-cols-3 gap-3">
              {[
                { label: 'Обновлено', val: '147', sub: 'за 30 дней' },
                { label: 'Успешно', val: '93%', sub: 'от общего' },
                { label: 'Магазины', val: '2', sub: 'активных' },
              ].map(s => (
                <div key={s.label} className="bg-white rounded-xl p-3 border border-[#e6e6e6]">
                  <p className="text-xs text-[#666]">{s.label}</p>
                  <p className="text-lg font-bold text-[#111]">{s.val}</p>
                  <p className="text-xs text-[#aaa]">{s.sub}</p>
                </div>
              ))}
            </div>
          </div>
          {/* Accent dot */}
          <div className="absolute -top-3 -right-3 w-8 h-8 bg-[#ffcc00] rounded-full" />
        </div>
      </div>
    </section>
  )
}
