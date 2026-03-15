import * as fs from 'fs';
import * as path from 'path';

interface DiscoveryConfig {
  handlersDir: string;
  hotReload: boolean;
}

let config: DiscoveryConfig | null = null;
const loadedModules: Map<string, any> = new Map();

export function configure(options: { handlersDir?: string; hotReload?: boolean }): void {
  config = {
    handlersDir: options.handlersDir ?? '',
    hotReload: options.hotReload ?? false,
  };
}

export function getDiscoveryConfig(): Record<string, any> {
  if (!config) return {};
  return { handlersDir: config.handlersDir, hotReload: config.hotReload };
}

export function resetConfig(): void {
  config = null;
  loadedModules.clear();
}

export async function discoverHandlers(): Promise<void> {
  if (!config || !config.handlersDir) return;

  const handlersPath = path.resolve(config.handlersDir);
  if (!fs.existsSync(handlersPath) || !fs.statSync(handlersPath).isDirectory()) return;

  if (config.hotReload && loadedModules.size > 0) {
    const { clearRegistry } = await import('./tool.js');
    const { clearGroupRegistry } = await import('./group.js');
    const { clearWorkflowRegistry } = await import('./workflow.js');
    const { clearContextRegistry } = await import('./serverContext.js');
    const { clearLocalMiddleware } = await import('./localMiddleware.js');
    const { clearResourceRegistry, clearTemplateRegistry } = await import('./resource.js');
    const { clearPromptRegistry } = await import('./prompt.js');
    const { clearCompletionRegistry } = await import('./completion.js');
    const { clearTelemetrySinks } = await import('./telemetry.js');
    const { clearSidecarRegistry } = await import('./sidecar.js');
    const { clearMiddlewareRegistry } = await import('./middleware.js');
    clearRegistry();
    clearGroupRegistry();
    clearWorkflowRegistry();
    clearContextRegistry();
    clearLocalMiddleware();
    clearResourceRegistry();
    clearTemplateRegistry();
    clearPromptRegistry();
    clearCompletionRegistry();
    clearTelemetrySinks();
    clearSidecarRegistry();
    clearMiddlewareRegistry();
    loadedModules.clear();
  }

  const entries = fs.readdirSync(handlersPath).sort();
  for (const entry of entries) {
    if (entry.startsWith('_')) continue;
    if (!entry.endsWith('.ts') && !entry.endsWith('.js')) continue;

    const fullPath = path.join(handlersPath, entry);
    const stat = fs.statSync(fullPath);
    if (!stat.isFile()) continue;

    try {
      const mod = await import(fullPath);
      loadedModules.set(fullPath, mod);
    } catch {
      // skip files that fail to import
    }
  }
}
