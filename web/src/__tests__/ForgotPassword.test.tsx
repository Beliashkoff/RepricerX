import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

const mockForgotPassword = vi.fn()
const mockToastError = vi.fn()

vi.mock('../api/auth', () => ({
  authApi: {
    forgotPassword: (...a: unknown[]) => mockForgotPassword(...a),
  },
}))

vi.mock('sonner', () => ({
  toast: { error: (...a: unknown[]) => mockToastError(...a) },
  Toaster: () => null,
}))

const { default: ForgotPassword } = await import('../pages/ForgotPassword')

function renderForgotPassword() {
  return render(<MemoryRouter><ForgotPassword /></MemoryRouter>)
}

describe('ForgotPassword — отрисовка', () => {
  it('показывает email и кнопку отправки', () => {
    renderForgotPassword()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Отправить ссылку/i })).toBeInTheDocument()
  })

  it('содержит ссылку на вход', () => {
    renderForgotPassword()
    expect(screen.getByRole('link', { name: /Войти/i })).toHaveAttribute('href', '/login')
  })
})

describe('ForgotPassword — валидация', () => {
  beforeEach(() => vi.clearAllMocks())

  it('показывает ошибку при пустом email', async () => {
    const user = userEvent.setup()
    renderForgotPassword()
    await user.click(screen.getByRole('button', { name: /Отправить ссылку/i }))
    expect(screen.getByText('Введите email')).toBeInTheDocument()
    expect(mockForgotPassword).not.toHaveBeenCalled()
  })

  it('показывает ошибку при невалидном email', async () => {
    const user = userEvent.setup()
    renderForgotPassword()
    await user.type(screen.getByLabelText(/email/i), 'bad-email')
    await user.click(screen.getByRole('button', { name: /Отправить ссылку/i }))
    expect(screen.getByText('Некорректный email')).toBeInTheDocument()
    expect(mockForgotPassword).not.toHaveBeenCalled()
  })
})

describe('ForgotPassword — отправка', () => {
  beforeEach(() => vi.clearAllMocks())

  it('вызывает API и показывает generic success', async () => {
    const user = userEvent.setup()
    mockForgotPassword.mockResolvedValue({})

    renderForgotPassword()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.click(screen.getByRole('button', { name: /Отправить ссылку/i }))

    await waitFor(() => expect(mockForgotPassword).toHaveBeenCalledWith('user@example.com'))
    expect(screen.getByText('Проверьте почту')).toBeInTheDocument()
    expect(screen.getByText(/Если аккаунт с таким email существует/i)).toBeInTheDocument()
  })

  it('показывает toast при ошибке API', async () => {
    const user = userEvent.setup()
    mockForgotPassword.mockRejectedValue(new Error('network'))

    renderForgotPassword()
    await user.type(screen.getByLabelText(/email/i), 'user@example.com')
    await user.click(screen.getByRole('button', { name: /Отправить ссылку/i }))

    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith('network'))
    expect(screen.queryByText('Проверьте почту')).not.toBeInTheDocument()
  })
})
