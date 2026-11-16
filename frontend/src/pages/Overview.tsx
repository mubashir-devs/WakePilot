import { Link } from 'react-router-dom'
import { ReachabilityBadge } from '../components/StatusBadges'
import { Card, ErrorBlock, EmptyState, LoadingBlock, PageHeader, formatTime, Button } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { api } from '../services/api'

function Stat({ label, value, hint, tone }: { label: string; value: number; hint?: string; tone: string }) {
  return (
    <Card className="p-5">
      <p className="text-sm font-medium text-slate-500">{label}</p>
      <p className={`mt-2 text-3xl font-semibold tabular-nums ${tone}`}>{value}</p>
      {hint && <p className="mt-1 text-xs text-slate-400">{hint}</p>}
    </Card>
  )
}

export default function Overview() {
  const { data, error, loading, reload } = useAsync(api.overview, [], 10_000)

  return (
    <>
      <PageHeader title="Overview" subtitle="Your devices and recent wake activity." />
      {loading && !data ? <LoadingBlock /> : error && !data ? <ErrorBlock error={error} onRetry={reload} /> : data && (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <Stat label="Registered devices" value={data.totalDevices} tone="text-slate-900" />
            <Stat label="Reachable" value={data.reachableDevices} tone="text-emerald-600" hint="Answered a network probe" />
            <Stat label="No response" value={data.noResponseDevices} tone="text-amber-600" hint="Off, asleep, blocked, or unchecked" />
            <Stat label="Wake requests" value={data.totalWakeRequests} tone="text-indigo-600" hint="Packets sent, all time" />
          </div>

          <Card className="mt-6">
            <div className="flex items-center justify-between border-b border-slate-100 px-5 py-4">
              <h2 className="text-sm font-semibold">Recent activity</h2>
              <Link to="/history" className="text-sm font-medium text-indigo-600 hover:text-indigo-500">View all</Link>
            </div>
            {data.recentActivity.length === 0 ? (
              <EmptyState title="No wake requests yet" body="Wake a device from the Devices page and it will show up here."
                action={<Link to="/devices"><Button variant="primary">Go to devices</Button></Link>} />
            ) : (
              <ul className="divide-y divide-slate-100">
                {data.recentActivity.map((r) => (
                  <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 px-5 py-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{r.deviceName}</p>
                      <p className="text-xs text-slate-500">{formatTime(r.requestedAt)}</p>
                    </div>
                    <div className="flex items-center gap-2">
                      {r.status === 'failed'
                        ? <span className="rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-700 ring-1 ring-inset ring-red-600/20" title={r.error}>Failed</span>
                        : <><span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 ring-1 ring-inset ring-slate-500/20">Packet sent</span><ReachabilityBadge value={r.reachability} /></>}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      )}
    </>
  )
}
