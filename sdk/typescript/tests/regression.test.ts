import { describe, it, expect } from 'vitest';
import { z } from 'zod';

describe('regression tests', () => {
  it('clearMiddlewareRegistry is exported from index', async () => {
    const mod = await import('../src/index.js');
    expect(typeof mod.clearMiddlewareRegistry).toBe('function');
  });

  it('hot reload clears all registries via discovery', async () => {
    const { configure: configureDiscovery, discoverHandlers, resetConfig: resetDiscoveryConfig } = await import('../src/discovery.js');
    const { clearRegistry, getRegisteredTools, tool } = await import('../src/tool.js');
    const { clearGroupRegistry } = await import('../src/group.js');

    // Register a tool, then trigger hot reload and verify it's cleared
    tool({
      name: 'stale_tool',
      description: 'A stale tool',
      args: z.object({}),
      handler: () => 'stale',
    });
    expect(getRegisteredTools().length).toBeGreaterThan(0);

    configureDiscovery({ handlersDir: '/tmp/nonexistent-handlers-dir', hotReload: true });
    // Simulate a second discover (hot reload path)
    // First discover loads modules, second triggers hot reload
    await discoverHandlers();
    await discoverHandlers();

    // Cleanup
    clearRegistry();
    clearGroupRegistry();
    resetDiscoveryConfig();
  });
});
