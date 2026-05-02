import { useState } from 'react'
import { cn } from '@/lib/utils'

const STEPS = [
  {
    num: '01',
    title: 'Подключите магазин',
    desc: 'Введите API-ключ Wildberries или Client ID + API Key Ozon. RepricerX проверит подключение и покажет статус. Credentials хранятся в зашифрованном виде.',
    detail: 'Поддерживаем любое количество магазинов. Каждый магазин настраивается независимо: своё расписание, свои стратегии, свои ограничения.',
  },
  {
    num: '02',
    title: 'Импортируйте товары',
    desc: 'Нажмите «Импортировать» — система сама загрузит все SKU из магазина. Для каждого товара можно задать минимальную цену, максимальную цену и себестоимость.',
    detail: 'Работаем в фоне: импорт не блокирует интерфейс. Статус задачи обновляется в реальном времени. Ошибочные товары попадают в отдельный лог.',
  },
  {
    num: '03',
    title: 'Создайте стратегию',
    desc: 'Выберите тип стратегии и задайте параметры: процент ниже медианы, шаг от минимальной цены конкурента, минимальную маржу или фиксированную цену.',
    detail: 'Стратегию можно применить к одному товару, группе или всему магазину. Если данных о конкурентах нет — сработает fallback-политика (держать текущую, ставить минимальную или фиксированную).',
  },
  {
    num: '04',
    title: 'Запустите автообновление',
    desc: 'Настройте расписание или нажмите «Запустить сейчас». RepricerX рассчитает оптимальные цены и отправит их в маркетплейс через официальный API.',
    detail: 'Все изменения фиксируются в журнале. Rate-limit, ретраи при ошибках, уведомления о результатах — всё включено.',
  },
]

export function HowItWorks() {
  const [active, setActive] = useState(0)

  return (
    <section id="how-it-works" className="py-24">
      <div className="max-w-6xl mx-auto px-6">
        <div className="text-center mb-14">
          <h2 className="text-[#111] mb-4">Как это работает</h2>
          <p className="text-lg text-[#555] max-w-xl mx-auto">
            Четыре шага от подключения до автоматических продаж
          </p>
        </div>
        <div className="grid lg:grid-cols-2 gap-12 items-start">
          {/* Step tabs */}
          <div className="flex flex-col gap-2">
            {STEPS.map((step, i) => (
              <button
                key={i}
                onClick={() => setActive(i)}
                className={cn(
                  'text-left px-6 py-5 rounded-2xl border transition-all',
                  active === i
                    ? 'bg-[#ffcc00] border-[#ffcc00]'
                    : 'bg-white border-[#e6e6e6] hover:border-[#ffcc00]/40 hover:bg-[#fffae6]'
                )}
              >
                <div className="flex items-center gap-4">
                  <span className={cn('text-sm font-bold', active === i ? 'text-[#111]' : 'text-[#aaa]')}>{step.num}</span>
                  <span className={cn('font-semibold', active === i ? 'text-[#111]' : 'text-[#333]')}>{step.title}</span>
                </div>
              </button>
            ))}
          </div>

          {/* Active step detail */}
          <div className="bg-[#f7f8fa] rounded-3xl p-8 border border-[#e6e6e6]">
            <div className="text-4xl font-bold text-[#ffcc00] mb-4">{STEPS[active].num}</div>
            <h3 className="text-xl font-semibold text-[#111] mb-3">{STEPS[active].title}</h3>
            <p className="text-[#555] leading-relaxed mb-4">{STEPS[active].desc}</p>
            <p className="text-sm text-[#666] leading-relaxed p-4 bg-white rounded-2xl border border-[#e6e6e6]">
              {STEPS[active].detail}
            </p>
          </div>
        </div>
      </div>
    </section>
  )
}
