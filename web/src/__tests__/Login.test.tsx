import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { act } from 'react'

const mockLogin = vi.fn()
const mockToastInfo = vi.fn()
const mockToastError = vi.fn()
const mockNavigate = vi.fn()

vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({
    user: null,
    isLoading: false,
    login: mockLogin,
    logout: vi.fn(),
    refreshMe: vi.fn(),
  }),
}))

vi.mock('sonner', () => ({
  toast: { info: (...a: unknown[]) => mockToastInfo(...a), error: (...a: unknown[]) => mockToastError(...a) },
  Toaster: () => null,
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

const { default: Login } = await import('../pages/Login')

function renderLogin() {
  return render(<MemoryRouter><Login /></MemoryRouter>)
}

const user = userEvent.setup()

describe('Login — отрисовка', () => {
  it('показывает поле email', () => {
    renderLogin()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
  })

  it('показывает поле пароль', () => {
    renderLogin()
    expect(screen.getByLabelText(/пароль/i)).toBeInTheDocument()
  })

  it('показывает кнопку «Войти»', () => {
    renderLogin()
    expect(screen.getByRole('button', { name: /^Войти$/i })).toBeInTheDocument()
  })

  it('показывает кнопку VK', () => {
    renderLogin()
    expect(screen.getByRole('button', { name: /VK ID/i })).toBeInTheDocument()
  })

  it('показывает кнопку Яндекс', () => {
    renderLogin()
    expect(screen.getByRole('button', { name: /Яндекс/i })).toBeInTheDocument()
  })

  it('содержит ссылку на регистрацию', () => {
    renderLogin()
    expect(screen.getByRole('link', { name: /Зарегистрироваться/i })).toBeInTheDocument()
  })

  it('содержит ссылку восстановления пароля', () => {
    renderLogin()
    expect(screen.getByRole('link', { name: /Забыли пароль/i })).toHaveAttribute('href', '/forgot-password')
  })
})

describe('Login — валидация', () => {
  beforeEach(() => vi.clearAllMocks())

  it('показывает "Введите email" при пустом email', async () => {
    renderLogin()
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })
    await waitFor(() => expect(screen.getByText('Введите email')).toBeInTheDocument())
  })

  it('показывает "Некорректный email" при невалидном email', async () => {
    renderLogin()
    await user.type(screen.getByLabelText(/email/i), 'notanemail')
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })
    await waitFor(() => expect(screen.getByText('Некорректный email')).toBeInTheDocument())
  })

  it('показывает "Введите пароль" при пустом пароле', async () => {
    renderLogin()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })
    await waitFor(() => expect(screen.getByText('Введите пароль')).toBeInTheDocument())
  })

  it('не вызывает login при невалидных данных', async () => {
    renderLogin()
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })
    await waitFor(() => screen.getByText('Введите email'))
    expect(mockLogin).not.toHaveBeenCalled()
  })
})

describe('Login — успешный вход', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockLogin.mockResolvedValue({ id: 'u1', email: 'user@example.com', displayName: 'U', status: 'active', createdAt: '' })
  })

  it('вызывает login и редиректит на /dashboard', async () => {
    renderLogin()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/пароль/i), 'ValidPass123')
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })

    await waitFor(() => expect(mockLogin).toHaveBeenCalledWith('user@example.com', 'ValidPass123'))
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/dashboard'))
  })
})

describe('Login — ошибка входа', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockLogin.mockRejectedValue(new Error('Неверный email или пароль'))
  })

  it('показывает toast с ошибкой', async () => {
    renderLogin()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/пароль/i), 'ValidPass123')
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })

    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith('Неверный email или пароль'))
  })

  it('не делает редирект при ошибке', async () => {
    renderLogin()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.type(screen.getByLabelText(/пароль/i), 'ValidPass123')
    await act(async () => { await user.click(screen.getByRole('button', { name: /^Войти$/i })) })

    await waitFor(() => expect(mockToastError).toHaveBeenCalled())
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})

describe('Login — социальные кнопки', () => {
  const originalLocation = window.location
  let assignedHref: string

  beforeEach(() => {
    vi.clearAllMocks()
    assignedHref = ''
    // Перехватываем присваивание window.location.href, не трогая остальные поля.
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: {
        ...originalLocation,
        get href() { return assignedHref },
        set href(v: string) { assignedHref = v },
        assign: (v: string) => { assignedHref = v },
        pathname: originalLocation.pathname,
        origin: originalLocation.origin,
      },
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation,
    })
  })

  it('VK → редирект на /api/auth/oauth/vk/start', async () => {
    renderLogin()
    await user.click(screen.getByRole('button', { name: /VK ID/i }))
    expect(assignedHref).toBe('/api/auth/oauth/vk/start')
  })

  it('Яндекс → редирект на /api/auth/oauth/yandex/start', async () => {
    renderLogin()
    await user.click(screen.getByRole('button', { name: /Яндекс/i }))
    expect(assignedHref).toBe('/api/auth/oauth/yandex/start')
  })

  it('VK не вызывает login', async () => {
    renderLogin()
    await user.click(screen.getByRole('button', { name: /VK ID/i }))
    expect(mockLogin).not.toHaveBeenCalled()
  })
})

describe('Login — показ/скрытие пароля', () => {
  it('поле пароля скрыто по умолчанию', () => {
    renderLogin()
    expect(screen.getByLabelText(/пароль/i)).toHaveAttribute('type', 'password')
  })

  it('клик на иконку показывает пароль', async () => {
    renderLogin()
    const passwordInput = screen.getByLabelText(/пароль/i)
    const toggleBtn = passwordInput.parentElement!.querySelector('button')!
    await user.click(toggleBtn)
    expect(passwordInput).toHaveAttribute('type', 'text')
  })

  it('повторный клик скрывает пароль', async () => {
    renderLogin()
    const passwordInput = screen.getByLabelText(/пароль/i)
    const toggleBtn = passwordInput.parentElement!.querySelector('button')!
    await user.click(toggleBtn)
    await user.click(toggleBtn)
    expect(passwordInput).toHaveAttribute('type', 'password')
  })
})
