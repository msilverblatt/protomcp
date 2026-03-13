// examples/typescript/real-world.ts
// A file search tool demonstrating progress, logging, and cancellation.
// Run: pmcp dev examples/typescript/real-world.ts

import { tool, ToolResult, ToolContext, ServerLogger } from 'protomcp';
import { z } from 'zod';
import * as fs from 'fs';
import * as path from 'path';

// Note: ServerLogger requires a transport send function. In a real pmcp process,
// this is wired up automatically by the runner. For demonstration purposes,
// we show the API shape — logging calls are forwarded to the MCP host.

tool({
  name: 'search_files',
  description: 'Search files in a directory by glob pattern',
  readOnlyHint: true,
  args: z.object({
    directory: z.string(),
    pattern: z.string(),
    max_results: z.number().default(50),
  }),
  handler({ directory, pattern, max_results }, ctx: ToolContext) {
    if (!fs.existsSync(directory)) {
      return new ToolResult({
        result: `Directory not found: ${directory}`,
        isError: true,
        errorCode: 'INVALID_PATH',
        message: 'The specified directory does not exist',
        suggestion: 'Check the path and try again',
      });
    }

    const matches: string[] = [];
    const allFiles: string[] = [];

    function walk(dir: string) {
      for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) walk(full);
        else allFiles.push(full);
      }
    }
    walk(directory);

    const total = allFiles.length;

    for (let i = 0; i < total; i++) {
      if (ctx.isCancelled()) {
        return new ToolResult({
          result: `Cancelled after scanning ${i}/${total} files. Found ${matches.length} matches so far.`,
          isError: true,
          errorCode: 'CANCELLED',
          retryable: true,
        });
      }

      if (i % 100 === 0) {
        ctx.reportProgress(i, total, `Scanning... ${i}/${total}`);
      }

      if (matchGlob(path.basename(allFiles[i]), pattern)) {
        matches.push(allFiles[i]);
        if (matches.length >= max_results) break;
      }
    }

    ctx.reportProgress(total, total, 'Complete');
    return new ToolResult({
      result: matches.length > 0 ? matches.join('\n') : 'No files found',
    });
  },
});

function matchGlob(filename: string, pattern: string): boolean {
  const regex = new RegExp(
    '^' + pattern.replace(/\*/g, '.*').replace(/\?/g, '.') + '$'
  );
  return regex.test(filename);
}
