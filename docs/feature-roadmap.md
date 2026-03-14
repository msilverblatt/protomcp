# protomcp Feature Roadmap

## MCP Spec Coverage

### Implemented
| Feature | Methods | Status |
|---------|---------|--------|
| Tools | `tools/list`, `tools/call` | Full support |
| Tasks | `tasks/get`, `tasks/cancel` | Partial (proto + handler, no SDK support) |
| Initialize | `initialize`, `notifications/initialized` | Full support |
| Cancellation | `notifications/cancelled` | Full support |

### Not Implemented
| Feature | Methods | Priority | Notes |
|---------|---------|----------|-------|
| Resources | `resources/list`, `resources/read`, `resources/subscribe`, `resources/templates/list` | High | Core MCP feature, needed for file/data serving |
| Prompts | `prompts/list`, `prompts/get` | High | Core MCP feature, needed for prompt templates |
| Ping | `ping` | Medium | Simple to add, useful for health checks |
| Sampling | `sampling/createMessage` | Low | Server-initiated LLM calls, niche use case |
| Roots | `roots/list` | Low | Client filesystem roots, rarely used |
| Completions | `completion/complete` | Low | Argument autocompletion, nice-to-have |
| Logging | `logging/setLevel` | Low | Runtime log level control |

## SDK Feature Matrix

| Feature | Python | TypeScript | Go | Rust |
|---------|--------|-----------|----|----|
| Tool Registration | Y | Y | Y | Y |
| Middleware | Y | Y | Y | Y |
| Logging (8 levels) | Y | Y | Y | Y |
| Progress Reporting | Y | Y | Y | Y |
| Cancellation | Flag | Flag | context.Context | Flag |
| send_raw / Sideband | Y | Y | Y | Y |
| zstd Compression | Y | Y | Y | Y |
| Schema Generation | Auto (hints) | Zod | Manual | Manual |
| Dynamic Tool Lists | Y | Y | N | N |
| Structured Output | Y | Y | N | N |
| Runtime Schema Validation | N | N | N | N |

## Priority Improvements

### Performance (Done / In Progress)
- [x] Raw sideband transfer (bypass protobuf for large payloads)
- [x] zstd compression on sideband (all 4 SDKs)
- [x] HTTP gzip Content-Encoding
- [x] Scanner buffers to 512MB for 250MB payloads

### SDK Parity Gaps
- [ ] Go/Rust: Dynamic tool list management (enable/disable at runtime)
- [ ] Go/Rust: Structured output support
- [ ] Python/TS/Rust: Context-based cancellation (currently flag-only)
- [ ] All SDKs: Runtime schema validation before handler invocation

### MCP Spec Features
- [ ] Resources support (proto messages + handler + all 4 SDKs)
- [ ] Prompts support (proto messages + handler + all 4 SDKs)
- [ ] Ping handler

### DX Improvements
- [ ] `pmcp init` command to scaffold new tool projects
- [ ] Documentation for compression/sideband usage
- [ ] Documentation for cancellation patterns
- [ ] Hot reload: watch imported modules, not just entry file
