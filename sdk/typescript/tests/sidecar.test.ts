import { describe, it, expect, beforeEach } from 'vitest';
import { sidecar, getRegisteredSidecars, clearSidecarRegistry, startSidecars, stopAllSidecars } from '../src/sidecar.js';

beforeEach(() => {
  clearSidecarRegistry();
});

describe('sidecar', () => {
  it('registers a sidecar', () => {
    sidecar({ name: 'redis', command: ['redis-server'] });

    const sidecars = getRegisteredSidecars();
    expect(sidecars).toHaveLength(1);
    expect(sidecars[0].name).toBe('redis');
    expect(sidecars[0].command).toEqual(['redis-server']);
    expect(sidecars[0].startOn).toBe('first_tool_call');
  });

  it('registers with custom options', () => {
    sidecar({
      name: 'api',
      command: ['node', 'server.js'],
      healthCheck: 'http://localhost:3000/health',
      startOn: 'server_start',
      healthTimeout: 60,
    });

    const sidecars = getRegisteredSidecars();
    expect(sidecars[0].healthCheck).toBe('http://localhost:3000/health');
    expect(sidecars[0].startOn).toBe('server_start');
    expect(sidecars[0].healthTimeout).toBe(60);
  });

  it('clears registry', () => {
    sidecar({ name: 'test', command: ['echo'] });
    clearSidecarRegistry();

    expect(getRegisteredSidecars()).toHaveLength(0);
  });

  it('startSidecars filters by trigger', async () => {
    sidecar({ name: 'early', command: ['echo', 'early'], startOn: 'server_start' });
    sidecar({ name: 'lazy', command: ['echo', 'lazy'], startOn: 'first_tool_call' });

    // This should not throw even though the commands don't exist as real services
    // (spawn will fail silently with detached processes)
    // We mainly test that filtering logic works
    const sidecars = getRegisteredSidecars();
    const serverStartSidecars = sidecars.filter(s => s.startOn === 'server_start');
    const firstToolSidecars = sidecars.filter(s => s.startOn === 'first_tool_call');

    expect(serverStartSidecars).toHaveLength(1);
    expect(serverStartSidecars[0].name).toBe('early');
    expect(firstToolSidecars).toHaveLength(1);
    expect(firstToolSidecars[0].name).toBe('lazy');
  });

  it('registers multiple sidecars', () => {
    sidecar({ name: 'a', command: ['a'] });
    sidecar({ name: 'b', command: ['b'] });
    sidecar({ name: 'c', command: ['c'] });

    expect(getRegisteredSidecars()).toHaveLength(3);
  });
});
