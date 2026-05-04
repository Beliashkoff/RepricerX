import { describe, it, expect, beforeEach } from 'vitest'
import { render, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'

// Заменяем window.location stub-объектом, чтобы ловить присвоение href.
beforeEach(() => {
  Object.defineProperty(window, 'location', {
    configurable: true,
    writable: true,
    value: { href: '' },
  })
})

const { default: VerifyEmail } = await import('../pages/VerifyEmail')

function renderVerify(search = '') {
  return render(
    <MemoryRouter initialEntries={[`/verify${search}`]}>
      <VerifyEmail />
    </MemoryRouter>,
  )
}

describe('VerifyEmail — внешний вид', () => {
  it('показывает спиннер на странице', () => {
    renderVerify('?token=tok')
    expect(document.querySelector('.animate-spin')).toBeInTheDocument()
  })

  it('не показывает текст — только анимацию загрузки', () => {
    const { container } = renderVerify('?token=tok')
    expect(container.textContent).toBe('')
  })
})

describe('VerifyEmail — редирект', () => {
  it('перенаправляет на /api/auth/verify с token из query', async () => {
    renderVerify('?token=abc123')
    await waitFor(() =>
      expect(window.location.href).toBe('/api/auth/verify?token=abc123'),
    )
  })

  it('без token перенаправляет с пустым параметром', async () => {
    renderVerify()
    await waitFor(() =>
      expect(window.location.href).toBe('/api/auth/verify?token='),
    )
  })

  it('URL-кодирует спецсимволы в token', async () => {
    // useSearchParams декодирует %40 → @; компонент повторно кодирует через encodeURIComponent
    renderVerify('?token=user%40example.com')
    await waitFor(() =>
      expect(window.location.href).toBe(
        '/api/auth/verify?token=user%40example.com',
      ),
    )
  })

  it('URL-кодирует пробелы и плюсы в token', async () => {
    renderVerify('?token=hello+world')
    // '+' в query string — это пробел; params.get вернёт 'hello world'
    await waitFor(() =>
      expect(window.location.href).toBe(
        '/api/auth/verify?token=hello%20world',
      ),
    )
  })
})
