import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'

export default function VerifyEmail() {
  const [params] = useSearchParams()

  useEffect(() => {
    const token = params.get('token') ?? ''
    window.location.href = `/api/auth/verify?token=${encodeURIComponent(token)}`
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="flex items-center justify-center min-h-screen bg-[#f7f8fa]">
      <div className="w-8 h-8 border-2 border-[#ffcc00] border-t-transparent rounded-full animate-spin" />
    </div>
  )
}
