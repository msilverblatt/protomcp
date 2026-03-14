interface TopBarProps {
  connected: boolean
  toolCount: number
  resourceCount: number
  promptCount: number
  onReload: () => void
  reloading: boolean
  startTime: Date
}

import { useState, useEffect } from 'react'

export default function TopBar({
  connected,
  toolCount,
  resourceCount,
  promptCount,
  onReload,
  reloading,
  startTime,
}: TopBarProps) {
  const [uptime, setUptime] = useState('')

  useEffect(() => {
    const tick = () => {
      const secs = Math.floor((Date.now() - startTime.getTime()) / 1000)
      const m = Math.floor(secs / 60)
      const s = secs % 60
      setUptime(`${m}:${s.toString().padStart(2, '0')}`)
    }
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [startTime])

  return (
    <div className="flex items-center justify-between px-4 py-2 bg-gray-800 border-b border-gray-700 shrink-0">
      <div className="flex items-center gap-3">
        <span
          className={`w-2.5 h-2.5 rounded-full ${connected ? 'bg-green-400' : 'bg-red-500'}`}
        />
        <span className="text-sm font-semibold text-gray-200">protomcp playground</span>
      </div>

      <div className="flex items-center gap-4 text-xs text-gray-400">
        <span>
          {toolCount} tools, {resourceCount} resources, {promptCount} prompts
        </span>
        <span>{uptime}</span>
        <button
          onClick={onReload}
          disabled={reloading}
          className="px-3 py-1 text-xs bg-gray-700 hover:bg-gray-600 rounded border border-gray-600 text-gray-200 disabled:opacity-50 cursor-pointer disabled:cursor-not-allowed"
        >
          {reloading ? 'Reloading...' : 'Reload'}
        </button>
      </div>
    </div>
  )
}
