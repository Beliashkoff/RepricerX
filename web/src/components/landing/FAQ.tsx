import * as Accordion from '@radix-ui/react-accordion'
import { Plus, Minus } from 'lucide-react'
import { useState } from 'react'

const FAQ_ITEMS = [
  {
    q: 'Какие маркетплейсы поддерживаются?',
    a: 'Wildberries и Ozon. Для WB нужен один API-ключ с правами на обновление цен, для Ozon — Client ID и API Key из личного кабинета продавца.',
  },
  {
    q: 'Безопасно ли хранить API-ключи?',
    a: 'Да. Все ключи хранятся в зашифрованном виде (AES-GCM), ключ шифрования задаётся на стороне сервера и никогда не попадает в базу данных.',
  },
  {
    q: 'Что произойдёт, если данных о конкурентах нет?',
    a: 'Сработает fallback-политика стратегии: держать текущую цену, установить минимальную или фиксированную. Вы выбираете поведение при создании стратегии.',
  },
  {
    q: 'Можно ли задать ограничения на изменение цены?',
    a: 'Да. Для каждой стратегии можно указать: минимальную цену, максимальную цену, максимальный % изменения за один шаг (0–50%) и минимальный интервал между обновлениями (1–1440 минут).',
  },
  {
    q: 'Как посмотреть историю изменений?',
    a: 'Раздел «Журнал» показывает все изменения цен за последние 180 дней. Есть фильтры по магазину, SKU и дате. Историю можно скачать в CSV.',
  },
  {
    q: 'Что значит «симуляция»?',
    a: 'Вы можете рассчитать, какой будет итоговая цена для конкретного товара при заданной стратегии — без фактической отправки в маркетплейс. Удобно для проверки настроек.',
  },
]

export function FAQ() {
  const [open, setOpen] = useState<string>('')

  return (
    <section id="faq" className="py-24">
      <div className="max-w-3xl mx-auto px-6">
        <div className="text-center mb-12">
          <h2 className="text-[#111] mb-4">Часто задаваемые вопросы</h2>
        </div>
        <Accordion.Root type="single" collapsible value={open} onValueChange={setOpen} className="flex flex-col gap-2">
          {FAQ_ITEMS.map((item, i) => (
            <Accordion.Item key={i} value={String(i)} className="bg-[#f7f8fa] rounded-2xl border border-[#e6e6e6] overflow-hidden">
              <Accordion.Header>
                <Accordion.Trigger className="flex w-full items-center justify-between px-6 py-5 text-left text-sm font-semibold text-[#111] hover:bg-white/50 transition-colors gap-4">
                  {item.q}
                  {open === String(i) ? (
                    <Minus className="h-4 w-4 shrink-0 text-[#666]" />
                  ) : (
                    <Plus className="h-4 w-4 shrink-0 text-[#666]" />
                  )}
                </Accordion.Trigger>
              </Accordion.Header>
              <Accordion.Content className="px-6 pb-5 text-sm text-[#555] leading-relaxed data-[state=open]:animate-in data-[state=closed]:animate-out">
                {item.a}
              </Accordion.Content>
            </Accordion.Item>
          ))}
        </Accordion.Root>
      </div>
    </section>
  )
}
