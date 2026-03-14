import type { ToolContext } from './context.js';

interface LocalMiddlewareDef {
  priority: number;
  handler: (
    ctx: ToolContext | null,
    toolName: string,
    args: Record<string, any>,
    next: (ctx: ToolContext | null, args: Record<string, any>) => any,
  ) => any;
}

const localMwRegistry: LocalMiddlewareDef[] = [];

export function localMiddleware(
  priority: number,
  handler: LocalMiddlewareDef['handler'],
): void {
  localMwRegistry.push({ priority, handler });
}

export function getLocalMiddleware(): LocalMiddlewareDef[] {
  return [...localMwRegistry].sort((a, b) => a.priority - b.priority);
}

export function clearLocalMiddleware(): void {
  localMwRegistry.length = 0;
}

export function buildMiddlewareChain(
  toolName: string,
  handler: Function,
): (ctx: ToolContext | null, args: Record<string, any>) => any {
  const middlewares = getLocalMiddleware();

  let chain: (ctx: ToolContext | null, args: Record<string, any>) => any = (ctx, args) => {
    if (ctx !== null) {
      return handler(args, ctx);
    }
    return handler(args);
  };

  for (let i = middlewares.length - 1; i >= 0; i--) {
    const mwHandler = middlewares[i].handler;
    const nextFn = chain;
    chain = (ctx: ToolContext | null, args: Record<string, any>) => {
      const callNext = (c: ToolContext | null, a: Record<string, any>) => nextFn(c, a);
      return mwHandler(ctx, toolName, args, callNext);
    };
  }

  return chain;
}
