import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ToastProvider } from './components/Toast'
import { Button, Card, LoadingBlock } from './components/ui'
import { ApiError, UNAUTHORIZED_EVENT, api, auth } from './services/api'
import type { Health } from './types'
import DeviceForm from './pages/DeviceForm'
import Devices from './pages/Devices'
import History from './pages/History'
import Network from './pages/Network'
import Overview from './pages/Overview'
import Settings from './pages/Settings'

function Login({ onDone }: { onDone: () => void }) {
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    auth.set(token.trim())
    try {
      await api.devices() // any authenticated call validates the token
      onDone()
    } catch (err) {
      auth.clear()
      setError(err instanceof ApiError && err.status === 401 ? 'That token was not accepted.' : (err as Error).message)
    } finally { setBusy(false) }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm p-6">
        <h1 className="text-xl font-semibold">Sign in to WakePilot</h1>
        <p className="mt-1 text-sm text-slate-500">Enter the access token from your server configuration.</p>
        <form onSubmit={submit} className="mt-5 space-y-4">
          <div>
            <label htmlFor="token" className="block text-sm font-medium text-slate-700">Access token</label>
            <input id="token" type="password" autoComplete="current-password" autoFocus value={token} onChange={(e) => setToken(e.target.value)}
              className="mt-1 block w-full rounded-lg border-0 px-3 py-2 text-sm shadow-xs ring-1 ring-inset ring-slate-300 focus:ring-2 focus:ring-indigo-600" />
            {error && <p className="mt-1 text-sm text-red-600" role="alert">{error}</p>}
          </div>
          <Button type="submit" variant="primary" className="w-full" loading={busy} disabled={!token.trim()}>Sign in</Button>
        </form>
      </Card>
    </div>
  )
}

export default function App() {
  const [health, setHealth] = useState<Health>()
  const [error, setError] = useState('')
  const [signedIn, setSignedIn] = useState(!!auth.get())

  useEffect(() => { api.health().then(setHealth, (e: Error) => setError(e.message)) }, [])
  useEffect(() => {
    const onUnauthorized = () => { auth.clear(); setSignedIn(false) }
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
  }, [])
  const logout = useCallback(() => { auth.clear(); setSignedIn(false) }, [])

  if (error) return <div className="p-8 text-center text-red-700" role="alert">{error}</div>
  if (!health) return <LoadingBlock label="Connecting…" />
  if (health.authRequired && !signedIn) return <Login onDone={() => setSignedIn(true)} />

  return (
    <ToastProvider>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout authRequired={health.authRequired} onLogout={logout} />}>
            <Route index element={<Overview />} />
            <Route path="devices" element={<Devices />} />
            <Route path="devices/new" element={<DeviceForm />} />
            <Route path="devices/:id/edit" element={<DeviceForm />} />
            <Route path="network" element={<Network />} />
            <Route path="history" element={<History />} />
            <Route path="settings" element={<Settings />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </ToastProvider>
  )
}
