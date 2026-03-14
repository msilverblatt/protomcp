import { useState, useEffect } from 'react'
import type { Tool } from '../types'

interface ToolFormProps {
  tool: Tool
  onSubmit: (name: string, args: Record<string, unknown>) => void
  loading: boolean
}

export default function ToolForm({ tool, onSubmit, loading }: ToolFormProps) {
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [errors, setErrors] = useState<string[]>([])

  // Reset form when tool changes
  useEffect(() => {
    setValues({})
    setErrors([])
  }, [tool.name])

  const properties = tool.inputSchema?.properties || {}
  const required = new Set(tool.inputSchema?.required || [])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const missing = [...required].filter((k) => {
      const v = values[k]
      return v === undefined || v === '' || v === null
    })
    if (missing.length > 0) {
      setErrors(missing.map((k) => `${k} is required`))
      return
    }
    setErrors([])
    onSubmit(tool.name, values)
  }

  const setValue = (key: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  return (
    <form onSubmit={handleSubmit} className="p-3 space-y-3">
      <h3 className="text-sm font-semibold text-gray-200">{tool.name}</h3>
      {Object.entries(properties).map(([key, prop]) => {
        const isRequired = required.has(key)
        return (
          <div key={key}>
            <label className="block text-xs text-gray-400 mb-1">
              {key}
              {isRequired && <span className="text-red-400 ml-0.5">*</span>}
              {prop.description && (
                <span className="ml-2 text-gray-500">({prop.description})</span>
              )}
            </label>
            {prop.type === 'boolean' ? (
              <input
                type="checkbox"
                checked={!!values[key]}
                onChange={(e) => setValue(key, e.target.checked)}
                className="accent-blue-500"
              />
            ) : prop.type === 'integer' || prop.type === 'number' ? (
              <input
                type="number"
                step={prop.type === 'integer' ? 1 : 'any'}
                value={(values[key] as string) ?? ''}
                onChange={(e) =>
                  setValue(key, e.target.value === '' ? '' : Number(e.target.value))
                }
                className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
              />
            ) : (
              <input
                type="text"
                value={(values[key] as string) ?? ''}
                onChange={(e) => setValue(key, e.target.value)}
                className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
              />
            )}
          </div>
        )
      })}
      {errors.length > 0 && (
        <div className="text-xs text-red-400">{errors.join(', ')}</div>
      )}
      <button
        type="submit"
        disabled={loading}
        className="px-4 py-1.5 text-sm bg-blue-600 hover:bg-blue-500 rounded text-white disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
      >
        {loading ? (
          <span className="flex items-center gap-2">
            <span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            Calling...
          </span>
        ) : (
          'Call'
        )}
      </button>
    </form>
  )
}
