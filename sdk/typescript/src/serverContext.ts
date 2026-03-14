interface ContextDef {
  paramName: string;
  resolver: (args: Record<string, any>) => any;
  expose: boolean;
}

const contextRegistry: ContextDef[] = [];

export function serverContext(
  paramName: string,
  resolver: (args: Record<string, any>) => any,
  options?: { expose?: boolean },
): void {
  contextRegistry.push({
    paramName,
    resolver,
    expose: options?.expose ?? true,
  });
}

export function resolveContexts(args: Record<string, any>): Record<string, any> {
  const resolved: Record<string, any> = {};
  for (const def of contextRegistry) {
    resolved[def.paramName] = def.resolver(args);
  }
  return resolved;
}

export function getHiddenContextParams(): Set<string> {
  return new Set(
    contextRegistry.filter(c => !c.expose).map(c => c.paramName),
  );
}

export function clearContextRegistry(): void {
  contextRegistry.length = 0;
}
