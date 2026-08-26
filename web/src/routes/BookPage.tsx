import { useSearchParams } from 'react-router-dom'
import BookingForm from '../features/booking/BookingForm'

export default function BookPage() {
  const [params] = useSearchParams()
  const homeId = Number(params.get('home') ?? '1')

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-3xl font-semibold text-emerald-950">Đặt phòng</h1>
        <p className="mt-2 text-emerald-800/80">
          Điền thông tin bên dưới. Sau khi tạo đặt phòng, bạn sẽ thấy mã VietQR để chuyển khoản.
        </p>
      </div>
      <BookingForm defaultHomeId={Number.isFinite(homeId) && homeId > 0 ? homeId : 1} />
    </div>
  )
}
