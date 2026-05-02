import { TrendingDown, Store, BarChart2, Shield, Bell, RefreshCw } from 'lucide-react'

const FEATURES = [
  {
    icon: TrendingDown,
    title: 'Умные стратегии',
    desc: '4 типа стратегий: ниже медианы, минимальная цена конкурента + шаг, минимальная маржа, фиксированная. Задайте правила — система применит их автоматически.',
  },
  {
    icon: Store,
    title: 'WB и Ozon',
    desc: 'Единый кабинет для Wildberries и Ozon. Подключите любое количество магазинов и управляйте всеми ценами из одного места.',
  },
  {
    icon: BarChart2,
    title: 'Симуляция цен',
    desc: 'Проверьте, как сработает стратегия, прежде чем запустить её. Симуляция покажет итоговую цену с учётом всех ограничений.',
  },
  {
    icon: Shield,
    title: 'Защита от потерь',
    desc: 'Задайте минимальную цену, максимальный процент изменения и интервал обновления. Система никогда не выйдет за заданные рамки.',
  },
  {
    icon: RefreshCw,
    title: 'Автообновление',
    desc: 'Расписание пересчёта от 1 раза в минуту до 1 раза в день. Ручной запуск по одному нажатию. Rate-limit по умолчанию 5 rps на магазин.',
  },
  {
    icon: Bell,
    title: 'Журнал и экспорт',
    desc: 'История всех изменений цен хранится 180 дней. Фильтры по магазину, SKU и дате. Экспорт в CSV одной кнопкой.',
  },
]

export function Features() {
  return (
    <section id="features" className="bg-[#f7f8fa] py-24">
      <div className="max-w-6xl mx-auto px-6">
        <div className="text-center mb-14">
          <h2 className="text-[#111] mb-4">Всё необходимое для управления ценами</h2>
          <p className="text-lg text-[#555] max-w-xl mx-auto">
            Инструменты, которые нужны продавцу на маркетплейсах — без лишнего.
          </p>
        </div>
        <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {FEATURES.map(({ icon: Icon, title, desc }) => (
            <div key={title} className="bg-white rounded-3xl p-7 border border-[#e6e6e6]">
              <div className="w-11 h-11 rounded-2xl bg-[#fff9cc] flex items-center justify-center mb-5">
                <Icon className="w-5 h-5 text-[#7a6000]" />
              </div>
              <h3 className="text-base font-semibold text-[#111] mb-2">{title}</h3>
              <p className="text-sm text-[#666] leading-relaxed">{desc}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
