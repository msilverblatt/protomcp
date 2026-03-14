import { z } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
import type { ToolContext } from './context.js';
import { type ToolDef, _setGroupsToToolDefs } from './tool.js';

export interface ActionOptions<T extends z.ZodObject<any>> {
  description: string;
  args: T;
  handler: (args: z.infer<T>, ctx: ToolContext) => any;
  requires?: string[];
  enumFields?: Record<string, string[]>;
}

export interface GroupOptions {
  name: string;
  description: string;
  actions: Record<string, ActionOptions<any>>;
  strategy?: 'union' | 'separate';
}

interface GroupDef {
  name: string;
  description: string;
  actions: Record<string, ActionOptions<any>>;
  strategy: 'union' | 'separate';
}

const groupRegistry: GroupDef[] = [];

export function toolGroup(options: GroupOptions): GroupDef {
  const def: GroupDef = {
    name: options.name,
    description: options.description,
    actions: options.actions,
    strategy: options.strategy ?? 'union',
  };
  groupRegistry.push(def);
  return def;
}

export function getRegisteredGroups(): GroupDef[] {
  return [...groupRegistry];
}

export function clearGroupRegistry(): void {
  groupRegistry.length = 0;
}

function dispatchGroupAction(group: GroupDef, args: Record<string, any>, ctx: ToolContext): any {
  const actionName = args.action;
  const actionNames = Object.keys(group.actions);

  if (!actionName) {
    return {
      result: `Missing 'action' field. Available actions: ${actionNames.join(', ')}`,
      isError: true,
      errorCode: 'MISSING_ACTION',
    };
  }

  const actionDef = group.actions[actionName];
  if (actionDef) {
    const { action, ...rest } = args;
    return actionDef.handler(rest, ctx);
  }

  const suggestion = fuzzyMatch(actionName, actionNames);
  let msg = `Unknown action '${actionName}'.`;
  if (suggestion) {
    msg += ` Did you mean '${suggestion}'?`;
  }
  msg += ` Available actions: ${actionNames.join(', ')}`;
  return {
    result: msg,
    isError: true,
    errorCode: 'UNKNOWN_ACTION',
  };
}

function fuzzyMatch(input: string, candidates: string[]): string | null {
  if (candidates.length === 0) return null;
  let best = '';
  let bestDist = input.length + 10;
  for (const c of candidates) {
    const d = levenshtein(input.toLowerCase(), c.toLowerCase());
    if (d < bestDist) {
      bestDist = d;
      best = c;
    }
  }
  const threshold = Math.floor(input.length / 2) + 1;
  return bestDist <= threshold ? best : null;
}

function levenshtein(a: string, b: string): number {
  const la = a.length;
  const lb = b.length;
  const matrix: number[][] = Array.from({ length: la + 1 }, () => Array(lb + 1).fill(0));
  for (let i = 0; i <= la; i++) matrix[i][0] = i;
  for (let j = 0; j <= lb; j++) matrix[0][j] = j;
  for (let i = 1; i <= la; i++) {
    for (let j = 1; j <= lb; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      matrix[i][j] = Math.min(
        matrix[i - 1][j] + 1,
        matrix[i][j - 1] + 1,
        matrix[i - 1][j - 1] + cost,
      );
    }
  }
  return matrix[la][lb];
}

export function groupsToToolDefs(): ToolDef<any>[] {
  const defs: ToolDef<any>[] = [];
  for (const group of groupRegistry) {
    if (group.strategy === 'separate') {
      defs.push(...groupToSeparateDefs(group));
    } else {
      defs.push(groupToUnionDef(group));
    }
  }
  return defs;
}

function groupToUnionDef(group: GroupDef): ToolDef<any> {
  const actionNames = Object.keys(group.actions);
  const oneOf: any[] = [];

  for (const [actionName, actionOpts] of Object.entries(group.actions)) {
    const actionSchema = zodToJsonSchema(actionOpts.args, { target: 'openApi3' }) as any;
    const props = {
      action: { const: actionName },
      ...(actionSchema.properties || {}),
    };
    const required = ['action', ...(actionSchema.required || [])];
    oneOf.push({
      type: 'object',
      properties: props,
      required,
    });
  }

  const schema = {
    type: 'object',
    properties: {
      action: {
        type: 'string',
        enum: actionNames,
      },
    },
    required: ['action'],
    oneOf,
  };

  const actionList = actionNames.join(', ');
  const desc = group.description
    ? `${group.description} Actions: ${actionList}`
    : `Actions: ${actionList}`;

  return {
    name: group.name,
    description: desc,
    inputSchemaJson: JSON.stringify(schema),
    outputSchemaJson: '',
    title: '',
    destructiveHint: false,
    idempotentHint: false,
    readOnlyHint: false,
    openWorldHint: false,
    taskSupport: false,
    handler: (args: any, ctx: ToolContext) => dispatchGroupAction(group, args, ctx),
  };
}

function groupToSeparateDefs(group: GroupDef): ToolDef<any>[] {
  const defs: ToolDef<any>[] = [];
  for (const [actionName, actionOpts] of Object.entries(group.actions)) {
    const schema = zodToJsonSchema(actionOpts.args, { target: 'openApi3' });
    defs.push({
      name: `${group.name}.${actionName}`,
      description: actionOpts.description || `${group.name} ${actionName}`,
      inputSchemaJson: JSON.stringify(schema),
      outputSchemaJson: '',
      title: '',
      destructiveHint: false,
      idempotentHint: false,
      readOnlyHint: false,
      openWorldHint: false,
      taskSupport: false,
      handler: (args: any, ctx: ToolContext) => actionOpts.handler(args, ctx),
    });
  }
  return defs;
}

// Register with tool.ts so getRegisteredTools() includes group defs.
_setGroupsToToolDefs(groupsToToolDefs);
