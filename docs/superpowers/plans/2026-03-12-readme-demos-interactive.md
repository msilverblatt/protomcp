# README, Working Demos & Interactive Demo Page — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a README, runnable examples at three tiers in Python+TypeScript, and an animated interactive demo page in the Starlight docs site.

**Architecture:** Three independent deliverables. Examples use the protomcp Python/TypeScript SDKs directly. The demo page uses Astro components embedded in a Starlight MDX page with CSS animations and minimal JS for tab toggling.

**Tech Stack:** Markdown (README), Python/TypeScript (examples), Astro/Starlight (demo page), CSS @keyframes (animations), bash (demo runner)

---

## Chunk 1: Working Code Examples

### Task 1: Basic Python Example

**Files:**
- Create: `examples/python/basic.py`

- [ ] **Step 1: Create the basic Python example**

```python
# examples/python/basic.py
# A minimal protomcp tool — adds two numbers.
# Run: pmcp dev examples/python/basic.py

from protomcp import tool, ToolResult

@tool("Add two numbers")
def add(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a + b))

@tool("Multiply two numbers")
def multiply(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a * b))
```

- [ ] **Step 2: Verify it parses**

Run: `python -c "import ast; ast.parse(open('examples/python/basic.py').read()); print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add examples/python/basic.py
git commit -m "examples: add basic Python example"
```

---

### Task 2: Basic TypeScript Example

**Files:**
- Create: `examples/typescript/basic.ts`

- [ ] **Step 1: Create the basic TypeScript example**

```typescript
// examples/typescript/basic.ts
// A minimal protomcp tool — adds two numbers.
// Run: pmcp dev examples/typescript/basic.ts

import { tool, ToolResult } from 'protomcp';
import { z } from 'zod';

tool({
  name: 'add',
  description: 'Add two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a + b) });
  },
});

tool({
  name: 'multiply',
  description: 'Multiply two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a * b) });
  },
});
```

- [ ] **Step 2: Commit**

```bash
git add examples/typescript/basic.ts
git commit -m "examples: add basic TypeScript example"
```

---

### Task 3: Real-World Python Example

**Files:**
- Create: `examples/python/real_world.py`

- [ ] **Step 1: Create the real-world Python example**

This is a file search tool that demonstrates progress reporting, logging, and cancellation.

```python
# examples/python/real_world.py
# A file search tool demonstrating progress, logging, and cancellation.
# Run: pmcp dev examples/python/real_world.py

import os
import fnmatch
from protomcp import tool, ToolResult, ToolContext, log

@tool("Search files in a directory by glob pattern", read_only=True)
def search_files(ctx: ToolContext, directory: str, pattern: str, max_results: int = 50) -> ToolResult:
    log.info(f"Searching {directory} for '{pattern}'")

    if not os.path.isdir(directory):
        return ToolResult(
            result=f"Directory not found: {directory}",
            is_error=True,
            error_code="INVALID_PATH",
            message="The specified directory does not exist",
            suggestion="Check the path and try again",
        )

    matches = []
    all_files = []
    for root, dirs, files in os.walk(directory):
        for f in files:
            all_files.append(os.path.join(root, f))

    total = len(all_files)
    log.debug(f"Found {total} files to scan")

    for i, filepath in enumerate(all_files):
        if ctx.is_cancelled():
            log.warning("Search cancelled by client")
            return ToolResult(
                result=f"Cancelled after scanning {i}/{total} files. Found {len(matches)} matches so far.",
                is_error=True,
                error_code="CANCELLED",
                retryable=True,
            )

        if i % 100 == 0:
            ctx.report_progress(i, total, f"Scanning... {i}/{total}")

        if fnmatch.fnmatch(os.path.basename(filepath), pattern):
            matches.append(filepath)
            if len(matches) >= max_results:
                log.info(f"Hit max_results={max_results}, stopping early")
                break

    ctx.report_progress(total, total, "Complete")
    log.info(f"Search complete: {len(matches)} matches")
    return ToolResult(result="\n".join(matches) if matches else "No files found")
```

- [ ] **Step 2: Verify it parses**

Run: `python -c "import ast; ast.parse(open('examples/python/real_world.py').read()); print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add examples/python/real_world.py
git commit -m "examples: add real-world Python example with progress and cancellation"
```

---

### Task 4: Real-World TypeScript Example

**Files:**
- Create: `examples/typescript/real-world.ts`

- [ ] **Step 1: Create the real-world TypeScript example**

```typescript
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
```

- [ ] **Step 2: Commit**

```bash
git add examples/typescript/real-world.ts
git commit -m "examples: add real-world TypeScript example with progress and cancellation"
```

---

### Task 5: Full Showcase Python Example

**Files:**
- Create: `examples/python/full_showcase.py`

- [ ] **Step 1: Create the full showcase Python example**

Multi-tool server demonstrating structured output, dynamic tool lists, metadata, progress, cancellation, and logging.

