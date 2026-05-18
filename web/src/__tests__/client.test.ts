import { describe, it, expect } from 'vitest'

// Тестируем бизнес-логику interceptors в изоляции без реального axios

// --- Response error interceptor: логика формирования сообщения об ошибке ---
describe('response error interceptor', () => {
  function applyErrorInterceptor(err: unknown): never {
    const e = err as { response?: { data?: { error?: { message?: string }; message?: string } }; message?: string }
    const msg = e.response?.data?.error?.message || e.response?.data?.message || e.message || 'Ошибка запроса'
    throw new Error(msg)
  }

  it('берёт message из response.data.error.message', () => {
    expect(() =>
      applyErrorInterceptor({ response: { data: { error: { message: 'Недействительный токен' } } } })
    ).toThrow('Недействительный токен')
  })

  it('берёт message из response.data.message', () => {
    expect(() =>
      applyErrorInterceptor({ response: { data: { message: 'Не найдено' } } })
    ).toThrow('Не найдено')
  })

  it('берёт message из err.message если нет response.data.message', () => {
    expect(() =>
      applyErrorInterceptor({ response: { data: {} }, message: 'Network Error' })
    ).toThrow('Network Error')
  })

  it('использует err.message если нет response', () => {
    expect(() =>
      applyErrorInterceptor({ message: 'timeout' })
    ).toThrow('timeout')
  })

  it('использует fallback "Ошибка запроса" если нет message', () => {
    expect(() => applyErrorInterceptor({})).toThrow('Ошибка запроса')
  })

  it('response.data.message приоритетнее err.message', () => {
    expect(() =>
      applyErrorInterceptor({
        response: { data: { message: 'Конкретная ошибка' } },
        message: 'Общая ошибка',
      })
    ).toThrow('Конкретная ошибка')
  })

  it('response.data.error.message приоритетнее response.data.message', () => {
    expect(() =>
      applyErrorInterceptor({
        response: { data: { error: { message: 'Ошибка envelope' }, message: 'Старый формат' } },
        message: 'Общая ошибка',
      })
    ).toThrow('Ошибка envelope')
  })
})

// --- Конфигурация axios client (проверяем импорт не падает) ---
describe('api/client module', () => {
  it('импортируется без ошибок', async () => {
    await expect(import('../api/client')).resolves.toBeDefined()
  })

  it('экспортирует default (axios instance)', async () => {
    const mod = await import('../api/client')
    expect(mod.default).toBeDefined()
    expect(typeof mod.default.get).toBe('function')
    expect(typeof mod.default.post).toBe('function')
  })
})
