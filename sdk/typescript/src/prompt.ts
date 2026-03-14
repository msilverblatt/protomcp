export interface PromptArg {
  name: string;
  description?: string;
  required?: boolean;
}

export interface PromptMessage {
  role: 'user' | 'assistant';
  content: string;
}

export interface PromptDef {
  name: string;
  description: string;
  arguments: PromptArg[];
  handler: (args: Record<string, string>) => PromptMessage | PromptMessage[];
}

const promptRegistry: PromptDef[] = [];

export interface PromptOptions {
  description: string;
  name?: string;
  arguments?: PromptArg[];
  handler: (args: Record<string, string>) => PromptMessage | PromptMessage[];
}

export function prompt(options: PromptOptions): PromptDef {
  const def: PromptDef = {
    name: options.name ?? `prompt_${promptRegistry.length}`,
    description: options.description,
    arguments: options.arguments ?? [],
    handler: options.handler,
  };
  promptRegistry.push(def);
  return def;
}

export function getRegisteredPrompts(): PromptDef[] {
  return [...promptRegistry];
}
