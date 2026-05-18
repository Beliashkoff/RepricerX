import axios from 'axios'

const apiClient = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true,
})

apiClient.interceptors.response.use(
  (r) => r,
  (err) => {
    const status = err.response?.status
    const url = String(err.config?.url ?? '')
    if (status === 401 && !url.includes('/auth/me')) {
      window.dispatchEvent(new CustomEvent('rx:unauthorized'))
    }
    const code = err.response?.data?.error?.code
    const msg = err.response?.data?.error?.message || err.response?.data?.message || err.message || 'Ошибка запроса'
    const e = new Error(msg) as Error & { code?: string }
    if (code) e.code = code
    return Promise.reject(e)
  }
)

export default apiClient
