import type { ReactNode } from 'react'
import { Badge, Card, ErrorBlock, LoadingBlock, PageHeader } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { api } from '../services/api'

function Section({ title, rows }: { title: string; rows: [string, ReactNode][] }) {
  return (
    <Card>
      <h2 className="border-b border-slate-100 px-5 py-3 text-sm font-semibold">{title}</h2>
      <dl className="divide-y divide-slate-100">
        {rows.map(([k, v]) => (
          <div key={k} className="grid grid-cols-3 gap-4 px-5 py-3 text-sm">
            <dt className="text-slate-500">{k}</dt>
            <dd className="col-span-2 font-medium">{v}</dd>
          </div>
        ))}
      </dl>
    </Card>
  )
}

export default function Settings() {
  const { data: s, error, loading, reload } = useAsync(api.settings, [])
  return (
    <>
      <PageHeader title="Settings" subtitle="Application configuration. Edit the YAML config file and restart to change these values." />
      {loading ? <LoadingBlock /> : error ? <ErrorBlock error={error} onRetry={reload} /> : s && (
        <div className="grid max-w-3xl gap-4">
          <Section title="Wake-on-LAN" rows={[
            ['Default UDP port', s.wol.defaultPort],
            ['Default network interface', s.wol.defaultInterface || <span className="font-normal text-slate-400">none (choose per device)</span>],
            ['Send timeout', s.wol.sendTimeout],
            ['Post-wake verification', s.wol.verifyEnabled ? `Enabled (up to ${s.wol.verifyTimeout})` : 'Disabled'],
          ]} />
          <Section title="Availability monitoring" rows={[
            ['Background checks', s.monitor.enabled ? `Every ${s.monitor.interval}` : 'Disabled'],
            ['Probe method', `TCP connect on ports ${s.monitor.ports.join(', ')}`],
          ]} />
          <Section title="Logging" rows={[['Level', s.logging.level], ['Format', s.logging.format]]} />
          <Section title="Server & security" rows={[
            ['Listening on', <span className="font-mono">{s.server.host}:{s.server.port}</span>],
            ['Access', s.server.loopbackOnly ? <Badge tone="green">Local only</Badge> : <Badge tone="amber">Network accessible</Badge>],
            ['Authentication', s.auth.enabled ? <Badge tone="green">Bearer token</Badge> : <Badge>Off (local only)</Badge>],
            ['Version', s.version],
          ]} />
        </div>
      )}
    </>
  )
}
