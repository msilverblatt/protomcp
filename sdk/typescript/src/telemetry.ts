export interface ToolCallEvent {
  toolName: string;
  action: string;
  phase: string;
  args: Record<string, any>;
  result?: string;
  error?: Error;
  durationMs?: number;
  progress?: number;
  total?: number;
  message?: string;
}

type SinkHandler = (event: ToolCallEvent) => void;

const sinkRegistry: SinkHandler[] = [];

export function telemetrySink(handler: SinkHandler): void {
  sinkRegistry.push(handler);
}

export function emitTelemetry(event: ToolCallEvent): void {
  for (const sink of sinkRegistry) {
    try {
      sink(event);
    } catch {
      // fail-safe: never let a sink error propagate
    }
  }
}

export function getTelemetrySinks(): SinkHandler[] {
  return [...sinkRegistry];
}

export function clearTelemetrySinks(): void {
  sinkRegistry.length = 0;
}
