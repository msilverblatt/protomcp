import { useState, useEffect, useCallback } from 'react'
import type { Tool, Resource, PromptDef, TraceEntry, CallResult, ResourceReadResult, PromptGetResult, WsEvent } from './types'
import { useWebSocket } from './hooks/useWebSocket'
import { useApi } from './hooks/useApi'
import TopBar from './components/TopBar'
import FeaturePicker from './components/FeaturePicker'
import ToolForm from './components/ToolForm'
import ResourceForm from './components/ResourceForm'
import PromptForm from './components/PromptForm'
import TracePanel from './components/TracePanel'

type Tab = 'tools' | 'resources' | 'prompts'

export default function App() {
  const [tools, setTools] = useState<Tool[]>([])
  const [resources, setResources] = useState<Resource[]>([])
  const [prompts, setPrompts] = useState<PromptDef[]>([])
  const [tab, setTab] = useState<Tab>('tools')
  const [selectedName, setSelectedName] = useState<string | null>(null)
  const [traceEntries, setTraceEntries] = useState<TraceEntry[]>([])
  const [startTime] = useState(() => new Date())

  const callApi = useApi<CallResult>()
  const readApi = useApi<ResourceReadResult>()
  const promptApi = useApi<PromptGetResult>()
  const reloadApi = useApi<Record<string, never>>()

  const handleWsMessage = useCallback(
    (event: WsEvent) => {
      switch (event.type) {
        case 'trace':
          setTraceEntries((prev) => {
            if (prev.some((e) => e.seq === event.data.seq)) return prev
            return [...prev, event.data].sort((a, b) => a.seq - b.seq)
          })
          // Re-fetch when tool/resource/prompt list changes
          if (event.data.method?.includes('list_changed')) {
            fetchAll()
          }
          break
        case 'tools_changed':
          setTools(event.data)
          break
        case 'reload':
          fetchAll()
          break
        case 'connection':
          if (event.data.status === 'connected') fetchAll()
          break
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  const { connected } = useWebSocket(handleWsMessage)

  const fetchAll = useCallback(async () => {
    try {
      const [toolsRes, resourcesRes, promptsRes, traceRes] = await Promise.all([
        fetch('/api/tools').then((r) => r.json()),
        fetch('/api/resources').then((r) => r.json()),
        fetch('/api/prompts').then((r) => r.json()),
        fetch('/api/trace').then((r) => r.json()),
      ])
      setTools(Array.isArray(toolsRes) ? toolsRes : toolsRes?.tools ?? [])
      setResources(Array.isArray(resourcesRes) ? resourcesRes : resourcesRes?.resources ?? [])
      setPrompts(Array.isArray(promptsRes) ? promptsRes : promptsRes?.prompts ?? [])
      if (Array.isArray(traceRes)) {
        setTraceEntries(traceRes.sort((a: TraceEntry, b: TraceEntry) => a.seq - b.seq))
      }
    } catch {
      // will retry on reconnect
    }
  }, [])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  const handleToolCall = async (name: string, args: Record<string, unknown>) => {
    const result = await callApi.execute('/api/call', {
      method: 'POST',
      body: JSON.stringify({ name, args }),
    })
    if (result) {
      if (result.tools_enabled?.length || result.tools_disabled?.length) {
        fetchAll()
      }
    }
  }

  const handleResourceRead = async (uri: string) => {
    await readApi.execute('/api/resource/read', {
      method: 'POST',
      body: JSON.stringify({ uri }),
    })
  }

  const handlePromptGet = async (name: string, args: Record<string, string>) => {
    await promptApi.execute('/api/prompt/get', {
      method: 'POST',
      body: JSON.stringify({ name, arguments: args }),
    })
  }

  const handleReload = async () => {
    await reloadApi.execute('/api/reload', { method: 'POST' })
    fetchAll()
  }

  const selectedTool = tab === 'tools' ? tools.find((t) => t.name === selectedName) : undefined
  const selectedResource = tab === 'resources' ? resources.find((r) => r.uri === selectedName) : undefined
  const selectedPrompt = tab === 'prompts' ? prompts.find((p) => p.name === selectedName) : undefined

  return (
    <>
      <TopBar
        connected={connected}
        toolCount={tools.length}
        resourceCount={resources.length}
        promptCount={prompts.length}
        onReload={handleReload}
        reloading={reloadApi.loading}
        startTime={startTime}
      />
      <div className="flex flex-1 min-h-0">
        {/* Left Panel */}
        <div className="w-1/2 flex flex-col border-r border-gray-700 bg-gray-800">
          <FeaturePicker
            tab={tab}
            onTabChange={(t) => {
              setTab(t)
              setSelectedName(null)
            }}
            tools={tools}
            resources={resources}
            prompts={prompts}
            selectedName={selectedName}
            onSelect={setSelectedName}
          />

          <div className="overflow-y-auto border-t border-gray-700 shrink-0">
            {selectedTool && (
              <ToolForm tool={selectedTool} onSubmit={handleToolCall} loading={callApi.loading} />
            )}
            {selectedResource && (
              <ResourceForm
                resource={selectedResource}
                onSubmit={handleResourceRead}
                loading={readApi.loading}
              />
            )}
            {selectedPrompt && (
              <PromptForm
                prompt={selectedPrompt}
                onSubmit={handlePromptGet}
                loading={promptApi.loading}
              />
            )}
            {!selectedTool && !selectedResource && !selectedPrompt && (
              <div className="p-4 text-xs text-gray-500 italic">
                Select an item from the list above
              </div>
            )}
          </div>
        </div>

        {/* Right Panel */}
        <div className="w-1/2 bg-gray-800">
          <TracePanel
            entries={traceEntries}
            onClear={() => setTraceEntries([])}
          />
        </div>
      </div>
    </>
  )
}
