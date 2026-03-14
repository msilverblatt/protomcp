import { describe, it, expect, beforeEach } from 'vitest';
import { configure, getDiscoveryConfig, resetConfig, discoverHandlers } from '../src/discovery.js';

beforeEach(() => {
  resetConfig();
});

describe('discovery', () => {
  it('configures handler discovery', () => {
    configure({ handlersDir: '/tmp/handlers', hotReload: true });

    const config = getDiscoveryConfig();
    expect(config.handlersDir).toBe('/tmp/handlers');
    expect(config.hotReload).toBe(true);
  });

  it('defaults to empty string and false', () => {
    configure({});

    const config = getDiscoveryConfig();
    expect(config.handlersDir).toBe('');
    expect(config.hotReload).toBe(false);
  });

  it('returns empty config when not configured', () => {
    const config = getDiscoveryConfig();
    expect(Object.keys(config)).toHaveLength(0);
  });

  it('resets config', () => {
    configure({ handlersDir: '/tmp/test' });
    resetConfig();

    const config = getDiscoveryConfig();
    expect(Object.keys(config)).toHaveLength(0);
  });

  it('discoverHandlers is no-op without config', async () => {
    // Should not throw
    await discoverHandlers();
  });

  it('discoverHandlers is no-op with empty handlersDir', async () => {
    configure({ handlersDir: '' });
    await discoverHandlers();
  });

  it('discoverHandlers handles non-existent directory', async () => {
    configure({ handlersDir: '/tmp/nonexistent_protomcp_test_dir_12345' });
    // Should not throw
    await discoverHandlers();
  });
});
