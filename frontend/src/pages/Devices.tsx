import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { AvailabilityBadge } from '../components/StatusBadges'
import { useToast } from '../components/Toast'
import { Button, Card, EmptyState, ErrorBlock, LoadingBlock, PageHeader } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { ApiError, api } from '../services/api'
import type { Device } from '../types'

export default function Devices() {
  const toast = useToast()
  const { data, error, loading, reload } = useAsync(api.devices, [], 5_000)
  const [busy, setBusy] = useState<Record<string, boolean>>({})
  const [toDelete, setToDelete] = useState<Device | null>(null)

  const withBusy = async (key: string, fn: () => Promise<void>) => {
    setBusy((b) => ({ ...b, [key]: true }))
    try { await fn() } finally { setBusy((b) => ({ ...b, [key]: false })) }
  }
  const fail = (title: string, e: unknown) => toast('error', title, e instanceof ApiError ? e.message : String(e))

  const wake = (d: Device) => withBusy(`wake${d.id}`, async () => {
    try {
      await api.wake(d.id)
      toast('success', `Magic packet sent to ${d.name}`,
        'This does not confirm the device has powered on. WakePilot will check whether it becomes reachable.')
      reload()
    } catch (e) { fail(`Could not wake ${d.name}`, e) }
  })

  const check = (d: Device) => withBusy(`check${d.id}`, async () => {
    try { await api.check(d.id); reload() } catch (e) { fail(`Could not check ${d.name}`, e) }
  })

  const remove = () => toDelete && withBusy('delete', async () => {
    try {
      await api.deleteDevice(toDelete.id)
      toast('success', `Deleted ${toDelete.name}`)
      setToDelete(null)
      reload()
    } catch (e) { fail('Delete failed', e) }
  })

  return (
    <>
      <PageHeader title="Devices" subtitle="Computers you can wake over the network."
        actions={<Link to="/devices/new"><Button variant="primary">Add device</Button></Link>} />
      <Card className="overflow-hidden">
        {loading && !data ? <LoadingBlock /> : error && !data ? <ErrorBlock error={error} onRetry={reload} /> : data && data.length === 0 ? (
          <EmptyState title="No devices yet" body="Register a computer by its MAC address to start waking it remotely."
            action={<Link to="/devices/new"><Button variant="primary">Add your first device</Button></Link>} />
        ) : (
          <div className="overflow-x-auto">
            <table className="min-w-full text-left text-sm">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  {['Name', 'MAC address', 'IP address', 'Interface', 'Availability', ''].map((h) => (
                    <th key={h} className="px-4 py-3 font-medium">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {data?.map((d) => (
                  <tr key={d.id} className="hover:bg-slate-50/60">
                    <td className="px-4 py-3">
                      <p className="font-medium">{d.name}</p>
                      {d.description && <p className="max-w-xs truncate text-xs text-slate-500">{d.description}</p>}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">{d.mac}</td>
                    <td className="px-4 py-3 font-mono text-xs">{d.ip || <span className="text-slate-400">—</span>}</td>
                    <td className="px-4 py-3">{d.interface || <span className="text-slate-400">auto</span>}</td>
                    <td className="px-4 py-3"><AvailabilityBadge status={d.availability.status} /></td>
                    <td className="px-4 py-3">
                      <div className="flex justify-end gap-1.5">
                        <Button size="sm" variant="primary" loading={busy[`wake${d.id}`]} onClick={() => wake(d)}>Wake</Button>
                        {d.ip && <Button size="sm" loading={busy[`check${d.id}`]} onClick={() => check(d)} title="Probe this device now">Check</Button>}
                        <Link to={`/devices/${d.id}/edit`}><Button size="sm">Edit</Button></Link>
                        <Button size="sm" variant="ghost" className="text-red-600 hover:bg-red-50" onClick={() => setToDelete(d)}>Delete</Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <p className="mt-3 text-xs text-slate-500">
        “No response” means no network probe was answered. The device may be off, asleep, or blocking probes.
      </p>

      <ConfirmDialog open={!!toDelete} title="Delete device?" confirmLabel="Delete" busy={busy['delete']}
        message={`“${toDelete?.name ?? ''}” will be removed. Its wake history is kept.`}
        onConfirm={remove} onCancel={() => setToDelete(null)} />
    </>
  )
}