```python
# examples/python/full_showcase.py
# Full-featured protomcp demo — multiple tools showcasing the complete API.
# Run: pmcp dev examples/python/full_showcase.py

import json
import time
from dataclasses import dataclass
from protomcp import tool, ToolResult, ToolContext, log
from protomcp import tool_manager

# --- Tool 1: Structured output with output schema ---

@dataclass
class WeatherData:
    location: str
    temperature_f: float
    conditions: str
    humidity: int

@tool(
    "Get current weather for a location",
    output_type=WeatherData,
    read_only=True,
    title="Weather Lookup",
)
def get_weather(location: str) -> ToolResult:
    log.info(f"Weather lookup for {location}")
    # Simulated weather data
    data = WeatherData(
        location=location,
        temperature_f=72.5,
        conditions="Partly cloudy",
        humidity=45,
    )
    return ToolResult(result=json.dumps({
        "location": data.location,
        "temperature_f": data.temperature_f,
        "conditions": data.conditions,
        "humidity": data.humidity,
    }))

# --- Tool 2: Long-running operation with progress ---

@tool(
    "Analyze a dataset (simulated long-running task)",
    title="Dataset Analyzer",
    idempotent=True,
    task_support=True,
)
def analyze_dataset(ctx: ToolContext, dataset_name: str, depth: str = "basic") -> ToolResult:
    log.info(f"Starting analysis of {dataset_name} at depth={depth}")
    steps = 10

    for i in range(steps):
        if ctx.is_cancelled():
            log.warning(f"Analysis cancelled at step {i}/{steps}")
            return ToolResult(
                result=f"Analysis cancelled at step {i}/{steps}",
                is_error=True,
                error_code="CANCELLED",
                retryable=True,
            )
        ctx.report_progress(i, steps, f"Analyzing step {i+1}/{steps}...")
        time.sleep(0.1)  # Simulate work

    ctx.report_progress(steps, steps, "Analysis complete")
    log.info("Analysis finished successfully")
    return ToolResult(result=json.dumps({
        "dataset": dataset_name,
        "depth": depth,
        "rows_analyzed": 15000,
        "anomalies_found": 3,
        "summary": "Dataset is healthy with 3 minor anomalies detected.",
    }))

# --- Tool 3: Dynamic tool list management ---

@tool(
    "Enable or disable tools at runtime",
    title="Tool Manager",
    destructive=True,
)
def manage_tools(action: str, tool_names: str) -> ToolResult:
    names = [n.strip() for n in tool_names.split(",")]
    log.info(f"manage_tools: action={action}, names={names}")

    if action == "enable":
        active = tool_manager.enable(names)
    elif action == "disable":
        active = tool_manager.disable(names)
    elif action == "list":
        active = tool_manager.get_active_tools()
    else:
        return ToolResult(
            result=f"Unknown action: {action}",
            is_error=True,
            error_code="INVALID_ACTION",
            suggestion="Use 'enable', 'disable', or 'list'",
        )

    return ToolResult(result=json.dumps({"active_tools": active}))

# --- Tool 4: Demonstrates error handling and logging levels ---

@tool(
    "Validate data against a schema (demonstrates error handling)",
    title="Data Validator",
    read_only=True,
    idempotent=True,
)
def validate_data(data_json: str, strict: bool = False) -> ToolResult:
    log.debug("Starting validation")

    try:
        data = json.loads(data_json)
    except json.JSONDecodeError as e:
        log.error(f"Invalid JSON: {e}")
        return ToolResult(
            result=f"Invalid JSON: {e}",
            is_error=True,
            error_code="PARSE_ERROR",
            message="The input is not valid JSON",
            suggestion="Check for syntax errors and try again",
            retryable=True,
        )

    issues = []
    if not isinstance(data, dict):
        issues.append("Root must be an object")
    elif "name" not in data:
        issues.append("Missing required field: name")

    if strict and isinstance(data, dict):
        allowed = {"name", "value", "tags"}
        extra = set(data.keys()) - allowed
        if extra:
            issues.append(f"Unknown fields: {', '.join(extra)}")

    if issues:
        log.warning(f"Validation failed: {issues}")
        return ToolResult(
            result=json.dumps({"valid": False, "issues": issues}),
            is_error=True,
            error_code="VALIDATION_FAILED",
        )

    log.info("Validation passed")
    return ToolResult(result=json.dumps({"valid": True, "issues": []}))
```

- [ ] **Step 2: Verify it parses**

