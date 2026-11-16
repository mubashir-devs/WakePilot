import type { AvailabilityStatus, Reachability } from '../types'
import { Badge } from './ui'

const dot = (c: string) => <span className={`h-1.5 w-1.5 rounded-full ${c}`} />

// "No response" is deliberately not "Offline": a silent host may be asleep,
// firewalled or simply off. We cannot tell.
export function AvailabilityBadge({ status }: { status: AvailabilityStatus }) {
  switch (status) {
    case 'reachable':
      return <Badge tone="green" title="The device answered a network probe.">{dot('bg-emerald-500')}Reachable</Badge>
    case 'unreachable':
      return <Badge tone="amber" title="No probe was answered. The device may be off, asleep, or blocking probes.">{dot('bg-amber-500')}No response</Badge>
    default:
      return <Badge tone="slate" title="Not checked yet, or no IP address configured.">{dot('bg-slate-400')}Unknown</Badge>
  }
}

export function ReachabilityBadge({ value }: { value: Reachability }) {
  switch (value) {
    case 'pending':
      return <Badge tone="indigo" title="Waiting to see whether the device responds.">{dot('bg-indigo-500 animate-pulse')}Checking…</Badge>
    case 'reachable':
      return <Badge tone="green" title="The device answered after the packet was sent.">{dot('bg-emerald-500')}Reachable</Badge>
    case 'unknown':
      return <Badge tone="amber" title="No answer within the check window. The device may still be booting or blocking probes.">{dot('bg-amber-500')}Not confirmed</Badge>
    default:
      return <Badge tone="slate" title="No IP address configured or verification disabled.">Not checked</Badge>
  }
}
