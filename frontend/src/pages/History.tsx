import { useState } from 'react'
import { ReachabilityBadge } from '../components/StatusBadges'
import { Badge, Button, Card, EmptyState, ErrorBlock, LoadingBlock, PageHeader, formatTime } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { api } from '../services/api'

const PAGE = 20

export default function History() {
  const [offset, setOffset] = useState(0)
  const { data, error, loading, reload } = useAsync(() => api.history(PAGE, offset), [offset], 5_000)

  return (
    <>
      <PageHeader title="Wake history" subtitle="“Packet sent” means the packet left this machine, not that the device woke up." />
      <Card className="overflow-hidden">
        {loading && !data ? <LoadingBlock /> : error && !data ? <ErrorBlock error={error} onRetry={reload} /> : data && data.records.length === 0 ? (
          <EmptyState title="No wake requests yet" body="Wake attempts, successful or not, are recorded here." />
        ) : data && (
          <>
            <div className="overflow-x-auto">
              <table className="min-w-full text-left text-sm">
                <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                  <tr>{['Requested', 'Device', 'MAC address', 'Destination', 'Packet', 'Device status', 'Error'].map((h) => <th key={h} className="px-4 py-3 font-medium">{h}</th>)}</tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {data.records.map((r) => (
                    <tr key={r.id} className="align-top">
                      <td className="whitespace-nowrap px-4 py-3 text-xs text-slate-600">{formatTime(r.requestedAt)}</td>
                      <td className="px-4 py-3 font-medium">{r.deviceName}</td>
                      <td className="px-4 py-3 font-mono text-xs">{r.mac}</td>
                      <td className="px-4 py-3 font-mono text-xs">{r.broadcast ? `${r.broadcast}:${r.port}` : '—'}{r.interface && <span className="block font-sans text-slate-500">{r.interface}</span>}</td>
                      <td className="px-4 py-3">{r.status === 'sent' ? <Badge tone="green">Sent</Badge> : <Badge tone="red">Failed</Badge>}</td>
                      <td className="px-4 py-3">{r.status === 'sent' ? <ReachabilityBadge value={r.reachability} /> : <span className="text-slate-400">—</span>}</td>
                      <td className="max-w-xs px-4 py-3 text-xs text-red-700">{r.error}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between border-t border-slate-100 px-4 py-3 text-sm text-slate-500">
              <span>{offset + 1}–{offset + data.records.length} of {data.total}</span>
              <div className="flex gap-2">
                <Button size="sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>Previous</Button>
                <Button size="sm" disabled={offset + PAGE >= data.total} onClick={() => setOffset(offset + PAGE)}>Next</Button>
              </div>
            </div>
          </>
        )}
      </Card>
    </>
  )
}
