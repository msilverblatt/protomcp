import type { Tool, Resource, PromptDef } from '../types'

type Tab = 'tools' | 'resources' | 'prompts'

interface FeaturePickerProps {
  tab: Tab
  onTabChange: (tab: Tab) => void
  tools: Tool[]
  resources: Resource[]
  prompts: PromptDef[]
  selectedName: string | null
  onSelect: (name: string) => void
}

export default function FeaturePicker({
  tab,
  onTabChange,
  tools,
  resources,
  prompts,
  selectedName,
  onSelect,
}: FeaturePickerProps) {
  const tabs: { key: Tab; label: string }[] = [
    { key: 'tools', label: 'Tools' },
    { key: 'resources', label: 'Resources' },
    { key: 'prompts', label: 'Prompts' },
  ]

  const items =
    tab === 'tools'
      ? tools.map((t) => ({ id: t.name, label: t.name, desc: t.description }))
      : tab === 'resources'
        ? resources.map((r) => ({ id: r.uri, label: r.name || r.uri, desc: r.description }))
        : prompts.map((p) => ({ id: p.name, label: p.name, desc: p.description }))

  return (
    <div className="flex flex-col">
      <div className="flex border-b border-gray-700">
        {tabs.map((t) => (
          <button
            key={t.key}
            onClick={() => onTabChange(t.key)}
            className={`px-4 py-2 text-xs font-medium cursor-pointer ${
              tab === t.key
                ? 'text-blue-400 border-b-2 border-blue-400'
                : 'text-gray-400 hover:text-gray-200'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="overflow-y-auto max-h-48">
        {items.length === 0 && (
          <p className="p-3 text-xs text-gray-500 italic">No {tab} registered</p>
        )}
        {items.map((item) => (
          <button
            key={item.id}
            onClick={() => onSelect(item.id)}
            className={`w-full text-left px-3 py-2 text-sm border-b border-gray-700/50 cursor-pointer ${
              selectedName === item.id
                ? 'bg-gray-700/70 text-white'
                : 'text-gray-300 hover:bg-gray-700/40'
            }`}
          >
            <div className="font-medium">{item.label}</div>
            {item.desc && <div className="text-xs text-gray-500 truncate">{item.desc}</div>}
          </button>
        ))}
      </div>
    </div>
  )
}
