interface ResultViewProps {
  data: unknown
  durationMs?: number
}

interface ToolErrorShape {
  error_code?: string
  message?: string
  suggestion?: string
  retryable?: boolean
}

function isToolError(data: unknown): data is ToolErrorShape {
  if (typeof data !== 'object' || data === null) return false
  const obj = data as Record<string, unknown>
  return typeof obj.error_code === 'string' || typeof obj.suggestion === 'string'
}

function extractToolError(data: unknown): ToolErrorShape | null {
  // Check top-level
  if (isToolError(data)) return data

  // Check nested in result field (the shape returned by handleCallTool)
  if (typeof data === 'object' && data !== null) {
    const obj = data as Record<string, unknown>
    if (isToolError(obj.result)) return obj.result
    // Check inside content array (MCP content items)
    if (Array.isArray(obj.result)) {
      for (const item of obj.result) {
        if (typeof item === 'object' && item !== null && isToolError(item)) {
          return item
        }
      }
    }
  }
  return null
}

export default function ResultView({ data, durationMs }: ResultViewProps) {
  const toolError = extractToolError(data)
  const formatted = typeof data === 'string' ? data : JSON.stringify(data, null, 2)

  return (
    <div className="p-3 border-t border-gray-700">
      {durationMs !== undefined && (
        <div className="text-xs text-gray-500 mb-1">{durationMs}ms</div>
      )}

      {toolError && (
        <div className="mb-2 p-2 rounded bg-red-900/20 border border-red-800/40 text-xs space-y-1">
          {toolError.error_code && (
            <div className="flex items-center gap-2">
              <span className="text-red-400 font-semibold">{toolError.error_code}</span>
              {toolError.retryable && (
                <span className="px-1.5 py-0.5 bg-yellow-800/30 border border-yellow-700/40 rounded text-yellow-400 text-[10px] font-medium">
                  retryable
                </span>
              )}
            </div>
          )}
          {toolError.message && (
            <div className="text-red-300">{toolError.message}</div>
          )}
          {toolError.suggestion && (
            <div className="text-gray-400">
              <span className="text-gray-500">suggestion:</span> {toolError.suggestion}
            </div>
          )}
        </div>
      )}

      <pre className="text-xs text-gray-300 bg-gray-900 p-2 rounded overflow-auto max-h-64 whitespace-pre-wrap">
        {formatted}
      </pre>
    </div>
  )
}
