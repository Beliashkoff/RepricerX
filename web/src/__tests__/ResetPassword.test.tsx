import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'

const mockResetPassword = vi.fn()

vi.mock('../api/auth', () => ({
  authApi: {
    resetPassword: (...a: unknown[]) => mockResetPassword(...a),
  },
}))

const { default: ResetPassword } = await import('../pages/ResetPassword')

function renderResetPassword(url = '/reset-password#token=reset-token') {
  window.history.pushState({}, '', url)
  return render(<MemoryRouter><ResetPassword /></MemoryRouter>)
}

describe('ResetPassword — token state', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => {
    window.history.pushState({}, '', '/')
  })

  it('читает token из fragment и очищает address bar', () => {
    renderResetPassword('/reset-password#token=secret-token')
    expect(window.location.hash).toBe('')
    expect(screen.getByLabelText(/Новый пароль/i)).toBeInTheDocument()
  })

  it('не принимает query token и убирает его из address bar', () => {
    renderResetPassword('/reset-password?token=query-token&next=login')
    expect(window.location.search).toBe('?next=login')
    expect(screen.getByText('Ссылка недействительна')).toBeInTheDocument()
  })

  it('показывает invalid-token без token', () => {
    renderResetPassword('/reset-password')
    expect(screen.getByText('Ссылка недействительна')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Запросить ссылку/i })).toHaveAttribute('href', '/forgot-password')
  })
})

describe('ResetPassword — валидация', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => {
    window.history.pushState({}, '', '/')
  })

  it('показывает ошибку при пустом пароле', async () => {
    const user = userEvent.setup()
    renderResetPassword()
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))
    expect(screen.getByText('Введите новый пароль')).toBeInTheDocument()
    expect(mockResetPassword).not.toHaveBeenCalled()
  })

  it('проверяет требования backend password basics', async () => {
    const user = userEvent.setup()
    renderResetPassword()
    await user.type(screen.getByLabelText(/Новый пароль/i), 'short1')
    await user.type(screen.getByLabelText(/Повторите пароль/i), 'short1')
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))
    expect(screen.getAllByText(/Пароль должен быть от 12 до 128 символов/i).length).toBeGreaterThan(0)
    expect(mockResetPassword).not.toHaveBeenCalled()
  })

  it('показывает ошибку если пароли не совпадают', async () => {
    const user = userEvent.setup()
    renderResetPassword()
    await user.type(screen.getByLabelText(/Новый пароль/i), 'ValidPassword123')
    await user.type(screen.getByLabelText(/Повторите пароль/i), 'ValidPassword456')
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))
    expect(screen.getByText('Пароли не совпадают')).toBeInTheDocument()
    expect(mockResetPassword).not.toHaveBeenCalled()
  })
})

describe('ResetPassword — отправка', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => {
    window.history.pushState({}, '', '/')
  })

  it('вызывает API с token и показывает success', async () => {
    const user = userEvent.setup()
    mockResetPassword.mockResolvedValue({})

    renderResetPassword('/reset-password#token=abc123')
    await user.type(screen.getByLabelText(/Новый пароль/i), 'ValidPassword123')
    await user.type(screen.getByLabelText(/Повторите пароль/i), 'ValidPassword123')
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))

    await waitFor(() => expect(mockResetPassword).toHaveBeenCalledWith('abc123', 'ValidPassword123', 'ValidPassword123'))
    expect(screen.getByText('Пароль обновлён')).toBeInTheDocument()
  })

  it('показывает error state при обычной ошибке API', async () => {
    const user = userEvent.setup()
    mockResetPassword.mockRejectedValue(new Error('network'))

    renderResetPassword()
    await user.type(screen.getByLabelText(/Новый пароль/i), 'ValidPassword123')
    await user.type(screen.getByLabelText(/Повторите пароль/i), 'ValidPassword123')
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))

    await waitFor(() => expect(screen.getByText('network')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /Обновить пароль/i })).toBeInTheDocument()
  })

  it('показывает invalid-token state если API отверг token', async () => {
    const user = userEvent.setup()
    mockResetPassword.mockRejectedValue(new Error('invalid_token'))

    renderResetPassword()
    await user.type(screen.getByLabelText(/Новый пароль/i), 'ValidPassword123')
    await user.type(screen.getByLabelText(/Повторите пароль/i), 'ValidPassword123')
    await user.click(screen.getByRole('button', { name: /Обновить пароль/i }))

    await waitFor(() => expect(screen.getByText('Ссылка недействительна')).toBeInTheDocument())
  })
})
