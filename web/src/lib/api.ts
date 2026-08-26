export class ApiError extends Error {
  status: number
  code?: string

  constructor(status: number, message: string, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

type ApiErrorBody = {
  message?: string
  code?: string
  error?: string
}

const BASE_URL = import.meta.env.VITE_API_BASE_URL

async function parseBody(response: Response): Promise<unknown> {
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

function errorMessage(body: unknown, fallback: string): string {
  if (body && typeof body === 'object') {
    const record = body as ApiErrorBody
    return record.message ?? record.error ?? fallback
  }
  return fallback
}

export async function apiFetch<T>(path: string, init?: RequestInit, token?: string | null): Promise<T> {
  const headers = new Headers(init?.headers)
  if (!headers.has('Content-Type') && init?.body) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers,
  })

  const body = await parseBody(response)

  if (!response.ok) {
    const record = body && typeof body === 'object' ? (body as ApiErrorBody) : undefined
    throw new ApiError(response.status, errorMessage(body, 'request failed'), record?.code)
  }

  return body as T
}
