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

- [pmcp](https://github.com/msilverblatt/protomcp) installed (`brew install msilverblatt/tap/protomcp`)
- Python 3.10+ (for Python examples)
- Node.js 18+ (for TypeScript examples)

## Links

- [Documentation](https://github.com/msilverblatt/protomcp)
- [Python Guide](https://github.com/msilverblatt/protomcp/tree/master/docs)
- [TypeScript Guide](https://github.com/msilverblatt/protomcp/tree/master/docs)