Run: `python -c "import ast; ast.parse(open('examples/python/full_showcase.py').read()); print('OK')"`
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add examples/python/full_showcase.py
git commit -m "examples: add full showcase Python example with structured output, dynamic tools, and error handling"
```

---

### Task 6: Full Showcase TypeScript Example

**Files:**
- Create: `examples/typescript/full-showcase.ts`

- [ ] **Step 1: Create the full showcase TypeScript example**

```typescript
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
```

- [ ] **Step 2: Commit**

```bash
git add examples/typescript/full-showcase.ts
git commit -m "examples: add full showcase TypeScript example with structured output, dynamic tools, and error handling"
```

---

### Task 7: Example Dependency Files

**Files:**
- Create: `examples/python/requirements.txt`
- Create: `examples/typescript/package.json`
- Create: `examples/typescript/tsconfig.json`

- [ ] **Step 1: Create Python requirements.txt**

```
protomcp
```

- [ ] **Step 2: Create TypeScript package.json**

```json
{
  "name": "protomcp-examples",
  "private": true,
  "type": "module",
  "dependencies": {
    "protomcp": "file:../../sdk/typescript",
    "zod": "^3.22.0"
  }
}
```

Note: Uses `file:` reference to the local SDK so examples work without publishing to npm.

- [ ] **Step 3: Create TypeScript tsconfig.json**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ES2022",
    "moduleResolution": "node",
    "esModuleInterop": true,
    "strict": true,
    "outDir": "dist"
  },
  "include": ["*.ts"]
}
```

- [ ] **Step 4: Commit**

```bash
git add examples/python/requirements.txt examples/typescript/package.json examples/typescript/tsconfig.json
git commit -m "examples: add dependency files for Python and TypeScript examples"
```

---

### Task 8: Demo Runner Script

**Files:**
- Create: `examples/run-demo.sh`

- [ ] **Step 1: Create the demo runner script**

This script starts `pmcp dev` for each example, sends JSON-RPC messages over stdio, and prints human-readable output. It performs the full MCP handshake: `initialize` → `initialized` notification → `tools/list` → `tools/call`.

```bash
#!/usr/bin/env bash
set -euo pipefail

# examples/run-demo.sh
# Runs each example through pmcp, demonstrating the MCP protocol interaction.
# Requires: pmcp installed and on PATH

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASSED=0
FAILED=0

if ! command -v pmcp &> /dev/null; then
    echo -e "${RED}Error: pmcp not found. Install it first: brew install protomcp/tap/protomcp${NC}"
    exit 1
fi

# Send a JSON-RPC request to a pmcp process and capture the response.
# Usage: run_example <label> <file> <tool_name> <args_json>
run_example() {
    local label="$1"
    local file="$2"
    local tool_name="$3"
    local args_json="$4"

    echo -e "\n${CYAN}━━━ ${label} ━━━${NC}"
    echo -e "  File: ${file}"
    echo -e "  Tool: ${tool_name}"
    echo -e "  Args: ${args_json}"

    # Build JSON-RPC messages
    local init_req='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"demo","version":"1.0"}}}'
    local init_notif='{"jsonrpc":"2.0","method":"notifications/initialized"}'
    local list_req='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    local call_req="{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"${tool_name}\",\"arguments\":${args_json}}}"

    # Send all messages and capture output
    local output
    output=$(printf '%s\n%s\n%s\n%s\n' "$init_req" "$init_notif" "$list_req" "$call_req" | \
        timeout 10 pmcp dev "$file" 2>/dev/null || true)

    if [ -z "$output" ]; then
        echo -e "  ${RED}✗ No response${NC}"
        FAILED=$((FAILED + 1))
        return
    fi

    # Extract the tools/call response (id: 3)
    local result
    result=$(echo "$output" | grep '"id":3' | head -1 || true)

    if [ -n "$result" ]; then
        echo -e "  ${GREEN}✓ Result:${NC}"
        echo "    $result" | python3 -m json.tool 2>/dev/null || echo "    $result"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${YELLOW}⚠ Could not extract result (raw output below)${NC}"
        echo "$output" | head -5
        FAILED=$((FAILED + 1))
    fi
}

echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
echo -e "${CYAN}║       protomcp Demo Runner           ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"

# Python examples
run_example "Python: Basic (add)" \
    "${SCRIPT_DIR}/python/basic.py" \
    "add" \
    '{"a": 5, "b": 3}'

run_example "Python: Real-World (file search)" \
    "${SCRIPT_DIR}/python/real_world.py" \
    "search_files" \
    "{\"directory\": \"${SCRIPT_DIR}\", \"pattern\": \"*.py\"}"

run_example "Python: Full Showcase (weather)" \
    "${SCRIPT_DIR}/python/full_showcase.py" \
    "get_weather" \
    '{"location": "San Francisco"}'

run_example "Python: Full Showcase (validate)" \
    "${SCRIPT_DIR}/python/full_showcase.py" \
    "validate_data" \
    '{"data_json": "{\"name\": \"test\", \"value\": 42}", "strict": true}'

# TypeScript examples
run_example "TypeScript: Basic (add)" \
    "${SCRIPT_DIR}/typescript/basic.ts" \
    "add" \
    '{"a": 5, "b": 3}'

run_example "TypeScript: Real-World (file search)" \
    "${SCRIPT_DIR}/typescript/real-world.ts" \
    "search_files" \
    "{\"directory\": \"${SCRIPT_DIR}\", \"pattern\": \"*.ts\"}"

run_example "TypeScript: Full Showcase (weather)" \
    "${SCRIPT_DIR}/typescript/full-showcase.ts" \
    "get_weather" \
    '{"location": "San Francisco"}'

run_example "TypeScript: Full Showcase (validate)" \
    "${SCRIPT_DIR}/typescript/full-showcase.ts" \
    "validate_data" \
    '{"data_json": "{\"name\": \"test\", \"value\": 42}", "strict": true}'

# Summary
echo -e "\n${CYAN}━━━ Summary ━━━${NC}"
echo -e "  ${GREEN}Passed: ${PASSED}${NC}"
if [ "$FAILED" -gt 0 ]; then
    echo -e "  ${RED}Failed: ${FAILED}${NC}"
else
    echo -e "  Failed: 0"
fi
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x examples/run-demo.sh`

