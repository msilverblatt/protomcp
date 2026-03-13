# README, Working Demos & Interactive Demo Page — Design Spec

## Overview

Three deliverables to make protomcp (pmcp) immediately understandable and adoptable:

1. **README.md** — project introduction with architecture diagram, dual-language quick start, feature highlights, and comparison table
2. **Working code examples** — runnable demos at three complexity tiers in both Python and TypeScript
3. **Interactive demo page** — Astro component-driven page in the Starlight docs site with animated architecture hero, terminal replay, and protocol view

Primary audience: developers evaluating pmcp AND developers already adopting it.

---

## 1. README.md

**Location:** `/README.md` (repo root)

### Structure

1. **Project name + one-liner** — "Language-agnostic MCP runtime — write tools in any language, hot-reload without restarting your AI host."
2. **Badges** — build status, license, Go version, npm version, PyPI version
3. **Text-based architecture diagram** — ASCII art showing `MCP Host ←→ pmcp (Go) ←→ tool process`
4. **Quick Start** — install via Homebrew, then two code blocks side by side: Python (`@tool` decorator) and TypeScript (`tool()` with Zod). Each followed by `pmcp dev <file>`.
5. **Features** — bullet list: any language, hot reload, dynamic tool lists, 5 transports (stdio, SSE, HTTP, WebSocket, gRPC), structured output, async tasks, progress/cancellation, server logging, tool metadata/annotations
6. **Comparison table** — pmcp vs FastMCP (Python) vs official MCP SDKs (TypeScript/Python). Columns: feature, pmcp, FastMCP (Python), MCP SDKs. Rows: language support, hot reload, dynamic tools, transport options, structured output, async tasks.
7. **Examples** — brief description of `examples/` directory contents with links
8. **Documentation** — link to Starlight docs site
9. **License**

### Constraints

- No hallucinated URLs — only link to `https://github.com/msilverblatt/protomcp`
- CLI command is `pmcp`, never `protomcp`
- Both Python AND TypeScript examples in every code section
- Badge URLs must be real (GitHub Actions, shields.io pattern)

---

## 2. Working Code Examples

**Location:** `examples/` at repo root

### Directory Structure

```
examples/
├── python/
│   ├── basic.py
│   ├── real_world.py
│   ├── full_showcase.py
│   └── requirements.txt
├── typescript/
│   ├── basic.ts
│   ├── real-world.ts
│   ├── full-showcase.ts
│   ├── package.json
│   └── tsconfig.json
├── run-demo.sh
└── README.md
```

### Example Tiers

**Basic** (`basic.py`, `basic.ts`):
- Single `add` tool
- Minimal code — the absolute simplest starting point
- Header comment: what it does, how to run (`pmcp dev examples/python/basic.py`)

**Real-world** (`real_world.py`, `real-world.ts`):
- File search tool that searches a directory for files matching a pattern
- Demonstrates: `ToolContext` for progress reporting, `ServerLogger` for logging, `ctx.is_cancelled()` for cancellation support
- Real-ish logic, not toy math

**Full showcase** (`full_showcase.py`, `full-showcase.ts`):
- Multi-tool server with 3-4 tools demonstrating the full feature set:
  - Structured output with output schema
  - Async task support (long-running operation)
  - Dynamic tool lists (enable/disable tools at runtime)
  - Tool metadata/annotations (destructive, read-only, idempotent hints)
  - Progress reporting
  - Cancellation
  - Server logging at multiple levels
- Each tool demonstrates a different subset of features

### run-demo.sh

- Checks that `pmcp` is installed
- Uses stdio transport: starts `pmcp dev <file>` as a subprocess, pipes JSON-RPC messages to stdin, reads JSON-RPC responses from stdout
- Performs the full MCP handshake for each example: `initialize` request → `initialized` notification → `tools/list` → `tools/call` with example arguments
- JSON-RPC messages are constructed inline in the script (heredocs)
- Prints a human-readable summary of each interaction (tool name, args, result)
- Works with both Python and TypeScript examples
- Exits cleanly with summary of what ran

