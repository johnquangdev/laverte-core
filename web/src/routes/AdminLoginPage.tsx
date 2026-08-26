import { Alert, Button, Card } from '@heroui/react'
import { ApiError } from '../lib/api'
import { useGoogleLogin } from '../features/auth/useGoogleLogin'

export default function AdminLoginPage() {
  const login = useGoogleLogin()
  const errorMessage = login.error instanceof ApiError ? login.error.message : null

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-[#071612] px-4">
      <Card className="w-full max-w-sm border border-emerald-900/40 bg-[#0a1f18] text-emerald-50">
        <Card.Header>
          <Card.Title>La Verte Admin</Card.Title>
          <Card.Description className="text-emerald-200/70">Đăng nhập bằng Google để quản lý lịch và doanh thu.</Card.Description>
        </Card.Header>
        <Card.Content className="grid gap-4">
          {errorMessage && (
            <Alert status="danger">
              <Alert.Content>
                <Alert.Description>{errorMessage}</Alert.Description>
              </Alert.Content>
            </Alert>
          )}
          <Button type="button" fullWidth size="lg" isDisabled={login.isPending} onPress={() => login.mutate()}>
            {login.isPending ? 'Đang chuyển hướng…' : 'Đăng nhập với Google'}
          </Button>
        </Card.Content>
      </Card>
    </div>
  )
}
