import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Alert, Card, Spinner } from '@heroui/react'
import { ApiError } from '../lib/api'
import { formatDateInput, formatVnd } from '../lib/format'
import { fetchOverview } from '../features/booking/api'

function monthRange(date: string): { from: string; to: string } {
  const [year, month] = date.split('-').map(Number)
  const from = `${year}-${String(month).padStart(2, '0')}-01`
  const lastDay = new Date(year, month, 0).getDate()
  const to = `${year}-${String(month).padStart(2, '0')}-${String(lastDay).padStart(2, '0')}`
  return { from, to }
}

export default function AdminOverviewPage() {
  const [month, setMonth] = useState(formatDateInput(new Date()).slice(0, 7))
  const range = monthRange(`${month}-01`)

  const overviewQuery = useQuery({
    queryKey: ['admin', 'overview', range.from, range.to],
    queryFn: () => fetchOverview(range.from, range.to),
  })

  const error = overviewQuery.error instanceof ApiError ? overviewQuery.error.message : null

  return (
    <div className="grid gap-6">
      <div>
        <h1 className="text-3xl font-semibold text-emerald-950">Doanh thu</h1>
        <p className="mt-2 text-emerald-800/80">Tổng tiền đã thu và số booking theo tháng.</p>
      </div>

      <label className="grid max-w-xs gap-1 text-sm">
        <span className="font-medium text-emerald-950">Tháng</span>
        <input
          type="month"
          value={month}
          onChange={(e) => setMonth(e.target.value)}
          className="rounded-lg border border-emerald-200 px-3 py-2"
        />
      </label>

      {error && (
        <Alert status="danger">
          <Alert.Content>
            <Alert.Description>{error}</Alert.Description>
          </Alert.Content>
        </Alert>
      )}

      {overviewQuery.isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : overviewQuery.data ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <Card className="border border-emerald-100 bg-white">
            <Card.Header>
              <Card.Title>Doanh thu</Card.Title>
              <Card.Description>Tiền thu thực tế trong kỳ</Card.Description>
            </Card.Header>
            <Card.Content>
              <p className="text-3xl font-semibold text-emerald-950">
                {formatVnd(overviewQuery.data.total_revenue_vnd)}
              </p>
            </Card.Content>
          </Card>
          <Card className="border border-emerald-100 bg-white">
            <Card.Header>
              <Card.Title>Số booking</Card.Title>
              <Card.Description>Theo ngày nhận phòng</Card.Description>
            </Card.Header>
            <Card.Content>
              <p className="text-3xl font-semibold text-emerald-950">{overviewQuery.data.booking_count}</p>
            </Card.Content>
          </Card>
        </div>
      ) : null}
    </div>
  )
}
