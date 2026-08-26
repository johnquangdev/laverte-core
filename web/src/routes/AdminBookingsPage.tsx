import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Spinner } from '@heroui/react'
import { ApiError } from '../lib/api'
import { bookingStatusLabel, formatDateInput, formatDateTime, formatVnd } from '../lib/format'
import { listBookings, listHomes } from '../features/booking/api'

export default function AdminBookingsPage() {
  const [date, setDate] = useState(formatDateInput(new Date()))
  const [homeId, setHomeId] = useState<number | undefined>(undefined)

  const homesQuery = useQuery({
    queryKey: ['admin', 'homes'],
    queryFn: listHomes,
  })

  const bookingsQuery = useQuery({
    queryKey: ['admin', 'bookings', date, homeId],
    queryFn: () => listBookings({ date, homeId }),
  })

  const homesById = useMemo(() => {
    const map = new Map<number, string>()
    for (const home of homesQuery.data ?? []) {
      map.set(home.id, home.name)
    }
    return map
  }, [homesQuery.data])

  const error =
    (homesQuery.error instanceof ApiError && homesQuery.error.message) ||
    (bookingsQuery.error instanceof ApiError && bookingsQuery.error.message) ||
    null

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-3xl font-semibold text-emerald-950">Lịch đặt phòng</h1>
        <p className="mt-2 text-emerald-800/80">Xem booking theo ngày và theo phòng.</p>
      </div>

      <div className="flex flex-wrap gap-3">
        <label className="grid gap-1 text-sm">
          <span className="font-medium text-emerald-950">Ngày</span>
          <input
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            className="rounded-lg border border-emerald-200 px-3 py-2"
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="font-medium text-emerald-950">Phòng</span>
          <select
            value={homeId ?? ''}
            onChange={(e) => setHomeId(e.target.value ? Number(e.target.value) : undefined)}
            className="rounded-lg border border-emerald-200 px-3 py-2"
          >
            <option value="">Tất cả</option>
            {(homesQuery.data ?? []).map((home) => (
              <option key={home.id} value={home.id}>
                {home.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      {error && (
        <Alert status="danger">
          <Alert.Content>
            <Alert.Description>{error}</Alert.Description>
          </Alert.Content>
        </Alert>
      )}

      {bookingsQuery.isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : (
        <div className="grid gap-3">
          {(bookingsQuery.data ?? []).length === 0 ? (
            <Card className="border border-emerald-100 bg-white">
              <Card.Content className="py-8 text-center text-emerald-800/70">Không có booking trong ngày này.</Card.Content>
            </Card>
          ) : (
            (bookingsQuery.data ?? []).map((booking) => (
              <Card key={booking.id} className="border border-emerald-100 bg-white">
                <Card.Header>
                  <Card.Title>
                    #{booking.id} · {booking.customer_name}
                  </Card.Title>
                  <Card.Description>
                    {homesById.get(booking.home_id) ?? `Home ${booking.home_id}`} ·{' '}
                    {bookingStatusLabel(booking.status)} · {formatVnd(booking.computed_price)}
                  </Card.Description>
                </Card.Header>
                <Card.Content className="text-sm text-emerald-900/80">
                  <p>
                    {formatDateTime(booking.start_time)} → {formatDateTime(booking.end_time)}
                  </p>
                  <p className="mt-1">{booking.customer_phone}</p>
                  {booking.door_lock_code ? (
                    <p className="mt-2 font-medium text-emerald-950">Mã cửa: {booking.door_lock_code}</p>
                  ) : null}
                </Card.Content>
              </Card>
            ))
          )}
        </div>
      )}
    </div>
  )
}
