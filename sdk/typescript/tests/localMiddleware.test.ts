import { describe, it, expect, beforeEach } from 'vitest';
import { localMiddleware, buildMiddlewareChain, clearLocalMiddleware } from '../src/localMiddleware.js';
import { ToolContext } from '../src/context.js';

function dummyCtx(): ToolContext {
  return new ToolContext('', () => {});
}

beforeEach(() => {
  clearLocalMiddleware();
});

describe('localMiddleware', () => {
  it('calls handler directly with no middleware', () => {
    const handler = (args: any, ctx: any) => `hello ${args.name}`;
    const chain = buildMiddlewareChain('test_tool', handler);
    const result = chain(dummyCtx(), { name: 'world' });
    expect(result).toBe('hello world');
  });

  it('wraps handler with a single middleware', () => {
    const log: string[] = [];

    localMiddleware(100, (ctx, toolName, args, next) => {
      log.push(`before:${toolName}`);
      const result = next(ctx, args);
      log.push(`after:${toolName}`);
      return result;
    });

    const handler = (args: any) => `result:${args.x}`;
    const chain = buildMiddlewareChain('my_tool', handler);
    const result = chain(null, { x: 42 });

    expect(result).toBe('result:42');
    expect(log).toEqual(['before:my_tool', 'after:my_tool']);
  });

  it('respects priority ordering (lowest first = outermost)', () => {
    const log: string[] = [];

    localMiddleware(200, (ctx, toolName, args, next) => {
      log.push('inner-before');
      const result = next(ctx, args);
      log.push('inner-after');
      return result;
    });

    localMiddleware(50, (ctx, toolName, args, next) => {
      log.push('outer-before');
      const result = next(ctx, args);
      log.push('outer-after');
      return result;
    });

    const handler = (args: any) => {
      log.push('handler');
      return 'done';
    };

    const chain = buildMiddlewareChain('tool', handler);
    chain(null, {});

    expect(log).toEqual(['outer-before', 'inner-before', 'handler', 'inner-after', 'outer-after']);
  });

  it('middleware can modify args', () => {
    localMiddleware(100, (ctx, toolName, args, next) => {
      return next(ctx, { ...args, extra: 'injected' });
    });

    const handler = (args: any) => args.extra;
    const chain = buildMiddlewareChain('tool', handler);
    const result = chain(null, { original: true });
    expect(result).toBe('injected');
  });

  it('middleware can short-circuit', () => {
    localMiddleware(100, (_ctx, _toolName, _args, _next) => {
      return 'blocked';
    });

    const handler = () => 'should not reach';
    const chain = buildMiddlewareChain('tool', handler);
    const result = chain(null, {});
    expect(result).toBe('blocked');
  });

  it('clears middleware', () => {
    localMiddleware(100, (_ctx, _toolName, _args, next) => {
      return 'mw:' + next(_ctx, _args);
    });

    clearLocalMiddleware();

    const handler = () => 'raw';
    const chain = buildMiddlewareChain('tool', handler);
    const result = chain(null, {});
    expect(result).toBe('raw');
  });
});
