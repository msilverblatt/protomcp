// examples/typescript/full-showcase.ts
// Full-featured protomcp demo — multiple tools showcasing the complete API.
// Run: pmcp dev examples/typescript/full-showcase.ts

import { tool, ToolResult, ToolContext, toolManager, ServerLogger } from 'protomcp';
import { z } from 'zod';

// Note: ServerLogger is wired to the MCP host transport automatically by the runner.
// For demonstration, we show how to create one — in practice, use the runner-provided instance.

// --- Tool 1: Structured output with output schema ---

const WeatherOutput = z.object({
  location: z.string(),
  temperature_f: z.number(),
  conditions: z.string(),
  humidity: z.number(),
});

tool({
  name: 'get_weather',
  description: 'Get current weather for a location',
  title: 'Weather Lookup',
  readOnlyHint: true,
  output: WeatherOutput,
  args: z.object({ location: z.string() }),
  handler({ location }) {
    const data = {
      location,
      temperature_f: 72.5,
      conditions: 'Partly cloudy',
      humidity: 45,
    };
    return new ToolResult({ result: JSON.stringify(data) });
  },
});

// --- Tool 2: Long-running operation with progress + task support ---

tool({
  name: 'analyze_dataset',
  description: 'Analyze a dataset (simulated long-running task)',
  title: 'Dataset Analyzer',
  idempotentHint: true,
  taskSupport: true,
  args: z.object({
    dataset_name: z.string(),
    depth: z.enum(['basic', 'deep']).default('basic'),
  }),
  async handler({ dataset_name, depth }, ctx: ToolContext) {
    const steps = 10;
    for (let i = 0; i < steps; i++) {
      if (ctx.isCancelled()) {
        return new ToolResult({
          result: `Analysis cancelled at step ${i}/${steps}`,
          isError: true,
          errorCode: 'CANCELLED',
          retryable: true,
        });
      }
      ctx.reportProgress(i, steps, `Analyzing step ${i + 1}/${steps}...`);
      await new Promise(r => setTimeout(r, 100)); // Simulate work
    }
    ctx.reportProgress(steps, steps, 'Analysis complete');
    return new ToolResult({
      result: JSON.stringify({
        dataset: dataset_name,
        depth,
        rows_analyzed: 15000,
        anomalies_found: 3,
        summary: 'Dataset is healthy with 3 minor anomalies detected.',
      }),
    });
  },
});

// --- Tool 3: Dynamic tool list management ---

tool({
  name: 'manage_tools',
  description: 'Enable or disable tools at runtime',
  title: 'Tool Manager',
  destructiveHint: true,
  args: z.object({
    action: z.enum(['enable', 'disable', 'list']),
    tool_names: z.string().describe('Comma-separated tool names'),
  }),
  async handler({ action, tool_names }) {
    const names = tool_names.split(',').map(n => n.trim());
    let active: string[];
    switch (action) {
      case 'enable':
        active = await toolManager.enable(names);
        break;
      case 'disable':
        active = await toolManager.disable(names);
        break;
      case 'list':
        active = await toolManager.getActiveTools();
        break;
    }
    return new ToolResult({ result: JSON.stringify({ active_tools: active }) });
  },
});

// --- Tool 4: Error handling, validation, and logging ---

tool({
  name: 'validate_data',
  description: 'Validate data against a schema (demonstrates error handling)',
  title: 'Data Validator',
  readOnlyHint: true,
  idempotentHint: true,
  args: z.object({
    data_json: z.string(),
    strict: z.boolean().default(false),
  }),
  handler({ data_json, strict }) {
    let data: unknown;
    try {
      data = JSON.parse(data_json);
    } catch (e) {
      return new ToolResult({
        result: `Invalid JSON: ${e}`,
        isError: true,
        errorCode: 'PARSE_ERROR',
        message: 'The input is not valid JSON',
        suggestion: 'Check for syntax errors and try again',
        retryable: true,
      });
    }

    const issues: string[] = [];
    if (typeof data !== 'object' || data === null || Array.isArray(data)) {
      issues.push('Root must be an object');
    } else {
      if (!('name' in data)) issues.push('Missing required field: name');
      if (strict) {
        const allowed = new Set(['name', 'value', 'tags']);
        const extra = Object.keys(data).filter(k => !allowed.has(k));
        if (extra.length) issues.push(`Unknown fields: ${extra.join(', ')}`);
      }
    }

    if (issues.length) {
      return new ToolResult({
        result: JSON.stringify({ valid: false, issues }),
        isError: true,
        errorCode: 'VALIDATION_FAILED',
      });
    }

    return new ToolResult({ result: JSON.stringify({ valid: true, issues: [] }) });
  },
});
