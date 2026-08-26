import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { Button } from '@heroui/react'
import { useAuth } from '../features/auth/AuthProvider'

const NAV_ITEMS = [
  { to: '/admin/bookings', label: 'Lịch đặt phòng' },
  { to: '/admin/overview', label: 'Doanh thu' },
]

export default function AdminShell() {
  const { session, logout } = useAuth()
  const navigate = useNavigate()

  async function handleLogout() {
    await logout()
    navigate('/admin/login')
  }

  return (
    <div className="flex h-screen bg-[#071612] text-[#d7f5e8]">
      <aside className="flex w-64 shrink-0 flex-col gap-1 border-r border-emerald-900/60 bg-[#0a1f18] p-4">
        <div className="mb-6 px-2 pt-[env(safe-area-inset-top)]">
          <p className="text-xl font-semibold text-emerald-50">La Verte</p>
          <p className="text-xs text-emerald-300/70">Admin</p>
          {session?.user.email && (
            <p className="mt-2 truncate text-xs text-emerald-200/60">{session.user.email}</p>
          )}
        </div>
        <nav className="flex flex-col gap-1">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-2.5 text-sm font-medium transition-colors ${
                  isActive ? 'bg-emerald-500 text-emerald-950' : 'text-emerald-200/80 hover:bg-emerald-900/50'
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="mt-auto px-2 pb-[env(safe-area-inset-bottom)]">
          <Button type="button" variant="secondary" fullWidth onPress={handleLogout}>
            Đăng xuất
          </Button>
        </div>
      </aside>
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto bg-[#f8fffb] text-[#0f172a]">
        <div className="mx-auto max-w-6xl p-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
