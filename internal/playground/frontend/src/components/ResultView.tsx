interface ResultViewProps {
  data: unknown
  durationMs?: number
}

export default function ResultView({ data, durationMs }: ResultViewProps) {
  const formatted = typeof data === 'string' ? data : JSON.stringify(data, null, 2)

  return (
    <div className="p-3 border-t border-gray-700">
      {durationMs !== undefined && (
        <div className="text-xs text-gray-500 mb-1">{durationMs}ms</div>
      )}
      <pre className="text-xs text-gray-300 bg-gray-900 p-2 rounded overflow-auto max-h-64 whitespace-pre-wrap">
        {formatted}
      </pre>
    </div>
  )
}
