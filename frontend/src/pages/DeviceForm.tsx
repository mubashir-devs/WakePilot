import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useToast } from '../components/Toast'
import { Button, Card, ErrorBlock, LoadingBlock, PageHeader } from '../components/ui'
import { useAsync } from '../hooks/useAsync'
import { ApiError, api } from '../services/api'
import type { DeviceInput } from '../types'

const MAC_RE = /^([0-9a-f]{2}([:-]?)){5}[0-9a-f]{2}$|^[0-9a-f]{4}\.[0-9a-f]{4}\.[0-9a-f]{4}$/i
const IPV4_RE = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/

const empty: DeviceInput = { name: '', mac: '', ip: '', broadcast: '', port: 0, interface: '', description: '' }
type Errors = Partial<Record<keyof DeviceInput | 'body', string>>

function validate(v: DeviceInput): Errors {
  const e: Errors = {}
  if (!v.name.trim()) e.name = 'Name is required.'
  if (!MAC_RE.test(v.mac.trim())) e.mac = 'Enter a valid MAC address, e.g. aa:bb:cc:dd:ee:ff.'
  if (v.ip && !IPV4_RE.test(v.ip.trim())) e.ip = 'Enter a valid IPv4 address.'
  if (v.broadcast && !IPV4_RE.test(v.broadcast.trim())) e.broadcast = 'Enter a valid IPv4 address.'
  if (v.port && (!Number.isInteger(v.port) || v.port < 1 || v.port > 65535)) e.port = 'Port must be between 1 and 65535.'
  return e
}

