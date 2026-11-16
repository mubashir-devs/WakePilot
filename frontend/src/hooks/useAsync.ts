import { useCallback, useEffect, useRef, useState } from 'react'

export interface AsyncState<T> {
  data: T | undefined
  error: Error | undefined
  loading: boolean
  reload: () => void
}

/**
 * Runs `fn` on mount and whenever `deps` change. With `pollMs`, refreshes in the
 * background (no loading flash). Stale responses are ignored.
 */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[] = [], pollMs?: number): AsyncState<T> {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<Error>()
  const [loading, setLoading] = useState(true)
  const seq = useRef(0)
  const fnRef = useRef(fn)
  fnRef.current = fn

  const run = useCallback((silent: boolean) => {
    const id = ++seq.current
    if (!silent) setLoading(true)
    fnRef.current().then(
      (d) => { if (id === seq.current) { setData(d); setError(undefined); setLoading(false) } },
      (e) => { if (id === seq.current) { setError(e as Error); setLoading(false) } },
    )
  }, [])

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { run(false); return () => { seq.current++ } }, deps)

  useEffect(() => {
    if (!pollMs) return
    const t = setInterval(() => run(true), pollMs)
    return () => clearInterval(t)
  }, [pollMs, run])

  return { data, error, loading, reload: useCallback(() => run(true), [run]) }
}
