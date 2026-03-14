import { useRef, useEffect, useCallback } from 'react'
import type { TraceEntry as TraceEntryType } from '../types'
import TraceEntry from './TraceEntry'

interface TracePanelProps {
  entries: TraceEntryType[]
  onClear: () => void
}

export default function TracePanel({ entries, onClear }: TracePanelProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const userScrolledUp = useRef(false)

  const handleScroll = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 30
    userScrolledUp.current = !atBottom
  }, [])

  useEffect(() => {
    const el = containerRef.current
    if (el && !userScrolledUp.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [entries.length])

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-3 py-2 border-b border-gray-700 shrink-0">
        <span className="text-xs font-semibold text-gray-300">Protocol Trace</span>
        <button
          onClick={onClear}
          className="px-2 py-0.5 text-xs bg-gray-700 hover:bg-gray-600 rounded text-gray-300 cursor-pointer"
        >
          Clear
        </button>
      </div>
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 overflow-y-auto"
      >
        {entries.length === 0 && (
          <p className="p-3 text-xs text-gray-500 italic">No trace entries yet</p>
        )}
        {(() => {
          const sorted = [...entries].sort((a, b) => a.seq - b.seq)
          // Find the last response entry (direction=recv, has result data)
          let lastResponseSeq = -1
          for (let i = sorted.length - 1; i >= 0; i--) {
            if (sorted[i].direction === 'recv' && sorted[i].method?.includes('response')) {
              lastResponseSeq = sorted[i].seq
              break
            }
          }
          return sorted.map((entry) => (
            <TraceEntry key={entry.seq} entry={entry} defaultExpanded={entry.seq === lastResponseSeq} />
          ))
        })()}
      </div>
    </div>
  )
}
