import { useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { auth } from '../services/api'

const icons: Record<string, string> = {
  overview: 'M3 13h8V3H3v10Zm0 8h8v-6H3v6Zm10 0h8V11h-8v10Zm0-18v6h8V3h-8Z',
  devices: 'M4 5h16v10H4V5Zm4 14h8m-4-4v4',
  add: 'M12 5v14M5 12h14',
  network: 'M12 3v6m0 0a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm0 6v6M5 21h14M5 3h14',
  history: 'M12 8v4l3 2M3 12a9 9 0 1 0 3-6.7M3 4v5h5',
  settings: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Zm7.4-3a7.4 7.4 0 0 0-.1-1.2l2-1.6-2-3.4-2.4 1a7.6 7.6 0 0 0-2-1.2L14.5 3h-4l-.4 2.6a7.6 7.6 0 0 0-2 1.2l-2.4-1-2 3.4 2 1.6a7.4 7.4 0 0 0 0 2.4l-2 1.6 2 3.4 2.4-1c.6.5 1.3.9 2 1.2l.4 2.6h4l.4-2.6c.7-.3 1.4-.7 2-1.2l2.4 1 2-3.4-2-1.6c.1-.4.1-.8.1-1.2Z',
}

const nav = [
  { to: '/', label: 'Overview', icon: 'overview', end: true },
  { to: '/devices', label: 'Devices', icon: 'devices', end: true },
  { to: '/devices/new', label: 'Add Device', icon: 'add', end: true },
  { to: '/network', label: 'Network', icon: 'network', end: true },
  { to: '/history', label: 'Wake History', icon: 'history', end: true },
  { to: '/settings', label: 'Settings', icon: 'settings', end: true },
]

function Icon({ name }: { name: string }) {
  return (
    <svg className="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      <path d={icons[name]} />
    </svg>
  )
}

export function Layout({ authRequired, onLogout }: { authRequired: boolean; onLogout: () => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="min-h-screen lg:flex">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3 lg:hidden">
        <Brand />
        <button aria-label="Toggle navigation" aria-expanded={open} onClick={() => setOpen((o) => !o)} className="rounded-md p-2 text-slate-600 hover:bg-slate-100">
          <svg className="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8"><path d="M4 7h16M4 12h16M4 17h16" strokeLinecap="round" /></svg>
        </button>
      </header>

      <aside className={`${open ? 'block' : 'hidden'} w-full shrink-0 border-r border-slate-200 bg-white lg:sticky lg:top-0 lg:block lg:h-screen lg:w-60`}>
        <div className="hidden px-5 py-5 lg:block"><Brand /></div>
        <nav className="flex flex-col gap-1 px-3 py-2" aria-label="Main">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              onClick={() => setOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-lg px-3 py-2 text-sm font-medium transition ${
                  isActive ? 'bg-indigo-50 text-indigo-700' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
                }`}
            >
              <Icon name={n.icon} /> {n.label}
            </NavLink>
          ))}
        </nav>
        {authRequired && auth.get() && (
          <div className="px-3 py-2 lg:absolute lg:bottom-4 lg:w-60">
            <button onClick={onLogout} className="w-full rounded-lg px-3 py-2 text-left text-sm text-slate-500 hover:bg-slate-100">Sign out</button>
          </div>
        )}
      </aside>

      <main className="min-w-0 flex-1 px-4 py-6 sm:px-8 lg:py-8">
        <div className="mx-auto max-w-6xl"><Outlet /></div>
      </main>
    </div>
  )
}

function Brand() {
  return (
    <div className="flex items-center gap-2.5">
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600 text-white">
        <svg className="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round"><path d="M12 3v9M7.5 7a7 7 0 1 0 9 0" /></svg>
      </span>
      <span className="text-lg font-semibold tracking-tight">WakePilot</span>
    </div>
  )
}
