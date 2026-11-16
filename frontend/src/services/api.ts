import type {
  Device, DeviceInput, Health, InterfacesResponse, Overview, Settings, WakeRecord, WakeResult,
} from '../types'

const BASE = '/api/v1'
const TOKEN_KEY = 'wakepilot.token'

export const auth = {
  get: () => sessionStorage.getItem(TOKEN_KEY) ?? '',
  set: (t: string) => sessionStorage.setItem(TOKEN_KEY, t),
  clear: () => sessionStorage.removeItem(TOKEN_KEY),
}

export const UNAUTHORIZED_EVENT = 'wakepilot:unauthorized'

export class ApiError extends Error {
  status: number
  code: string
  details: Record<string, string>
  constructor(status: number, code: string, message: string, details: Record<string, string> = {}) {
    super(message)
    this.status = status
    this.code = code
    this.details = details
  }
}

interface Envelope<T> {
  data?: T
  meta?: { total: number; limit: number; offset: number }
  error?: { code: string; message: string; details?: Record<string, string> }
}

async function raw<T>(method: string, path: string, body?: unknown): Promise<Envelope<T>> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const token = auth.get()
  if (token) headers['Authorization'] = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(BASE + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) })
  } catch {
    throw new ApiError(0, 'network_error', 'Cannot reach the WakePilot server.')
  }
  if (res.status === 204) return {}
  let env: Envelope<T> = {}
  try {
    env = await res.json()
  } catch {
    /* non-JSON body */
  }
  if (!res.ok) {
    if (res.status === 401) window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
    throw new ApiError(res.status, env.error?.code ?? 'error', env.error?.message ?? `Request failed (${res.status})`, env.error?.details)
  }
  return env
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  return (await raw<T>(method, path, body)).data as T
}

export const api = {
  health: () => request<Health>('GET', '/health'),
  overview: () => request<Overview>('GET', '/overview'),
  settings: () => request<Settings>('GET', '/settings'),
  interfaces: () => request<InterfacesResponse>('GET', '/network/interfaces'),
  devices: () => request<Device[]>('GET', '/devices'),
  device: (id: number) => request<Device>('GET', `/devices/${id}`),
  createDevice: (d: DeviceInput) => request<Device>('POST', '/devices', d),
  updateDevice: (id: number, d: DeviceInput) => request<Device>('PUT', `/devices/${id}`, d),
  deleteDevice: (id: number) => request<void>('DELETE', `/devices/${id}`),
  wake: (id: number) => request<WakeResult>('POST', `/devices/${id}/wake`),
  check: (id: number) => request<Device>('POST', `/devices/${id}/check`),
  history: async (limit: number, offset: number) => {
    const env = await raw<WakeRecord[]>('GET', `/wake-history?limit=${limit}&offset=${offset}`)
    return { records: env.data ?? [], total: env.meta?.total ?? 0 }
  },
}
