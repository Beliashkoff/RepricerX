const STATS = [
  { value: '2', label: 'маркетплейса', sub: 'Wildberries и Ozon' },
  { value: '4', label: 'типа стратегий', sub: 'на любой случай' },
  { value: '180', label: 'дней хранения', sub: 'журнала изменений' },
  { value: '5 rps', label: 'rate-limit', sub: 'на магазин по умолчанию' },
]

export function StatsBlock() {
  return (
    <section className="bg-[#f7f8fa] border-y border-[#e6e6e6]">
      <div className="max-w-6xl mx-auto px-6 py-16">
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-8">
          {STATS.map(({ value, label, sub }) => (
            <div key={label} className="text-center">
              <div className="text-4xl font-bold text-[#111] mb-1">{value}</div>
              <div className="text-sm font-medium text-[#333]">{label}</div>
              <div className="text-xs text-[#aaa] mt-0.5">{sub}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
