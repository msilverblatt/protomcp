import { useState } from 'react'
import type { Resource } from '../types'

interface ResourceFormProps {
  resource: Resource
  onSubmit: (uri: string) => void
  loading: boolean
}

export default function ResourceForm({ resource, onSubmit, loading }: ResourceFormProps) {
  const [uri, setUri] = useState(resource.uri)

  return (
    <div className="p-3 space-y-3">
      <h3 className="text-sm font-semibold text-gray-200">{resource.name || resource.uri}</h3>
      {resource.description && (
        <p className="text-xs text-gray-400">{resource.description}</p>
      )}
      <div>
        <label className="block text-xs text-gray-400 mb-1">URI</label>
        <input
          type="text"
          value={uri}
          onChange={(e) => setUri(e.target.value)}
          className="w-full px-2 py-1 text-sm bg-gray-700 border border-gray-600 rounded text-gray-200 focus:outline-none focus:border-blue-500"
        />
      </div>
      <button
        onClick={() => onSubmit(uri)}
        disabled={loading || !uri}
        className="px-4 py-1.5 text-sm bg-green-600 hover:bg-green-500 rounded text-white disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
      >
        {loading ? (
          <span className="flex items-center gap-2">
            <span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            Reading...
          </span>
        ) : (
          'Read'
        )}
      </button>
    </div>
  )
}
