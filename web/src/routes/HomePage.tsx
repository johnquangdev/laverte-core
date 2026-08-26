import { Link } from 'react-router-dom'
import { Card } from '@heroui/react'

export default function HomePage() {
  return (
    <div className="grid gap-8">
      <section className="overflow-hidden rounded-3xl bg-gradient-to-br from-emerald-900 via-emerald-800 to-teal-700 px-6 py-12 text-emerald-50 shadow-lg">
        <p className="text-sm uppercase tracking-[0.2em] text-emerald-200/90">La Verte Home</p>
        <h1 className="mt-3 max-w-2xl text-4xl font-semibold leading-tight">
          Homestay xanh — đặt theo giờ, qua đêm hoặc cả ngày
        </h1>
        <p className="mt-4 max-w-xl text-emerald-100/90">
          Chọn khung giờ phù hợp, điền tên và số điện thoại, thanh toán VietQR để giữ chỗ ngay. Không cần tạo tài
          khoản.
        </p>
        <div className="mt-8 flex flex-wrap gap-3">
          <Link
            to="/book"
            className="inline-flex items-center justify-center rounded-xl bg-emerald-100 px-6 py-3 text-base font-semibold text-emerald-950 transition hover:bg-white"
          >
            Bắt đầu đặt phòng
          </Link>
          <Link
            to="/admin/login"
            className="inline-flex items-center justify-center rounded-xl border border-emerald-200/40 px-6 py-3 text-base font-semibold text-emerald-50 transition hover:bg-emerald-900/30"
          >
            Admin
          </Link>
        </div>
      </section>

      <section className="grid gap-4 md:grid-cols-3">
        {[
          {
            title: 'Theo giờ',
            body: 'Linh hoạt cho họp nhóm, chụp hình hoặc nghỉ ngắn trong ngày.',
          },
          {
            title: 'Qua đêm',
            body: 'Khung giá cố định cho lưu trú buổi tối — check-in/check-out rõ ràng.',
          },
          {
            title: 'Theo ngày',
            body: 'Thuê trọn ngày cho staycation hoặc làm việc tập trung.',
          },
        ].map((item) => (
          <Card key={item.title} className="border border-emerald-100 bg-white">
            <Card.Header>
              <Card.Title>{item.title}</Card.Title>
              <Card.Description>{item.body}</Card.Description>
            </Card.Header>
          </Card>
        ))}
      </section>
    </div>
  )
}
