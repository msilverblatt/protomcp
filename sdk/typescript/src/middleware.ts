export interface MiddlewareDef {
  name: string;
  priority: number;
  handler: (
    phase: string,
    toolName: string,
    argsJson: string,
    resultJson: string,
    isError: boolean
  ) => Record<string, any> | void;
}

const middlewareRegistry: MiddlewareDef[] = [];

export function middleware(
  name: string,
  priority: number,
  handler: MiddlewareDef['handler']
): void {
  middlewareRegistry.push({ name, priority, handler });
}

export function getRegisteredMiddleware(): MiddlewareDef[] {
  return [...middlewareRegistry];
}

export function clearMiddlewareRegistry(): void {
  middlewareRegistry.length = 0;
}
