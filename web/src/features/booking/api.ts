import { apiFetch } from '../../lib/api'
import { getAccessToken } from '../../lib/authStorage'
import type {
  AdminBookingResponse,
  BookingResponse,
  CreateBookingInput,
  HomeResponse,
  OverviewResponse,
} from './types'

export async function createGuestBooking(input: CreateBookingInput): Promise<BookingResponse> {
  return apiFetch<BookingResponse>('/api/v1/bookings', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export async function listHomes(): Promise<HomeResponse[]> {
  return apiFetch<HomeResponse[]>('/api/v1/admin/homes', undefined, getAccessToken())
}

export async function listBookings(params: {
  homeId?: number
  date?: string
}): Promise<AdminBookingResponse[]> {
  const search = new URLSearchParams()
  if (params.homeId) search.set('home_id', String(params.homeId))
  if (params.date) search.set('date', params.date)
  const suffix = search.toString() ? `?${search.toString()}` : ''
  return apiFetch<AdminBookingResponse[]>(`/api/v1/admin/bookings${suffix}`, undefined, getAccessToken())
}

export async function fetchOverview(from: string, to: string): Promise<OverviewResponse> {
  const search = new URLSearchParams({ from, to })
  return apiFetch<OverviewResponse>(`/api/v1/admin/overview?${search.toString()}`, undefined, getAccessToken())
}
