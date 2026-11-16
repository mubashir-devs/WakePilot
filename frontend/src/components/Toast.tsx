import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'

type Kind = 'success' | 'error' | 'info'
interface Toast { id: number; kind: Kind; title: string; body?: string }

const Ctx = createContext<(kind: Kind, title: string, body?: string) => void>(() => {})
export const useToast = () => useContext(Ctx)

const styles: Record<Kind, string> = {
  success: 'bg-emerald-50 ring-emerald-200 text-emerald-900',
  error: 'bg-red-50 ring-red-200 text-red-900',
  info: 'bg-white ring-slate-200 text-slate-900',
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const push = useCallback((kind: Kind, title: string, body?: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, kind, title, body }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), kind === 'error' ? 8000 : 5000)
  }, [])
  const value = useMemo(() => push, [push])
  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-0 z-50 flex flex-col items-end gap-2 p-4" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} role={t.kind === 'error' ? 'alert' : 'status'} className={`pointer-events-auto w-full max-w-sm rounded-lg p-4 shadow-lg ring-1 ${styles[t.kind]}`}>
            <p className="text-sm font-medium">{t.title}</p>
            {t.body && <p className="mt-1 text-sm opacity-80">{t.body}</p>}
          </div>
        ))}
      </div>
    </Ctx.Provider>
  )
}
