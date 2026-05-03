import apiClient from './client'

export interface User {
  id: string
  email: string
  displayName: string
  status: string
  createdAt: string
}

export const authApi = {
  me: () => apiClient.get<User>('/auth/me').then(r => r.data),
  login: (email: string, password: string) =>
    apiClient.post<User>('/auth/login', { email, password }).then(r => r.data),
  register: (email: string, password: string, displayName: string) =>
    apiClient.post('/auth/register', { email, password, displayName }),
  forgotPassword: (email: string) =>
    apiClient.post('/auth/password/forgot', { email }),
  resetPassword: (token: string, password: string, passwordConfirmation: string) =>
    apiClient.post('/auth/password/reset', { token, password, passwordConfirmation }),
  logout: () => apiClient.post('/auth/logout'),
  updateMe: (displayName: string) =>
    apiClient.patch<User>('/auth/me', { displayName }).then(r => r.data),
}
