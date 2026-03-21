import { z } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
import type { ToolContext } from './context.js';
import { type ToolDef, _setGroupsToToolDefs } from './tool.js';
import { ToolResult } from './result.js';
import { resolveContexts } from './serverContext.js';

export interface CrossRule {
  condition: (args: Record<string, any>) => boolean;
  message: string;
}

export interface HintDef {
  condition: (args: Record<string, any>) => boolean;
  message: string;
}

export interface ActionOptions<T extends z.ZodObject<any>> {
  description: string;
  args: T;
  handler: (args: z.infer<T>, ctx: ToolContext) => any;
  requires?: string[];
  enumFields?: Record<string, string[]>;
  crossRules?: CrossRule[];
  hints?: Record<string, HintDef>;
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
    strategy: options.strategy ?? 'separate',
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

function validateAction(actionDef: ActionOptions<any>, args: Record<string, any>): ToolResult | null {
  // Check requires
  if (actionDef.requires) {
    for (const fieldName of actionDef.requires) {
      const val = args[fieldName];
      if (val === undefined || val === null || val === '') {
        return new ToolResult({
          result: `Missing required field: ${fieldName}`,
          isError: true,
          errorCode: 'MISSING_REQUIRED',
          message: `Missing required field: ${fieldName}`,
        });
      }
    }
  }

  // Check enumFields
  if (actionDef.enumFields) {
    for (const [fieldName, valid] of Object.entries(actionDef.enumFields)) {
      const val = args[fieldName];
      if (val !== undefined && val !== null && !valid.includes(val)) {
        const suggestion = fuzzyMatch(String(val), valid.map(String));
        const suggestionText = suggestion ? `Did you mean '${suggestion}'?` : undefined;
        return new ToolResult({
          result: `Invalid value '${val}' for field '${fieldName}'. Valid options: ${valid.join(', ')}`,
          isError: true,
          errorCode: 'INVALID_ENUM',
          message: `Invalid value '${val}' for field '${fieldName}'.`,
          suggestion: suggestionText,
        });
      }
    }
  }

  // Check crossRules
  if (actionDef.crossRules) {
    for (const rule of actionDef.crossRules) {
      if (rule.condition(args)) {
        return new ToolResult({
          result: rule.message,
          isError: true,
          errorCode: 'CROSS_PARAM_VIOLATION',
          message: rule.message,
        });
      }
    }
  }

  return null;
}

function collectHints(actionDef: ActionOptions<any>, args: Record<string, any>): string[] {
  const messages: string[] = [];
  if (actionDef.hints) {
    for (const hintDef of Object.values(actionDef.hints)) {
      if (hintDef.condition(args)) {
        messages.push(hintDef.message);
      }
    }
  }
  return messages;
}

function dispatchGroupAction(group: GroupDef, args: Record<string, any>, ctx: ToolContext): any {
  const actionName = args.action;
  const actionNames = Object.keys(group.actions);

  if (!actionName) {
    return new ToolResult({
      result: `Missing 'action' field. Available actions: ${actionNames.join(', ')}`,
      isError: true,
      errorCode: 'MISSING_ACTION',
    });
  }

  const actionDef = group.actions[actionName];
  if (actionDef) {
    const { action, ...rest } = args;

    // Resolve server contexts
    const ctxValues = resolveContexts(rest);
    for (const [paramName, value] of Object.entries(ctxValues)) {
      rest[paramName] = value;
    }

    // Validate before calling handler
    const validationError = validateAction(actionDef, rest);
    if (validationError !== null) {
      return validationError;
    }

    // Collect hints
    const hints = collectHints(actionDef, rest);

    const result = actionDef.handler(rest, ctx);

    // Append hints if any
    if (hints.length > 0) {
      const hintText = '\n\n**Hints:**\n' + hints.map(m => `- ${m}`).join('\n');
      if (result instanceof ToolResult) {
        return new ToolResult({
          result: result.result + hintText,
          isError: result.isError,
          enableTools: result.enableTools,
          disableTools: result.disableTools,
          errorCode: result.errorCode,
          message: result.message,
          suggestion: result.suggestion,
          retryable: result.retryable,
        });
      } else {
        return new ToolResult({ result: String(result) + hintText });
      }
    }

    return result;
  }

  const suggestion = fuzzyMatch(actionName, actionNames);
  let msg = `Unknown action '${actionName}'.`;
  if (suggestion) {
    msg += ` Did you mean '${suggestion}'?`;
  }
  msg += ` Available actions: ${actionNames.join(', ')}`;
  return new ToolResult({
    result: msg,
    isError: true,
    errorCode: 'UNKNOWN_ACTION',
  });
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
