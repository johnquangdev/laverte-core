import { useState, type FormEvent } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Alert, Button, Card, Input, Label, TextField } from '@heroui/react'
import { ApiError } from '../../lib/api'
import { formatDateInput, toIsoInHoChiMinh } from '../../lib/format'
import { createGuestBooking } from './api'
import type { BookingResponse, BookingType } from './types'
import PaymentPanel from './PaymentPanel'

const BOOKING_TYPES: { value: BookingType; label: string }[] = [
  { value: 'hourly', label: 'Theo giờ' },
  { value: 'overnight', label: 'Qua đêm' },
  { value: 'day', label: 'Theo ngày' },
]

type Props = {
  defaultHomeId?: number
}

export default function BookingForm({ defaultHomeId = 1 }: Props) {
  const today = formatDateInput(new Date())
  const [homeId, setHomeId] = useState(String(defaultHomeId))
  const [customerName, setCustomerName] = useState('')
  const [customerPhone, setCustomerPhone] = useState('')
  const [bookingType, setBookingType] = useState<BookingType>('hourly')
  const [startDate, setStartDate] = useState(today)
  const [startTime, setStartTime] = useState('14:00')
  const [endDate, setEndDate] = useState(today)
  const [endTime, setEndTime] = useState('18:00')
  const [result, setResult] = useState<BookingResponse | null>(null)

  const createBooking = useMutation({
    mutationFn: createGuestBooking,
    onSuccess: (booking) => setResult(booking),
  })

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    await createBooking.mutateAsync({
      home_id: Number(homeId),
      customer_name: customerName.trim(),
      customer_phone: customerPhone.trim(),
      booking_type: bookingType,
      start_time: toIsoInHoChiMinh(startDate, startTime),
      end_time: toIsoInHoChiMinh(endDate, endTime),
    })
  }

  const errorMessage = createBooking.error instanceof ApiError ? createBooking.error.message : null

  if (result) {
    return <PaymentPanel booking={result} onReset={() => setResult(null)} />
  }

  return (
    <Card className="border border-emerald-100 bg-white shadow-sm">
      <Card.Header>
        <Card.Title>Thông tin đặt phòng</Card.Title>
        <Card.Description>Không cần tài khoản — thanh toán VietQR để giữ chỗ trong 15 phút.</Card.Description>
      </Card.Header>
      <Card.Content>
        <form onSubmit={handleSubmit} className="grid gap-5">
          <label className="grid gap-2 text-sm">
            <span className="font-medium text-emerald-950">Mã phòng (home_id)</span>
            <input
              type="number"
              min={1}
              required
              value={homeId}
              onChange={(e) => setHomeId(e.target.value)}
              className="rounded-lg border border-emerald-200 px-3 py-2.5 outline-none focus:border-emerald-500 focus:ring-2 focus:ring-emerald-200"
            />
          </label>

          <TextField name="customer_name" value={customerName} onChange={setCustomerName} isRequired>
            <Label>Họ tên</Label>
            <Input autoComplete="name" />
          </TextField>

          <TextField name="customer_phone" value={customerPhone} onChange={setCustomerPhone} isRequired>
            <Label>Số điện thoại</Label>
            <Input autoComplete="tel" inputMode="tel" />
          </TextField>

          <label className="grid gap-2 text-sm">
            <span className="font-medium text-emerald-950">Loại đặt</span>
            <select
              value={bookingType}
              onChange={(e) => setBookingType(e.target.value as BookingType)}
              className="rounded-lg border border-emerald-200 px-3 py-2.5 outline-none focus:border-emerald-500 focus:ring-2 focus:ring-emerald-200"
            >
              {BOOKING_TYPES.map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </label>

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="grid gap-2 text-sm">
              <span className="font-medium text-emerald-950">Nhận phòng</span>
              <input
                type="date"
                required
                value={startDate}
                onChange={(e) => setStartDate(e.target.value)}
                className="rounded-lg border border-emerald-200 px-3 py-2.5"
              />
              <input
                type="time"
                required
                value={startTime}
                onChange={(e) => setStartTime(e.target.value)}
                className="rounded-lg border border-emerald-200 px-3 py-2.5"
              />
            </label>
            <label className="grid gap-2 text-sm">
              <span className="font-medium text-emerald-950">Trả phòng</span>
              <input
                type="date"
                required
                value={endDate}
                onChange={(e) => setEndDate(e.target.value)}
                className="rounded-lg border border-emerald-200 px-3 py-2.5"
              />
              <input
                type="time"
                required
                value={endTime}
                onChange={(e) => setEndTime(e.target.value)}
                className="rounded-lg border border-emerald-200 px-3 py-2.5"
              />
            </label>
          </div>

          {errorMessage && (
            <Alert status="danger">
              <Alert.Content>
                <Alert.Description>{errorMessage}</Alert.Description>
              </Alert.Content>
            </Alert>
          )}

          <Button type="submit" fullWidth size="lg" isDisabled={createBooking.isPending}>
            {createBooking.isPending ? 'Đang tạo đặt phòng…' : 'Tiếp tục thanh toán'}
          </Button>
        </form>
      </Card.Content>
    </Card>
  )
}
