import { Badge, Card, EmptyState, ErrorBlock, LoadingBlock, PageHeader } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { api } from '../services/api'

export default function Network() {
  const { data, error, loading, reload } = useAsync(api.interfaces, [])
  return (
    <>
      <PageHeader title="Network" subtitle="Local IPv4 interfaces and the broadcast addresses WakePilot may use." />
      {loading ? <LoadingBlock /> : error ? <ErrorBlock error={error} onRetry={reload} /> : data && data.interfaces.length === 0 ? (
        <Card><EmptyState title="No IPv4 interfaces found" body="Connect to a network and reload this page." /></Card>
      ) : data && (
        <div className="space-y-4">
          {data.interfaces.map((i) => (
            <Card key={i.name} className="overflow-hidden">
              <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 px-5 py-3">
                <h2 className="font-mono text-sm font-semibold">{i.name}</h2>
                <Badge tone={i.up ? 'green' : 'red'}>{i.up ? 'Up' : 'Down'}</Badge>
                {i.loopback && <Badge>Loopback</Badge>}
                {!i.supportsBroadcast && !i.loopback && <Badge tone="amber" title="This interface does not advertise broadcast support.">No broadcast flag</Badge>}
                {data.defaultInterface === i.name && <Badge tone="indigo">Default</Badge>}
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full text-left text-sm">
                  <thead className="text-xs uppercase tracking-wide text-slate-500">
                    <tr>{['IP address', 'CIDR', 'Netmask', 'Network', 'Broadcast'].map((h) => <th key={h} className="px-5 py-2 font-medium">{h}</th>)}</tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 font-mono text-xs">
                    {i.addresses.map((a) => (
                      <tr key={a.cidr}>
                        <td className="px-5 py-2">{a.ip}</td><td className="px-5 py-2">{a.cidr}</td>
                        <td className="px-5 py-2">{a.netmask}</td><td className="px-5 py-2">{a.network}</td>
                        <td className="px-5 py-2">{a.broadcast || <span className="text-slate-400">n/a (/{a.prefixLen})</span>}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          ))}
          <p className="text-xs text-slate-500">
            For safety, a device's broadcast address must be one of the broadcast addresses above or 255.255.255.255.
            When several interfaces could match, WakePilot asks you to choose instead of guessing.
          </p>
        </div>
      )}
    </>
  )
}
