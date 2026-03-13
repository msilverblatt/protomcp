import { describe, it, expect, beforeEach } from 'vitest';
import { middleware, getRegisteredMiddleware, clearMiddlewareRegistry } from '../src/middleware';

beforeEach(() => clearMiddlewareRegistry());

describe('middleware', () => {
  it('registers a middleware', () => {
    middleware('audit', 10, () => {});
    const mws = getRegisteredMiddleware();
    expect(mws).toHaveLength(1);
    expect(mws[0].name).toBe('audit');
    expect(mws[0].priority).toBe(10);
  });

  it('returns a copy from getRegisteredMiddleware', () => {
    middleware('test', 5, () => {});
    const a = getRegisteredMiddleware();
    const b = getRegisteredMiddleware();
    expect(a).not.toBe(b);
    expect(a).toEqual(b);
  });

  it('clears registry', () => {
    middleware('temp', 1, () => {});
    expect(getRegisteredMiddleware()).toHaveLength(1);
    clearMiddlewareRegistry();
    expect(getRegisteredMiddleware()).toHaveLength(0);
  });

  it('handler is callable', () => {
    middleware('blocker', 1, (phase, toolName) => {
      if (phase === 'before' && toolName === 'delete') {
        return { reject: true, rejectReason: 'blocked' };
      }
      return {};
    });

    const mws = getRegisteredMiddleware();
    const result = mws[0].handler('before', 'delete', '{}', '', false);
    expect(result).toEqual({ reject: true, rejectReason: 'blocked' });
  });

  it('registers multiple with different priorities', () => {
    middleware('first', 1, () => {});
    middleware('second', 10, () => {});
    middleware('third', 5, () => {});

    const mws = getRegisteredMiddleware();
    expect(mws).toHaveLength(3);
    expect(mws.map(m => m.name)).toEqual(['first', 'second', 'third']);
  });
});