- [ ] **Step 3: Commit**

```bash
git add examples/run-demo.sh
git commit -m "examples: add demo runner script with MCP handshake"
```

---

### Task 9: Examples README

**Files:**
- Create: `examples/README.md`

- [ ] **Step 1: Create the examples README**

```markdown
# protomcp Examples

Working examples for [protomcp](https://github.com/msilverblatt/protomcp) — a language-agnostic MCP runtime.

## Quick Start

Each example can be run directly with `pmcp dev`:

```sh
# Python
pmcp dev examples/python/basic.py

# TypeScript
pmcp dev examples/typescript/basic.ts
```

## Examples

| Example | Python | TypeScript | Features |
|---------|--------|------------|----------|
| **Basic** | `python/basic.py` | `typescript/basic.ts` | `@tool` decorator, `ToolResult` |
| **Real-World** | `python/real_world.py` | `typescript/real-world.ts` | Progress reporting, cancellation, logging, error codes |
| **Full Showcase** | `python/full_showcase.py` | `typescript/full-showcase.ts` | Structured output, dynamic tool lists, metadata/annotations, validation |

## Run All Demos

```sh
./examples/run-demo.sh
```

This starts `pmcp dev` for each example, sends MCP protocol messages, and prints the results.

## Prerequisites

- [pmcp](https://github.com/msilverblatt/protomcp) installed (`brew install protomcp/tap/protomcp`)
- Python 3.10+ (for Python examples)
- Node.js 18+ (for TypeScript examples)

## Links

- [Documentation](https://github.com/msilverblatt/protomcp)
- [Python Guide](https://github.com/msilverblatt/protomcp/tree/master/docs)
- [TypeScript Guide](https://github.com/msilverblatt/protomcp/tree/master/docs)
```

- [ ] **Step 2: Commit**

```bash
git add examples/README.md
git commit -m "examples: add README with feature matrix"
```

---

## Chunk 2: README.md

### Task 10: Create README.md

**Files:**
- Create: `README.md` (repo root)

- [ ] **Step 1: Create the README**

```markdown
# protomcp

[![Build](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![npm](https://img.shields.io/npm/v/protomcp)](https://www.npmjs.com/package/protomcp)
[![PyPI](https://img.shields.io/pypi/v/protomcp)](https://pypi.org/project/protomcp/)

**Language-agnostic MCP runtime** — write tools in any language, hot-reload without restarting your AI host.

## How It Works

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│             │  MCP     │              │ protobuf │              │
│  MCP Host   │◄───────►│    pmcp      │◄────────►│  Your Code   │
│  (Claude,   │ JSON-RPC │   (Go)      │  unix    │  (any lang)  │
│   Cursor…)  │  stdio   │             │  socket  │              │
└─────────────┘         └──────────────┘         └──────────────┘
```

pmcp sits between your MCP host and your tool process. It speaks MCP (JSON-RPC over stdio) on one side and a simple protobuf protocol over a unix socket on the other. Your tool process registers handlers, and pmcp handles everything else: listing tools, routing calls, hot reload, and dynamic tool management.

## Quick Start

### Install

```sh
brew install protomcp/tap/protomcp
```

### Python

```python
# tools.py
from protomcp import tool, ToolResult

@tool("Add two numbers")
def add(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a + b))
```

```sh
pmcp dev tools.py
```

### TypeScript

```typescript
// tools.ts
import { tool, ToolResult } from 'protomcp';
import { z } from 'zod';

tool({
  name: 'add',
  description: 'Add two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a + b) });
  },
});
```

```sh
pmcp dev tools.ts
```

Then add either `pmcp dev` command to your MCP client config. That's it.

**[See it in action →](docs/src/content/docs/demo.mdx)** — animated architecture diagram, terminal replay, and protocol view.

## Features

