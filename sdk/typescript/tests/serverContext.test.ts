import { describe, it, expect, beforeEach } from 'vitest';
import { serverContext, resolveContexts, clearContextRegistry, getHiddenContextParams } from '../src/serverContext.js';

beforeEach(() => {
  clearContextRegistry();
});

describe('serverContext', () => {
  it('registers and resolves a context', () => {
    serverContext('userId', (args) => args.token?.split('-')[0] ?? 'anonymous');

    const resolved = resolveContexts({ token: 'user123-abc' });
    expect(resolved.userId).toBe('user123');
  });

  it('resolves multiple contexts', () => {
    serverContext('userId', (args) => args.user ?? 'anon');
    serverContext('timestamp', () => 12345);

    const resolved = resolveContexts({ user: 'alice' });
    expect(resolved.userId).toBe('alice');
    expect(resolved.timestamp).toBe(12345);
  });

  it('returns empty when no contexts registered', () => {
    const resolved = resolveContexts({ foo: 'bar' });
    expect(Object.keys(resolved)).toHaveLength(0);
  });

  it('tracks hidden context params (expose=false)', () => {
    serverContext('visible', () => 'v', { expose: true });
    serverContext('hidden', () => 'h', { expose: false });

    const hidden = getHiddenContextParams();
    expect(hidden.has('hidden')).toBe(true);
    expect(hidden.has('visible')).toBe(false);
  });

  it('defaults expose to true', () => {
    serverContext('param', () => 'val');

    const hidden = getHiddenContextParams();
    expect(hidden.has('param')).toBe(false);
  });

  it('clears registry', () => {
    serverContext('x', () => 1);
    clearContextRegistry();

    const resolved = resolveContexts({});
    expect(Object.keys(resolved)).toHaveLength(0);
  });
});
