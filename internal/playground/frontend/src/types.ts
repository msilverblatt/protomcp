export interface Tool {
  name: string
  description: string
  inputSchema: JsonSchema
}

export interface JsonSchema {
  type: string
  properties?: Record<string, JsonSchemaProperty>
  required?: string[]
  oneOf?: JsonSchema[]
}

export interface JsonSchemaProperty {
  type: string
  description?: string
  default?: unknown
  enum?: unknown[]
  const?: unknown
  oneOf?: JsonSchema[]
  properties?: Record<string, JsonSchemaProperty>
  required?: string[]
}

export interface Resource {
  uri: string
  name: string
  description: string
  mimeType: string
}

export interface PromptDef {
  name: string
  description: string
  arguments: PromptArgument[]
}

export interface PromptArgument {
  name: string
  description: string
  required: boolean
}

export interface TraceEntry {
  seq: number
  timestamp: string
  direction: 'send' | 'recv'
  method: string
  raw: string
}

export interface CallResult {
  result: unknown
  duration_ms: number
  tools_enabled?: string[]
  tools_disabled?: string[]
}

export interface ResourceReadResult {
  contents: unknown[]
}

export interface PromptGetResult {
  description: string
  messages: unknown[]
}

export interface HistoryItem {
  id: string
  type: 'call' | 'resource' | 'prompt' | 'system' | 'error'
  timestamp: Date
  label: string
  request?: unknown
  response?: unknown
}

export type WsEvent =
  | { type: 'trace'; data: TraceEntry }
  | { type: 'tools_changed'; data: Tool[] }
  | { type: 'reload'; data: { tool_count: number } }
  | { type: 'connection'; data: { status: string } }
