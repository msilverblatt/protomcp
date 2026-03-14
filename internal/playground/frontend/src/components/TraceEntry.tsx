import { useState } from 'react'
import type { TraceEntry as TraceEntryType } from '../types'

interface TraceEntryProps {
  entry: TraceEntryType
}

export default function TraceEntry({ entry }: TraceEntryProps) {
  const [expanded, setExpanded] = useState(false)

  const isSend = entry.direction === 'send'
  const arrow = isSend ? '\u2192' : '\u2190'
  const colorClass = isSend
    ? 'text-blue-400'
    : entry.method?.startsWith('notifications/')
      ? 'text-yellow-400'
      : 'text-green-400'

  let prettyJson = ''
  if (expanded) {
    try {
      prettyJson = JSON.stringify(JSON.parse(entry.raw), null, 2)
    } catch {
      prettyJson = entry.raw
    }
  }

  const ts = entry.timestamp
    ? new Date(entry.timestamp).toLocaleTimeString('en-US', { hour12: false })
    : ''

  return (
    <div className="border-b border-gray-700/50">
      <button
        onClick={() => setExpanded(!expanded)}
        className={`w-full text-left px-3 py-1.5 text-xs flex items-center gap-2 hover:bg-gray-700/30 cursor-pointer ${colorClass}`}
      >
        <span className="text-gray-500 shrink-0">{ts}</span>
        <span className="shrink-0">{arrow}</span>
        <span className="truncate">{entry.method || 'unknown'}</span>
        <span className="ml-auto text-gray-600 shrink-0">{expanded ? '\u25BC' : '\u25B6'}</span>
      </button>
      {expanded && (
        <pre className="px-3 pb-2 text-xs text-gray-400 bg-gray-900/50 overflow-auto max-h-48 whitespace-pre-wrap">
          {prettyJson}
        </pre>
      )}
    </div>
  )
}
