export type AvailabilityStatus = 'reachable' | 'unreachable' | 'unknown'

export interface Availability {
  status: AvailabilityStatus
  checkedAt: string | null
}

export interface Device {
  id: number
  name: string
  mac: string
  ip: string
  broadcast: string
  port: number
  interface: string
  description: string
  createdAt: string
  updatedAt: string
  availability: Availability
}

export interface DeviceInput {
  name: string
  mac: string
  ip: string
  broadcast: string
  port: number
  interface: string
  description: string
}

export type Reachability = 'pending' | 'reachable' | 'unknown' | 'not_checked'

export interface WakeRecord {
  id: number
  deviceId: number | null
  deviceName: string
  mac: string
  broadcast: string
  port: number
  interface: string
  status: 'sent' | 'failed'
  error: string
  reachability: Reachability
  reachabilityCheckedAt: string | null
  requestedAt: string
}

export interface WakeResult extends WakeRecord {
  packetSent: boolean
  message: string
}

export interface NetAddress {
  ip: string
  cidr: string
  netmask: string
  network: string
  broadcast: string
  prefixLen: number
}

export interface NetInterface {
  name: string
  index: number
  up: boolean
  loopback: boolean
  supportsBroadcast: boolean
  addresses: NetAddress[]
}

export interface InterfacesResponse {
  interfaces: NetInterface[]
  defaultInterface: string
}

export interface Overview {
  totalDevices: number
  reachableDevices: number
  noResponseDevices: number
  totalWakeRequests: number
  recentActivity: WakeRecord[]
}

export interface Settings {
  server: { host: string; port: number; loopbackOnly: boolean }
  auth: { enabled: boolean }
  wol: { defaultPort: number; defaultInterface: string; sendTimeout: string; verifyEnabled: boolean; verifyTimeout: string }
  monitor: { enabled: boolean; interval: string; ports: number[] }
  logging: { level: string; format: string }
  version: string
  configFile: string
}

export interface Health {
  status: string
  version: string
  authRequired: boolean
}
