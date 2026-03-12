import { describe, it, expect, vi, beforeEach } from 'vitest';
import { toolManager } from '../src/manager.js';

// Mock transport
function makeMockTransport(responseToolNames: string[]) {
  return {
    send: vi.fn(),
    recv: vi.fn().mockResolvedValue({
      msg: 'activeTools',
      activeTools: { toolNames: responseToolNames },
    }),
    connect: vi.fn(),
    close: vi.fn(),
    getRoot: vi.fn(),
  };
}

describe('toolManager', () => {
  beforeEach(() => {
    toolManager._reset();
  });

  it('enable() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport(['tool_a', 'tool_b']);
    toolManager._init(mockTransport as any);

    const result = await toolManager.enable(['tool_a']);
    expect(result).toEqual(['tool_a', 'tool_b']);
    expect(mockTransport.send).toHaveBeenCalledOnce();
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('enableTools');
  });

  it('disable() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport(['tool_b']);
    toolManager._init(mockTransport as any);

    const result = await toolManager.disable(['tool_a']);
    expect(result).toEqual(['tool_b']);
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('disableTools');
  });

  it('setAllowed() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport(['tool_a']);
    toolManager._init(mockTransport as any);

    const result = await toolManager.setAllowed(['tool_a']);
    expect(result).toEqual(['tool_a']);
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('setAllowed');
  });

  it('setBlocked() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport([]);
    toolManager._init(mockTransport as any);

    const result = await toolManager.setBlocked(['tool_a']);
    expect(result).toEqual([]);
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('setBlocked');
  });

  it('getActiveTools() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport(['tool_x']);
    toolManager._init(mockTransport as any);

    const result = await toolManager.getActiveTools();
    expect(result).toEqual(['tool_x']);
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('getActiveTools');
  });

  it('batch() sends envelope and returns active tools', async () => {
    const mockTransport = makeMockTransport(['tool_a', 'tool_c']);
    toolManager._init(mockTransport as any);

    const result = await toolManager.batch({
      enable: ['tool_a'],
      disable: ['tool_b'],
      allow: [],
      block: ['tool_d'],
    });
    expect(result).toEqual(['tool_a', 'tool_c']);
    const sentEnv = mockTransport.send.mock.calls[0][0];
    expect(sentEnv.msg).toBe('batch');
  });

  it('throws if not initialized', async () => {
    await expect(toolManager.getActiveTools()).rejects.toThrow('protomcp not connected');
  });
});
