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
    const msg = err.response?.data?.error?.message || err.response?.data?.message || err.message || 'Ошибка запроса'
    return Promise.reject(new Error(msg))
  }
)

export default apiClient
