import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { act } from 'react'
import React from 'react'

// Мокируем api/auth ДО импорта контекста
const mockMe = vi.fn()
const mockLogin = vi.fn()
const mockLogout = vi.fn()

vi.mock('../api/auth', () => ({
  authApi: {
    me: (...args: unknown[]) => mockMe(...args),
    login: (...args: unknown[]) => mockLogin(...args),
    logout: (...args: unknown[]) => mockLogout(...args),
    register: vi.fn(),
    updateMe: vi.fn(),
  },
}))

// top-level await для динамического импорта после установки моков
const { AuthProvider, useAuth } = await import('../context/AuthContext')

const mockUser = {
  id: 'u1',
  email: 'test@example.com',
  displayName: 'Test',
  status: 'active',
  createdAt: '2026-01-01T00:00:00Z',
}

function TestConsumer() {
  const { user, isLoading } = useAuth()
  if (isLoading) return <div>loading</div>
  if (!user) return <div>no-user</div>
  return <div>user:{user.email}</div>
}

describe('AuthProvider — начальное состояние', () => {
  beforeEach(() => vi.clearAllMocks())

  it('показывает loading пока /me не завершён', () => {
    mockMe.mockReturnValue(new Promise(() => {}))
    render(<AuthProvider><TestConsumer /></AuthProvider>)
    expect(screen.getByText('loading')).toBeInTheDocument()
  })

  it('устанавливает user если /me успешен', async () => {
    mockMe.mockResolvedValue(mockUser)
    render(<AuthProvider><TestConsumer /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('user:test@example.com')).toBeInTheDocument())
  })

  it('user = null если /me возвращает 401', async () => {
    mockMe.mockRejectedValue(new Error('Unauthorized'))
    render(<AuthProvider><TestConsumer /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('no-user')).toBeInTheDocument())
  })
})

describe('useAuth().login', () => {
  beforeEach(() => vi.clearAllMocks())

  it('обновляет user после успешного логина', async () => {
    mockMe.mockRejectedValue(new Error('Unauthorized'))
    mockLogin.mockResolvedValue(mockUser)

    function LoginConsumer() {
      const { user, isLoading, login } = useAuth()
      if (isLoading) return <div>loading</div>
      return (
        <div>
          <div>{user ? `user:${user.email}` : 'no-user'}</div>
          <button onClick={() => { void login('test@example.com', 'pass') }}>login</button>
        </div>
      )
    }

    render(<AuthProvider><LoginConsumer /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('no-user')).toBeInTheDocument())

    await act(async () => { screen.getByText('login').click() })

    await waitFor(() => expect(screen.getByText('user:test@example.com')).toBeInTheDocument())
    expect(mockLogin).toHaveBeenCalledWith('test@example.com', 'pass')
  })

  it('пробрасывает ошибку логина', async () => {
    mockMe.mockRejectedValue(new Error('Unauthorized'))
    mockLogin.mockRejectedValue(new Error('invalid_credentials'))

    function ErrorConsumer() {
      const { login } = useAuth()
      const [err, setErr] = React.useState('')
      return (
        <div>
          <span data-testid="err">{err}</span>
          <button onClick={() => { login('x', 'y').catch(e => setErr((e as Error).message)) }}>login</button>
        </div>
      )
    }

    render(<AuthProvider><ErrorConsumer /></AuthProvider>)
    await act(async () => { screen.getByText('login').click() })
    await waitFor(() => expect(screen.getByTestId('err').textContent).toBe('invalid_credentials'))
  })
})

describe('useAuth().logout', () => {
  beforeEach(() => vi.clearAllMocks())

  it('сбрасывает user в null после logout', async () => {
    mockMe.mockResolvedValue(mockUser)
    mockLogout.mockResolvedValue(undefined)

    function LogoutConsumer() {
      const { user, isLoading, logout } = useAuth()
      if (isLoading) return <div>loading</div>
      return (
        <div>
          <div>{user ? 'logged-in' : 'no-user'}</div>
          <button onClick={() => { void logout() }}>logout</button>
        </div>
      )
    }

    render(<AuthProvider><LogoutConsumer /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('logged-in')).toBeInTheDocument())

    await act(async () => { screen.getByText('logout').click() })
    await waitFor(() => expect(screen.getByText('no-user')).toBeInTheDocument())
  })

  it('сбрасывает user даже если запрос logout упал', async () => {
    mockMe.mockResolvedValue(mockUser)
    mockLogout.mockRejectedValue(new Error('network'))

    function LogoutConsumer() {
      const { user, isLoading, logout } = useAuth()
      if (isLoading) return <div>loading</div>
      return (
        <div>
          <div>{user ? 'logged-in' : 'no-user'}</div>
          <button onClick={() => { void logout() }}>logout</button>
        </div>
      )
    }

    render(<AuthProvider><LogoutConsumer /></AuthProvider>)
    await waitFor(() => expect(screen.getByText('logged-in')).toBeInTheDocument())
    await act(async () => { screen.getByText('logout').click() })
    await waitFor(() => expect(screen.getByText('no-user')).toBeInTheDocument())
  })
})

describe('useAuth() возвращает полный контекст', () => {
  it('предоставляет все необходимые поля', async () => {
    mockMe.mockResolvedValue(mockUser)

    let capturedCtx: ReturnType<typeof useAuth> | null = null
    function Inspector() {
      capturedCtx = useAuth()
      return null
    }

    render(<AuthProvider><Inspector /></AuthProvider>)
    await waitFor(() => expect(mockMe).toHaveBeenCalled())

    expect(capturedCtx).not.toBeNull()
    expect(typeof capturedCtx!.login).toBe('function')
    expect(typeof capturedCtx!.logout).toBe('function')
    expect(typeof capturedCtx!.refreshMe).toBe('function')
    expect('user' in capturedCtx!).toBe(true)
    expect('isLoading' in capturedCtx!).toBe(true)
  })
})
