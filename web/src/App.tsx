import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import AdminShell from './components/AdminShell'
import GuestLayout from './components/GuestLayout'
import RequireAdmin from './components/RequireAdmin'
import { AuthProvider } from './features/auth/AuthProvider'
import AdminBookingsPage from './routes/AdminBookingsPage'
import AdminCallbackPage from './routes/AdminCallbackPage'
import AdminLoginPage from './routes/AdminLoginPage'
import AdminOverviewPage from './routes/AdminOverviewPage'
import BookPage from './routes/BookPage'
import HomePage from './routes/HomePage'

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          <Route element={<GuestLayout />}>
            <Route path="/" element={<HomePage />} />
            <Route path="/book" element={<BookPage />} />
          </Route>

          <Route path="/admin/login" element={<AdminLoginPage />} />
          <Route path="/admin/auth/callback" element={<AdminCallbackPage />} />

          <Route
            path="/admin"
            element={
              <RequireAdmin>
                <AdminShell />
              </RequireAdmin>
            }
          >
            <Route index element={<Navigate to="/admin/bookings" replace />} />
            <Route path="bookings" element={<AdminBookingsPage />} />
            <Route path="overview" element={<AdminOverviewPage />} />
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
