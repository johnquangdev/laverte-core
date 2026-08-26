import { Button, Card } from '@heroui/react'
import { bookingStatusLabel, formatDateTime, formatVnd } from '../../lib/format'
import type { BookingResponse } from './types'

type Props = {
  booking: BookingResponse
  onReset: () => void
}

export default function PaymentPanel({ booking, onReset }: Props) {
  return (
    <Card className="border border-emerald-100 bg-white shadow-sm">
      <Card.Header>
        <Card.Title>Thanh toán VietQR</Card.Title>
        <Card.Description>
          Mã đặt #{booking.id} · {bookingStatusLabel(booking.status)} · {formatVnd(booking.computed_price)}
        </Card.Description>
      </Card.Header>
      <Card.Content className="grid gap-5">
        <div className="rounded-xl border border-emerald-100 bg-emerald-50/60 p-4 text-sm text-emerald-900">
          <p>
            <span className="font-medium">Khách:</span> {booking.customer_name} ({booking.customer_phone})
          </p>
          <p className="mt-1">
            <span className="font-medium">Thời gian:</span> {formatDateTime(booking.start_time)} →{' '}
            {formatDateTime(booking.end_time)}
          </p>
          {booking.expires_at && (
            <p className="mt-1">
              <span className="font-medium">Hết hạn giữ chỗ:</span> {formatDateTime(booking.expires_at)}
            </p>
          )}
        </div>

        {booking.qr_content ? (
          <div className="flex flex-col items-center gap-3">
            <img
              src={booking.qr_content}
              alt="Mã QR thanh toán VietQR"
              className="max-h-72 rounded-xl border border-emerald-100 bg-white p-3"
            />
            <p className="text-center text-sm text-emerald-800/80">
              Quét mã để chuyển khoản đúng số tiền. Hệ thống tự xác nhận sau vài giây.
            </p>
          </div>
        ) : (
          <p className="text-sm text-emerald-800/80">Chưa có mã QR — liên hệ admin nếu lỗi kéo dài.</p>
        )}

        <Button type="button" variant="secondary" fullWidth onPress={onReset}>
          Đặt phòng khác
        </Button>
      </Card.Content>
    </Card>
  )
}
