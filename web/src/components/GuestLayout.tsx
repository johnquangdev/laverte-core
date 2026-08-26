import { Link, Outlet } from 'react-router-dom'

export default function GuestLayout() {
  return (
    <div className="flex min-h-screen flex-col bg-[#f4faf7] text-[#0f172a]">
      <header className="sticky top-0 z-10 border-b border-emerald-100/80 bg-white/90 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center justify-between px-4 py-4">
          <Link to="/" className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-xl bg-emerald-800 text-sm font-bold text-emerald-50">
              LV
            </span>
            <div>
              <p className="text-lg font-semibold text-emerald-950">La Verte Home</p>
              <p className="text-xs text-emerald-700/80">Homestay theo giờ · qua đêm · theo ngày</p>
            </div>
          </Link>
          <Link
            to="/book"
            className="inline-flex items-center justify-center rounded-lg bg-emerald-700 px-4 py-2 text-sm font-medium text-white transition hover:bg-emerald-800"
          >
            Đặt phòng
          </Link>
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-4 py-8">
        <Outlet />
      </main>
      <footer className="border-t border-emerald-100 bg-white px-4 py-6 text-center text-sm text-emerald-800/70">
        © {new Date().getFullYear()} La Verte Home
      </footer>
    </div>
  )
}
