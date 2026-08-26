export type BookingType = 'hourly' | 'overnight' | 'day'

export type BookingResponse = {
  id: number
  home_id: number
  customer_name: string
  customer_phone: string
  start_time: string
  end_time: string
  booking_type: BookingType
  computed_price: number
  status: string
  expires_at: string | null
  qr_content: string
}

export type CreateBookingInput = {
  home_id: number
  customer_name: string
  customer_phone: string
  start_time: string
  end_time: string
  booking_type: BookingType
}

export type HomeResponse = {
  id: number
  name: string
  category: string
  address: string
  description: string
  google_calendar_id: string
  is_active: boolean
}

export type AdminBookingResponse = BookingResponse & {
  created_at: string
  door_lock_code: string | null
  lock_code_sent_at: string | null
  google_calendar_event_id: string
  created_by_admin_id: number | null
}

export type OverviewResponse = {
  from: string
  to: string
  total_revenue_vnd: number
  booking_count: number
}
