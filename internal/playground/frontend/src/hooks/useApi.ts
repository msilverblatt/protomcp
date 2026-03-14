import { useState, useCallback } from 'react'

interface ApiState<T> {
  data: T | null
  loading: boolean
  error: string | null
}

export function useApi<T>() {
  const [state, setState] = useState<ApiState<T>>({
    data: null,
    loading: false,
    error: null,
  })

  const execute = useCallback(async (url: string, options?: RequestInit): Promise<T | null> => {
    setState({ data: null, loading: true, error: null })
    try {
      const res = await fetch(url, {
        headers: { 'Content-Type': 'application/json' },
        ...options,
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `HTTP ${res.status}`)
      }
      const data = (await res.json()) as T
      setState({ data, loading: false, error: null })
      return data
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setState({ data: null, loading: false, error: message })
      return null
    }
  }, [])

  return { ...state, execute }
}

export function useFetch<T>() {
  const [loading, setLoading] = useState(false)

  const fetchData = useCallback(async (url: string): Promise<T | null> => {
    setLoading(true)
    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      return (await res.json()) as T
    } catch {
      return null
    } finally {
      setLoading(false)
    }
  }, [])

  return { loading, fetchData }
}