- **Any Language** — write tools in Python, TypeScript, Go, Rust, or any language that speaks protobuf over a unix socket
- **Hot Reload** — save your file and tools reload instantly, no restart needed
- **Dynamic Tool Lists** — tools can enable/disable themselves at runtime based on context
- **5 Transports** — stdio, SSE, streamable HTTP, WebSocket, gRPC
- **Structured Output** — define output schemas for typed tool results
- **Async Tasks** — long-running operations with background task tracking
- **Progress & Cancellation** — report progress and respond to cancellation requests
- **Server Logging** — 8 RFC 5424 log levels forwarded to the MCP host
- **Tool Metadata** — annotations for destructive, read-only, idempotent, and open-world hints

## Comparison

| Feature | pmcp | FastMCP (Python) | MCP SDKs |
|---------|------|------------------|----------|
| Language support | Any (protobuf) | Python only | One SDK per language |
| Hot reload | Built-in | No | No |
| Dynamic tool lists | Built-in | No | Manual |
| Transports | stdio, SSE, HTTP, WS, gRPC | stdio, SSE | Varies by SDK |
| Structured output | Yes | No | Varies |
| Async tasks | Yes | No | No |
| Single binary | Yes (Go) | No (Python runtime) | No (per-language) |

## Examples

See [`examples/`](examples/) for runnable demos at three levels:

- **Basic** — minimal single-tool examples in Python and TypeScript
- **Real-world** — file search tool with progress reporting, cancellation, and logging
- **Full showcase** — multi-tool server with structured output, dynamic tool lists, error handling, and metadata

Run all demos: `./examples/run-demo.sh`

## Documentation

