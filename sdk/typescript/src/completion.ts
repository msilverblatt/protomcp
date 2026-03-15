export interface CompletionResult {
  values: string[];
  total?: number;
  hasMore?: boolean;
}

type CompletionHandler = (value: string) => CompletionResult | string[];

const completionRegistry = new Map<string, CompletionHandler>();

function makeKey(refType: string, refName: string, argName: string): string {
  return `${refType}:${refName}:${argName}`;
}

export function completion(
  refType: string,
  refName: string,
  argumentName: string,
  handler: CompletionHandler,
): void {
  completionRegistry.set(makeKey(refType, refName, argumentName), handler);
}

export function getCompletionHandler(
  refType: string,
  refName: string,
  argumentName: string,
): CompletionHandler | undefined {
  return completionRegistry.get(makeKey(refType, refName, argumentName));
}

export function clearCompletionRegistry(): void {
  completionRegistry.clear();
}
