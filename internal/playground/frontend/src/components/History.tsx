import type { HistoryItem } from '../types'
import ResultView from './ResultView'

interface HistoryProps {
  items: HistoryItem[]
}

export default function History({ items }: HistoryProps) {
  if (items.length === 0) return null

  return (
    <div className="border-t border-gray-700 overflow-y-auto max-h-80">
      <div className="px-3 py-1 text-xs font-semibold text-gray-400 bg-gray-800/50 sticky top-0">
        History
      </div>
      {[...items].reverse().map((item) => (
        <div key={item.id} className="border-b border-gray-700/50">
          <div className="px-3 py-1.5 flex items-center gap-2">
            <span
              className={`text-xs font-medium ${
                item.type === 'error'
                  ? 'text-red-400'
                  : item.type === 'system'
                    ? 'text-yellow-400'
                    : 'text-gray-300'
              }`}
            >
              {item.label}
            </span>
            <span className="text-xs text-gray-600 ml-auto">
              {item.timestamp.toLocaleTimeString('en-US', { hour12: false })}
            </span>
          </div>
          {item.type === 'error' && item.response != null && (
            <div className="px-3 pb-2 text-xs text-red-400">{String(item.response) as string}</div>
          )}
          {item.type !== 'error' && item.type !== 'system' && item.response != null && (
            <ResultView
              data={item.response}
              durationMs={
                typeof item.response === 'object' && item.response !== null && 'duration_ms' in (item.response as Record<string, unknown>)
                  ? (item.response as Record<string, unknown>).duration_ms as number
                  : undefined
              }
            />
          )}
        </div>
      ))}
    </div>
  )
}