function Field({ label, error, hint, children, htmlFor }: { label: string; error?: string; hint?: string; children: ReactNode; htmlFor: string }) {
  return (
    <div>
      <label htmlFor={htmlFor} className="block text-sm font-medium text-slate-700">{label}</label>
      <div className="mt-1">{children}</div>
      {error ? <p className="mt-1 text-sm text-red-600" role="alert">{error}</p> : hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  )
}

const input = (bad?: string) =>
  `block w-full rounded-lg border-0 px-3 py-2 text-sm shadow-xs ring-1 ring-inset focus:ring-2 focus:ring-indigo-600 ${bad ? 'ring-red-400' : 'ring-slate-300'}`

export default function DeviceForm() {
  const { id } = useParams()
  const editing = id !== undefined
  const nav = useNavigate()
  const toast = useToast()

  const ifaces = useAsync(api.interfaces, [])
  const settings = useAsync(api.settings, [])
  const existing = useAsync(async () => (editing ? api.device(Number(id)) : undefined), [id])

  const [form, setForm] = useState<DeviceInput>(empty)
  const [errors, setErrors] = useState<Errors>({})
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const d = existing.data
    if (d) setForm({ name: d.name, mac: d.mac, ip: d.ip, broadcast: d.broadcast, port: d.port, interface: d.interface, description: d.description })
  }, [existing.data])

  const set = <K extends keyof DeviceInput>(k: K, v: DeviceInput[K]) => {
    setForm((f) => ({ ...f, [k]: v }))
    setErrors((e) => ({ ...e, [k]: undefined }))
  }

  const selected = ifaces.data?.interfaces.find((i) => i.name === (form.interface || ifaces.data?.defaultInterface))
  const autoBroadcast = selected?.addresses.map((a) => a.broadcast).filter(Boolean).join(', ')
  const defaultPort = settings.data?.wol.defaultPort ?? 9

  async function submit(ev: FormEvent) {
    ev.preventDefault()
    const clientErrors = validate(form)
    setErrors(clientErrors)
    if (Object.keys(clientErrors).length) return
    setSaving(true)
    try {
      const body = { ...form, port: form.port || 0 }
      if (editing) await api.updateDevice(Number(id), body)
      else await api.createDevice(body)
      toast('success', editing ? 'Device updated' : 'Device added', form.name)
      nav('/devices')
    } catch (e) {
      if (e instanceof ApiError && e.details && Object.keys(e.details).length) setErrors(e.details as Errors)
      else toast('error', 'Could not save device', e instanceof Error ? e.message : String(e))
    } finally { setSaving(false) }
  }

  if (editing && existing.loading) return <LoadingBlock />
  if (editing && existing.error) return <ErrorBlock error={existing.error} />

  return (
    <>
      <PageHeader title={editing ? 'Edit device' : 'Add device'} subtitle="The MAC address identifies the machine; the network settings decide where the magic packet goes." />
      <Card className="max-w-3xl p-6">
        <form onSubmit={submit} noValidate className="grid gap-5 sm:grid-cols-2">
          {errors.body && <p className="rounded-lg bg-red-50 p-3 text-sm text-red-700 sm:col-span-2" role="alert">{errors.body}</p>}
          <div className="sm:col-span-2">
            <Field label="Device name" htmlFor="name" error={errors.name}>
              <input id="name" className={input(errors.name)} value={form.name} maxLength={100} onChange={(e) => set('name', e.target.value)} placeholder="Office PC" autoFocus />
            </Field>
          </div>
          <Field label="MAC address" htmlFor="mac" error={errors.mac}>
            <input id="mac" className={`${input(errors.mac)} font-mono`} value={form.mac} onChange={(e) => set('mac', e.target.value)} placeholder="aa:bb:cc:dd:ee:ff" spellCheck={false} />
          </Field>
          <Field label="IP address" htmlFor="ip" error={errors.ip} hint="Optional. Enables availability checks and subnet detection.">
            <input id="ip" className={`${input(errors.ip)} font-mono`} value={form.ip} onChange={(e) => set('ip', e.target.value)} placeholder="192.168.1.50" inputMode="decimal" />
          </Field>
          <Field label="Network interface" htmlFor="interface" error={errors.interface}
            hint={ifaces.data?.defaultInterface && !form.interface ? `Default interface: ${ifaces.data.defaultInterface}` : 'Which interface sends the packet. Leave on automatic to match by IP address.'}>
            <select id="interface" className={input(errors.interface)} value={form.interface} onChange={(e) => set('interface', e.target.value)}>
              <option value="">Automatic</option>
              {ifaces.data?.interfaces.map((i) => (
                <option key={i.name} value={i.name}>{i.name}{i.addresses[0] ? ` — ${i.addresses[0].cidr}` : ''}{i.up ? '' : ' (down)'}</option>
              ))}
              {form.interface && !ifaces.data?.interfaces.some((i) => i.name === form.interface) && <option value={form.interface}>{form.interface} (not present)</option>}
            </select>
          </Field>
          <Field label="UDP port" htmlFor="port" error={errors.port} hint="Usually 9 (or 7).">
            <input id="port" type="number" min={1} max={65535} className={input(errors.port)} value={form.port || ''} placeholder={String(defaultPort)}
              onChange={(e) => set('port', e.target.value === '' ? 0 : Number(e.target.value))} />
          </Field>
          <div className="sm:col-span-2">
            <Field label="Broadcast address" htmlFor="broadcast" error={errors.broadcast}
              hint={autoBroadcast ? `Leave empty to use the interface's broadcast address (${autoBroadcast}).` : 'Leave empty to derive it from the interface or IP address. Must belong to a local subnet, or be 255.255.255.255.'}>
              <input id="broadcast" className={`${input(errors.broadcast)} font-mono`} value={form.broadcast} onChange={(e) => set('broadcast', e.target.value)} placeholder={autoBroadcast || '192.168.1.255'} />
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field label="Description" htmlFor="description" error={errors.description}>
              <textarea id="description" rows={3} maxLength={500} className={input(errors.description)} value={form.description} onChange={(e) => set('description', e.target.value)} placeholder="Optional notes" />
            </Field>
          </div>
          <div className="flex justify-end gap-2 sm:col-span-2">
            <Link to="/devices"><Button type="button">Cancel</Button></Link>
            <Button type="submit" variant="primary" loading={saving}>{editing ? 'Save changes' : 'Add device'}</Button>
          </div>
        </form>
      </Card>
    </>
  )
}
