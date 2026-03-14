import { useState } from 'react'
import type { PromptDef } from '../types'

interface PromptFormProps {
  prompt: PromptDef
  onSubmit: (name: string, args: Record<string, string>) => void
  loading: boolean
}

export default function PromptForm({ prompt, onSubmit, loading }: PromptFormProps) {
  const [values, setValues] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<string[]>([])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const missing = (prompt.arguments || [])
      .filter((a) => a.required && !values[a.name])
      .map((a) => `${a.name} is required`)
    if (missing.length > 0) {
      setErrors(missing)
      return
    }
    setErrors([])
    onSubmit(prompt.name, values)
  }

  return (
    <form onSubmit={handleSubmit} className="p-3 space-y-3">
      <h3 className="text-sm font-semibold text-gray-200">{prompt.name}</h3>
      {prompt.description && (
        <p className="text-xs text-gray-400">{prompt.description}</p>
      )}
      {(prompt.arguments || []).map((arg) => (
        <div key={arg.name}>
          <label className="block text-xs text-gray-400 mb-1">
            {arg.name}
            {arg.required && <span className="text-red-400 ml-0.5">*</span>}
            {arg.description && (
              <span className="ml-2 text-gray-500">({arg.description})</span>
            )}
          </label>
          <input
            type="text"
            value={values[arg.name] ?? ''}
            onChange={(e) =>
              setValues((prev) => ({ ...prev, [arg.name]: e.target.value }))
            }
            className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
          />
        </div>
      ))}
      {errors.length > 0 && (
        <div className="text-xs text-red-400">{errors.join(', ')}</div>
      )}
      <button
        type="submit"
        disabled={loading}
        className="px-4 py-1.5 text-sm bg-purple-600 hover:bg-purple-500 rounded text-white disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
      >
        {loading ? (
          <span className="flex items-center gap-2">
            <span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            Getting...
          </span>
        ) : (
          'Get'
        )}
      </button>
    </form>
  )
}