### examples/package.json

- Minimal `package.json` with `protomcp` and `zod` as dependencies for the TypeScript examples
- Includes a `tsconfig.json` with `moduleResolution: "node"`, `esModuleInterop: true`

### examples/python/requirements.txt

- Lists `protomcp` as the sole dependency for the Python examples

### examples/README.md

- Table listing each example file, what features it demonstrates, and the run command
- Links to main README and docs

---

## 3. Interactive Demo Page

**Location:** `docs/src/content/docs/demo.mdx` + `docs/src/components/demo/`

### Components

```
docs/src/components/demo/
├── ArchitectureHero.astro
├── TerminalReplay.astro
└── ProtocolView.astro
```

### ArchitectureHero.astro (Hero Section)

- Large animated diagram at the top of the page
- Three boxes arranged horizontally: "MCP Host" (teal border) → "pmcp" (gold border) → "Your Code" (coral border)
- Animated "message packet" (small colored div) flows left-to-right for a tool call, then right-to-left for the result
- Arrow labels alternate between "JSON-RPC" (MCP side) and "protobuf" (tool side)
- CSS `@keyframes` animation — no JavaScript required
- Loops with a pause between cycles
- Brief explanatory text below: "pmcp sits between your MCP host and your tool process. It translates JSON-RPC to protobuf, handles hot reload, and manages tool registration."

### TerminalReplay.astro (Left Half Below Hero)

- Dark terminal mockup (`background: #0d0d0d`, monospace font)
- Simulated typing animation showing:
  1. `$ pmcp dev tools.py`
  2. `✓ connected · 3 tools registered`
  3. `→ call add {"a": 2, "b": 3}`
  4. `← result: 5`
- CSS `@keyframes` with staged delays for each line
- Tab toggle at top: "Python" / "TypeScript" — switches the filename and code style. Uses a small inline `<script>` tag (Astro client-side script) to toggle visibility classes on click.
- Loops after completing

### ProtocolView.astro (Right Half Below Hero)

- Two columns side by side: "MCP (JSON-RPC)" and "protobuf (wire format)"
- Shows the same `tools/call` request in both formats
- Lines highlight sequentially (CSS animation) to show field-by-field mapping
- Uses shared CSS custom properties for timing (`--cycle-duration`, `--phase-delay`) defined in a parent `<style>` in `demo.mdx`, so both TerminalReplay and ProtocolView stay in sync and can be adjusted from one place
- JSON-RPC side shows the standard MCP request format
- Protobuf side shows the Envelope with CallToolRequest

### demo.mdx

- Imports all three components
- Brief intro text: "See how pmcp works — from the high-level architecture down to the wire protocol."
- `<ArchitectureHero />` — hero section
- Section heading: "In Action" with brief text
- Flex container with `<TerminalReplay />` (left) and `<ProtocolView />` (right)
- Bottom CTA: links to Quick Start guide and examples directory

### Sidebar Integration

- Add "Demo" as a top-level link item in the Starlight sidebar config in `astro.config.mjs`: `{ label: 'Demo', slug: 'demo' }` placed before the first group ("Getting Started")

### Responsive Layout

- ArchitectureHero boxes stack vertically on screens < 640px
- TerminalReplay and ProtocolView stack vertically on screens < 768px (instead of side-by-side)
- Use CSS media queries, no JS

---

## 4. Integration

### Cross-linking

- **README** → links to demo page ("See it in action"), examples directory, and docs site
- **Demo page** → links to quick start guide and examples
- **Examples README** → links to main README and docs

### Makefile

- Add `demo` target that runs `examples/run-demo.sh`

### .gitignore

- Ensure `.superpowers/` is in `.gitignore`

---

## Non-Goals

- No live/interactive REPL in the browser (all animations are pre-scripted CSS)
- No video recording or GIF generation
- No CI/CD for the examples (they're documentation, tested manually)
- No separate demo app or SPA — everything lives in the Starlight site
