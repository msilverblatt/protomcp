import { z } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';

export interface ToolDef<T extends z.ZodType = z.ZodType> {
  name: string;
  description: string;
  inputSchemaJson: string;
  handler: (args: z.infer<T>) => any;
}

const registry: ToolDef<any>[] = [];

interface ToolOptions<T extends z.ZodObject<any>> {
  description: string;
  args: T;
  handler: (args: z.infer<T>) => any;
  name?: string;
}

export function tool<T extends z.ZodObject<any>>(options: ToolOptions<T>): ToolDef<T> {
  const schema = zodToJsonSchema(options.args, { target: 'openApi3' });
  // Arrow functions assigned to object properties get the property key as their
  // inferred name (e.g. "handler"). Ignore that inference and treat it as unnamed.
  const inferredName = options.handler.name;
  const usableName = inferredName && inferredName !== 'handler' ? inferredName : undefined;
  const def: ToolDef<T> = {
    name: options.name || usableName || `tool_${registry.length}`,
    description: options.description,
    inputSchemaJson: JSON.stringify(schema),
    handler: options.handler,
  };
  registry.push(def);
  return def;
}

export function getRegisteredTools(): ToolDef<any>[] {
  return [...registry];
}

export function clearRegistry(): void {
  registry.length = 0;
}
