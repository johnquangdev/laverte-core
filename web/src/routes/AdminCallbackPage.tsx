import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Alert, Card, Spinner } from '@heroui/react'
import { ApiError } from '../lib/api'
import { completeGoogleCallback } from '../features/auth/api'
import { useAuth } from '../features/auth/AuthProvider'

export default function AdminCallbackPage() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const { setSession } = useAuth()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const code = params.get('code')
    const state = params.get('state')
    if (!code || !state) {
      setError('Thiếu mã xác thực từ Google.')
      return
    }

    completeGoogleCallback(code, state)
      .then((session) => {
        setSession(session)
        navigate('/admin/bookings', { replace: true })
      })
      .catch((err) => {
        setError(err instanceof ApiError ? err.message : 'Đăng nhập thất bại.')
      })
  }, [params, navigate, setSession])

  return (
    <div className="flex min-h-screen items-center justify-center bg-[#071612] px-4">
      <Card className="w-full max-w-sm border border-emerald-900/40 bg-[#0a1f18] text-emerald-50">
        <Card.Header>
          <Card.Title>Đang xác thực…</Card.Title>
        </Card.Header>
        <Card.Content className="flex flex-col items-center gap-4 py-6">
          {!error ? <Spinner size="lg" /> : null}
          {error ? (
            <Alert status="danger">
              <Alert.Content>
                <Alert.Description>{error}</Alert.Description>
              </Alert.Content>
            </Alert>
          ) : null}
        </Card.Content>
      </Card>
    </div>
  )
}