Full documentation is available in the [`docs/`](docs/) directory, built with [Starlight](https://starlight.astro.build):

- [Quick Start](docs/src/content/docs/getting-started/quick-start.mdx)
- [Python Guide](docs/src/content/docs/guides/writing-tools-python.mdx)
- [TypeScript Guide](docs/src/content/docs/guides/writing-tools-typescript.mdx)
- [CLI Reference](docs/src/content/docs/reference/cli.mdx)
- [Protobuf Spec](docs/src/content/docs/reference/protobuf-spec.mdx)

## License

MIT
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add README with architecture diagram, quick start, and comparison table"
```

---

## Chunk 3: Interactive Demo Page — Components

### Task 11: ArchitectureHero Component

**Files:**
- Create: `docs/src/components/demo/ArchitectureHero.astro`

- [ ] **Step 1: Create the ArchitectureHero component**

This is the large animated hero at the top of the demo page. Three boxes with animated message packets flowing between them. Pure CSS animation.

```astro
---
// docs/src/components/demo/ArchitectureHero.astro
// Animated architecture diagram showing MCP Host → pmcp → Your Code
---

<div class="hero-container">
  <div class="architecture">
    <div class="node node-host">
      <div class="node-label">MCP Host</div>
      <div class="node-sub">Claude, Cursor, etc.</div>
    </div>

    <div class="connector connector-left">
      <div class="connector-label">JSON-RPC</div>
      <div class="connector-line"></div>
      <div class="packet packet-request"></div>
      <div class="packet packet-response"></div>
    </div>

    <div class="node node-pmcp">
      <div class="node-label">pmcp</div>
      <div class="node-sub">Go binary</div>
    </div>

    <div class="connector connector-right">
      <div class="connector-label">protobuf</div>
      <div class="connector-line"></div>
      <div class="packet packet-request"></div>
      <div class="packet packet-response"></div>
    </div>

    <div class="node node-code">
      <div class="node-label">Your Code</div>
      <div class="node-sub">Python, TS, Go…</div>
    </div>
  </div>

  <p class="hero-text">
    pmcp sits between your MCP host and your tool process. It translates JSON-RPC to protobuf,
    handles hot reload, and manages tool registration.
  </p>
</div>

<style>
  .hero-container {
    padding: 2rem 0 1rem;
    text-align: center;
  }

  .architecture {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0;
    padding: 2rem 1rem;
    max-width: 800px;
    margin: 0 auto;
  }

  .node {
    padding: 1.2rem 1.5rem;
    border-radius: 12px;
    background: var(--sl-color-bg-nav);
    border: 2px solid;
    text-align: center;
    min-width: 130px;
    z-index: 1;
  }

  .node-host { border-color: #64ffda; }
  .node-pmcp { border-color: #ffd700; }
  .node-code { border-color: #ff6b6b; }

  .node-label {
    font-weight: 700;
    font-size: 1.1rem;
    color: var(--sl-color-text);
  }

  .node-sub {
    font-size: 0.8rem;
    color: var(--sl-color-text-accent);
    margin-top: 0.25rem;
  }

  .connector {
    position: relative;
    width: 120px;
    height: 60px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .connector-label {
    position: absolute;
    top: -4px;
    font-size: 0.7rem;
    color: var(--sl-color-text-accent);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    white-space: nowrap;
  }

  .connector-line {
    width: 100%;
    height: 2px;
    background: var(--sl-color-gray-5);
  }

  .packet {
    position: absolute;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    opacity: 0;
  }

  /* Request packets flow left → right */
  .connector-left .packet-request {
    background: #64ffda;
    animation: packet-right var(--cycle-duration, 6s) ease-in-out infinite;
    animation-delay: var(--phase-delay, 0s);
  }

  .connector-right .packet-request {
    background: #ffd700;
    animation: packet-right var(--cycle-duration, 6s) ease-in-out infinite;
    animation-delay: calc(var(--phase-delay, 0s) + 0.3s);
  }

  /* Response packets flow right → left */
  .connector-left .packet-response {
    background: #64ffda;
    animation: packet-left var(--cycle-duration, 6s) ease-in-out infinite;
    animation-delay: calc(var(--phase-delay, 0s) + 2.5s);
  }

  .connector-right .packet-response {
    background: #ffd700;
    animation: packet-left var(--cycle-duration, 6s) ease-in-out infinite;
    animation-delay: calc(var(--phase-delay, 0s) + 2.2s);
  }

  @keyframes packet-right {
    0%, 15% { left: 0; opacity: 0; }
    20% { opacity: 1; }
    35% { left: calc(100% - 12px); opacity: 1; }
    40%, 100% { left: calc(100% - 12px); opacity: 0; }
  }

  @keyframes packet-left {
    0%, 15% { left: calc(100% - 12px); opacity: 0; }
    20% { opacity: 1; }
    35% { left: 0; opacity: 1; }
    40%, 100% { left: 0; opacity: 0; }
  }

  .hero-text {
    max-width: 600px;
    margin: 1rem auto 0;
    color: var(--sl-color-text-accent);
    font-size: 0.95rem;
    line-height: 1.6;
  }

  @media (max-width: 640px) {
    .architecture {
      flex-direction: column;
      gap: 0;
    }
    .connector {
      width: 60px;
      height: 80px;
      transform: rotate(90deg);
    }
    .node { min-width: 160px; }
  }
</style>
```

- [ ] **Step 2: Verify Astro syntax**

Run: `cd /Users/msilverblatt/hotmcp/docs && node -e "console.log('Astro file created')"`
(Actual build verification happens after all components are created.)

- [ ] **Step 3: Commit**

```bash
git add docs/src/components/demo/ArchitectureHero.astro
git commit -m "docs: add ArchitectureHero animated component"
```

---

### Task 12: TerminalReplay Component

**Files:**
- Create: `docs/src/components/demo/TerminalReplay.astro`

- [ ] **Step 1: Create the TerminalReplay component**

Dark terminal mockup with CSS typing animation. Includes Python/TypeScript tab toggle via a small client-side script.

```astro
---
// docs/src/components/demo/TerminalReplay.astro
// Simulated terminal session showing pmcp in action
---

<div class="terminal-container">
  <div class="terminal-tabs">
    <button class="tab active" data-lang="python">Python</button>
    <button class="tab" data-lang="typescript">TypeScript</button>
  </div>
  <div class="terminal">
    <div class="terminal-header">
      <span class="dot red"></span>
      <span class="dot yellow"></span>
      <span class="dot green"></span>
    </div>
    <div class="terminal-body">
      <div class="line line-1" data-python="$ pmcp dev tools.py" data-typescript="$ pmcp dev tools.ts">
        <span class="prompt">$</span> <span class="cmd">pmcp dev <span class="filename" data-python="tools.py" data-typescript="tools.ts">tools.py</span></span>
      </div>
      <div class="line line-2">
        <span class="success">✓</span> connected · 3 tools registered
      </div>
      <div class="line line-3">
        <span class="arrow-in">→</span> call <span class="tool-name">add</span> {"a": 2, "b": 3}
      </div>
      <div class="line line-4">
        <span class="arrow-out">←</span> result: <span class="result-value">5</span>
      </div>
      <div class="line line-5">
        <span class="arrow-in">→</span> call <span class="tool-name">search_files</span> {"directory": ".", "pattern": "*.py"}
      </div>
      <div class="line line-6">
        <span class="arrow-out">←</span> result: <span class="result-value">found 12 files</span>
      </div>
    </div>
  </div>
</div>

<script>
  const tabs = document.querySelectorAll('.terminal-tabs .tab');
  const filenames = document.querySelectorAll('.filename');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const lang = tab.getAttribute('data-lang') || 'python';
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      filenames.forEach(fn => {
        fn.textContent = fn.getAttribute(`data-${lang}`) || '';
      });
    });
  });
</script>

<style>
  .terminal-container {
    flex: 1;
    min-width: 300px;
  }

  .terminal-tabs {
    display: flex;
    gap: 0;
    margin-bottom: 0;
  }

  .tab {
    padding: 0.4rem 1rem;
    background: #1a1a2e;
    border: 1px solid #2d2d44;
    border-bottom: none;
    color: #888;
    cursor: pointer;
    font-size: 0.8rem;
    font-family: inherit;
    border-radius: 6px 6px 0 0;
  }

  .tab.active {
    background: #0d0d0d;
    color: #64ffda;
    border-color: #64ffda;
  }

  .terminal {
    background: #0d0d0d;
    border-radius: 0 8px 8px 8px;
    overflow: hidden;
    border: 1px solid #2d2d44;
  }

  .terminal-header {
    display: flex;
    gap: 6px;
    padding: 10px 14px;
    background: #1a1a2e;
  }

  .dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }

  .dot.red { background: #ff5f57; }
  .dot.yellow { background: #ffbd2e; }
  .dot.green { background: #28c841; }

  .terminal-body {
    padding: 1rem 1.2rem;
    font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 0.85rem;
    line-height: 1.8;
    color: #e0e0e0;
  }

  .line {
    opacity: 0;
    animation: fade-in 0.3s ease forwards;
  }

  .line-1 { animation-delay: calc(var(--phase-delay, 0s) + 0.5s); }
  .line-2 { animation-delay: calc(var(--phase-delay, 0s) + 1.2s); }
  .line-3 { animation-delay: calc(var(--phase-delay, 0s) + 2.0s); }
  .line-4 { animation-delay: calc(var(--phase-delay, 0s) + 2.8s); }
  .line-5 { animation-delay: calc(var(--phase-delay, 0s) + 3.6s); }
  .line-6 { animation-delay: calc(var(--phase-delay, 0s) + 4.4s); }

  @keyframes fade-in {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }

  .prompt { color: #888; }
  .cmd { color: #fff; }
  .success { color: #64ffda; }
  .arrow-in { color: #ffd700; }
  .arrow-out { color: #64ffda; }
  .tool-name { color: #c792ea; }
  .result-value { color: #c3e88d; }
  .filename { color: #89ddff; }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add docs/src/components/demo/TerminalReplay.astro
git commit -m "docs: add TerminalReplay animated component with language toggle"
```

---

### Task 13: ProtocolView Component

**Files:**
- Create: `docs/src/components/demo/ProtocolView.astro`

- [ ] **Step 1: Create the ProtocolView component**

Side-by-side view of JSON-RPC and protobuf formats with sequential line highlighting.

```astro
---
// docs/src/components/demo/ProtocolView.astro
// Side-by-side JSON-RPC ↔ protobuf protocol view
---

<div class="protocol-container">
  <div class="protocol-columns">
    <div class="protocol-col">
      <div class="col-header">
        <span class="col-dot" style="background: #64ffda"></span>
        MCP (JSON-RPC)
      </div>
      <div class="code-block">
        <pre class="protocol-code"><span class="hl hl-1">{"{"}</span>
<span class="hl hl-2">  "jsonrpc": "2.0",</span>
<span class="hl hl-3">  "id": 1,</span>
<span class="hl hl-4">  "method": "tools/call",</span>
<span class="hl hl-5">  "params": {"{"}</span>
<span class="hl hl-6">    "name": "add",</span>
<span class="hl hl-7">    "arguments": {"{"}</span>
<span class="hl hl-8">      "a": 2, "b": 3</span>
<span class="hl hl-7">    {"}"}</span>
<span class="hl hl-5">  {"}"}</span>
<span class="hl hl-1">{"}"}</span></pre>
      </div>
    </div>

    <div class="protocol-arrow">
      <span>↔</span>
    </div>

    <div class="protocol-col">
      <div class="col-header">
        <span class="col-dot" style="background: #ffd700"></span>
        protobuf (wire)
      </div>
      <div class="code-block">
        <pre class="protocol-code"><span class="hl hl-1">Envelope {"{"}</span>
<span class="hl hl-3">  request_id: 1</span>
<span class="hl hl-4">  call_tool: CallToolRequest {"{"}</span>
<span class="hl hl-6">    name: "add"</span>
<span class="hl hl-7">    arguments_json:</span>
<span class="hl hl-8">      '{"a": 2, "b": 3}'</span>
<span class="hl hl-4">  {"}"}</span>
<span class="hl hl-1">{"}"}</span>
<span class="hl proto-note">// 4-byte length prefix</span>
<span class="hl proto-note">// + serialized bytes</span></pre>
      </div>
    </div>
  </div>
</div>

<style>
  .protocol-container {
    flex: 1;
    min-width: 300px;
  }

  .protocol-columns {
    display: flex;
    gap: 0.5rem;
    align-items: stretch;
  }

  .protocol-col {
    flex: 1;
    background: #0d0d0d;
    border-radius: 8px;
    border: 1px solid #2d2d44;
    overflow: hidden;
  }

  .col-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0.6rem 1rem;
    background: #1a1a2e;
    font-size: 0.8rem;
    color: #e0e0e0;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .col-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .code-block {
    padding: 0.8rem 1rem;
  }

  .protocol-code {
    font-family: 'SF Mono', 'Fira Code', 'Consolas', monospace;
    font-size: 0.78rem;
    line-height: 1.7;
    color: #888;
    margin: 0;
    white-space: pre;
  }

  .protocol-arrow {
    display: flex;
    align-items: center;
    font-size: 1.5rem;
    color: var(--sl-color-text-accent);
    padding: 0 0.25rem;
  }

  .hl {
    transition: color 0.3s ease;
  }

  .hl-1 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 0.5s); }
  .hl-2 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 1.0s); }
  .hl-3 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 1.5s); }
  .hl-4 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 2.0s); }
  .hl-5 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 2.5s); }
  .hl-6 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 3.0s); }
  .hl-7 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 3.5s); }
  .hl-8 { animation: highlight var(--cycle-duration, 6s) ease infinite; animation-delay: calc(var(--phase-delay, 0s) + 4.0s); }

  @keyframes highlight {
    0%, 10% { color: #888; }
    15% { color: #e0e0e0; }
    30%, 100% { color: #888; }
  }

  .proto-note { color: #555; font-style: italic; }

  @media (max-width: 640px) {
    .protocol-columns { flex-direction: column; }
    .protocol-arrow { transform: rotate(90deg); justify-content: center; padding: 0.5rem; }
  }
</style>
```

- [ ] **Step 2: Commit**

```bash
git add docs/src/components/demo/ProtocolView.astro
git commit -m "docs: add ProtocolView side-by-side component"
```

---

## Chunk 4: Demo Page Assembly & Integration

### Task 14: Demo MDX Page

**Files:**
- Create: `docs/src/content/docs/demo.mdx`

- [ ] **Step 1: Create the demo page**

```mdx
---
title: Demo
description: See pmcp in action — animated architecture, terminal replay, and protocol view.
template: splash
hero:
  tagline: See how pmcp works — from high-level architecture down to the wire protocol.
---

import ArchitectureHero from '../../components/demo/ArchitectureHero.astro';
import TerminalReplay from '../../components/demo/TerminalReplay.astro';
import ProtocolView from '../../components/demo/ProtocolView.astro';

<div style="--cycle-duration: 8s; --phase-delay: 0s;">

<ArchitectureHero />

## In Action

Watch a tool call flow through the system — from the terminal command to the wire protocol.

<div class="demo-panels">
  <TerminalReplay />
  <ProtocolView />
</div>

</div>

<style>{`
  .demo-panels {
    display: flex;
    gap: 1.5rem;
    margin: 1.5rem 0;
    align-items: flex-start;
  }

  @media (max-width: 768px) {
    .demo-panels {
      flex-direction: column;
    }
  }
`}</style>

---

## Try It Yourself

```sh
brew install protomcp/tap/protomcp
pmcp dev examples/python/basic.py
```

Check out the [Quick Start](/getting-started/quick-start/) guide or browse the [examples](https://github.com/msilverblatt/protomcp/tree/master/examples).
```

- [ ] **Step 2: Commit**

```bash
git add docs/src/content/docs/demo.mdx
git commit -m "docs: add interactive demo page with animated components"
```

---

### Task 15: Sidebar Integration

**Files:**
- Modify: `docs/astro.config.mjs:12` (sidebar array)

- [ ] **Step 1: Add Demo link to sidebar**

In `docs/astro.config.mjs`, add the Demo link as a top-level item before the first group. Change the `sidebar` array from:

```javascript
      sidebar: [
        {
          label: 'Getting Started',
```

to:

```javascript
      sidebar: [
        { label: 'Demo', slug: 'demo' },
        {
          label: 'Getting Started',
```

- [ ] **Step 2: Commit**

```bash
git add docs/astro.config.mjs
git commit -m "docs: add Demo to sidebar navigation"
```

---

### Task 16: Build Verification

- [ ] **Step 1: Build the docs site**

Run: `cd /Users/msilverblatt/hotmcp/docs && npm run build`
Expected: Build succeeds with no errors. The demo page is generated.

- [ ] **Step 2: Verify demo page exists in output**

Run: `ls /Users/msilverblatt/hotmcp/docs/dist/demo/`
Expected: `index.html` exists

---

### Task 17: Makefile and .gitignore Updates

**Files:**
- Modify: `Makefile` (add demo target)
- Modify: `.gitignore` (add .superpowers/)

- [ ] **Step 1: Add demo target to Makefile**

Add after the existing `clean` target:

```makefile
demo:
	./examples/run-demo.sh
```

- [ ] **Step 2: Add .superpowers/ to .gitignore**

Append `.superpowers/` to `.gitignore` if not already present.

- [ ] **Step 3: Commit**

```bash
git add Makefile .gitignore
git commit -m "chore: add demo target and ignore .superpowers/"
```
