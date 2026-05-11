import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

const mockLogin = vi.fn()
const mockRegister = vi.fn()
const mockToastError = vi.fn()
const mockToastInfo = vi.fn()
const mockNavigate = vi.fn()

vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({ user: null, isLoading: false, login: mockLogin, logout: vi.fn(), refreshMe: vi.fn() }),
}))

vi.mock('../api/auth', () => ({
  authApi: { register: (...a: unknown[]) => mockRegister(...a), login: vi.fn(), me: vi.fn(), logout: vi.fn(), updateMe: vi.fn() },
}))

vi.mock('sonner', () => ({
  toast: { info: (...a: unknown[]) => mockToastInfo(...a), error: (...a: unknown[]) => mockToastError(...a) },
  Toaster: () => null,
}))

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>()
  return { ...actual, useNavigate: () => mockNavigate }
})

const { default: Register } = await import('../pages/Register')

function renderRegister() {
  return render(<MemoryRouter><Register /></MemoryRouter>)
}

describe('Register — отрисовка', () => {
  it('показывает поля имя, email, пароль', () => {
    renderRegister()
    expect(screen.getByLabelText(/имя/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/пароль/i)).toBeInTheDocument()
  })

  it('показывает кнопку «Создать аккаунт»', () => {
    renderRegister()
    expect(screen.getByRole('button', { name: /Создать аккаунт/i })).toBeInTheDocument()
  })

  it('содержит ссылку на вход', () => {
    renderRegister()
    expect(screen.getByRole('link', { name: /Войти/i })).toBeInTheDocument()
  })
})

describe('Register — валидация', () => {
  beforeEach(() => vi.clearAllMocks())

  it('показывает ошибку при пустом имени', async () => {
    renderRegister()
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))
    await waitFor(() => expect(screen.getByText(/введите имя/i)).toBeInTheDocument())
  })

  it('показывает ошибку при пустом email', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText(/имя/i), 'Иван')
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))
    await waitFor(() => expect(screen.getByText(/введите email/i)).toBeInTheDocument())
  })

  it('показывает ошибку при пароле < 8 символов', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText(/имя/i), 'Иван')
    await userEvent.type(screen.getByLabelText(/email/i), 'ivan@example.com')
    await userEvent.type(screen.getByLabelText(/пароль/i), 'short')
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))
    await waitFor(() => expect(screen.getByText(/минимум 8/i)).toBeInTheDocument())
  })

  it('не вызывает register при невалидных данных', async () => {
    renderRegister()
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))
    await waitFor(() => screen.getByText(/введите имя/i))
    expect(mockRegister).not.toHaveBeenCalled()
  })

  it('требует обязательное согласие с юридическими документами', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText(/имя/i), 'Иван')
    await userEvent.type(screen.getByLabelText(/email/i), 'ivan@example.com')
    await userEvent.type(screen.getByLabelText(/пароль/i), 'ValidPass1')
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))

    await waitFor(() => expect(screen.getByText(/подтвердите согласие/i)).toBeInTheDocument())
    expect(mockRegister).not.toHaveBeenCalled()
  })
})

describe('Register — успешная регистрация', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRegister.mockResolvedValue({})
    mockLogin.mockResolvedValue({ id: 'u1', email: 'ivan@example.com', displayName: 'Иван', status: 'active', createdAt: '' })
  })

  it('вызывает register и показывает экран подтверждения email', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText(/имя/i), 'Иван')
    await userEvent.type(screen.getByLabelText(/email/i), 'ivan@example.com')
    await userEvent.type(screen.getByLabelText(/пароль/i), 'ValidPass1')
    await userEvent.click(screen.getByRole('checkbox', { name: /я принимаю/i }))
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))

    await waitFor(() => expect(mockRegister).toHaveBeenCalledWith('ivan@example.com', 'ValidPass1', 'Иван'))
    expect(mockLogin).not.toHaveBeenCalled()
    expect(mockNavigate).not.toHaveBeenCalled()
    expect(await screen.findByText(/проверьте почту/i)).toBeInTheDocument()
    expect(screen.getByText('ivan@example.com')).toBeInTheDocument()
  })
})

describe('Register — ошибка регистрации', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRegister.mockRejectedValue(new Error('email_taken'))
  })

  it('показывает toast с ошибкой', async () => {
    renderRegister()
    await userEvent.type(screen.getByLabelText(/имя/i), 'Иван')
    await userEvent.type(screen.getByLabelText(/email/i), 'ivan@example.com')
    await userEvent.type(screen.getByLabelText(/пароль/i), 'ValidPass1')
    await userEvent.click(screen.getByRole('checkbox', { name: /я принимаю/i }))
    fireEvent.click(screen.getByRole('button', { name: /Создать аккаунт/i }))

    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith('email_taken'))
    expect(mockNavigate).not.toHaveBeenCalled()
  })
})

describe('Register — социальные кнопки', () => {
  const originalLocation = window.location
  let assignedHref: string

  beforeEach(() => {
    vi.clearAllMocks()
    assignedHref = ''
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

  it('клик VK → редирект на /api/auth/oauth/vk/start', () => {
    renderRegister()
    fireEvent.click(screen.getByRole('button', { name: /VK ID/i }))
    expect(assignedHref).toBe('/api/auth/oauth/vk/start')
  })

  it('клик Яндекс → редирект на /api/auth/oauth/yandex/start', () => {
    renderRegister()
    fireEvent.click(screen.getByRole('button', { name: /Яндекс/i }))
    expect(assignedHref).toBe('/api/auth/oauth/yandex/start')
  })
})
