# Full MCP Spec Coverage — Design Spec

**Goal:** Implement 100% of the MCP 2025-03-26 specification in protomcp — every method, notification, content type, transport, and auth mechanism — with tests, docs, and examples for every feature across all 4 SDKs.

**Architecture:** 12 vertical slices, each delivering a complete feature (proto + Go handler + 4 SDKs + tests + docs + examples) before moving to the next. Server-side features first, then transport, then auth.

**Tech Stack:** Go (handler/transport), Python/TypeScript/Go/Rust (SDKs), protobuf (internal protocol), fsnotify (file watching), JSON-RPC 2.0 (MCP wire format), OAuth 2.1 / JWT (auth)

---

## Current State

protomcp implements 5 of ~20 MCP spec features:

| Feature | Status |
|---------|--------|
| `initialize` / `notifications/initialized` | Implemented (protocol version `2024-11-05`) |
| `tools/list` / `tools/call` / `notifications/tools/list_changed` | Implemented |
| `notifications/cancelled` | Implemented |
| `tasks/get` / `tasks/cancel` | Partial |
| Progress notifications, SDK logging | Implemented |
| Resources, Prompts, Completions, Sampling, Roots, Logging/setLevel, Ping, Pagination, Rich content, Streamable HTTP, OAuth | Not implemented |

## Design Decisions

1. **Vertical slices** — each feature is fully implemented (proto → handler → 4 SDKs → tests → docs → examples) before moving to the next
2. **Resources: callback + file watcher** — SDK users register resources with optional onChange callbacks; `file://` URIs get automatic fsnotify watching
3. **Sampling: context API + server API** — Primary API is `ctx.sample()` inside tool handlers; escape hatch is `server.sample(session_id, ...)` for server-initiated sampling from background events
4. **Completions: per-argument inline** — completion providers are attached directly to argument definitions (static list or function), no separate registration
5. **Streamable HTTP: replace existing** — single spec-compliant transport replaces both current HTTP and SSE transports
6. **OAuth: resource server only** — protomcp validates tokens and serves metadata; actual authorization server is external
7. **Docs ship with each feature** — not batched at the end
8. **Examples: focused + kitchen sink** — individual per-feature examples plus one comprehensive example per SDK

## Slice Order & Dependencies

```
1. Ping                    (standalone)
2. Rich Content Types      (standalone)
3. Pagination              (standalone)
4. Logging/setLevel        (standalone)
5. Resources               (needs: pagination, rich content)
6. Prompts                 (needs: pagination, rich content)
7. Completions             (needs: resources, prompts)
8. Sampling                (needs: bidirectional request plumbing)
9. Roots                   (benefits from session plumbing in sampling)
10. Streamable HTTP        (replaces HTTP + SSE transports)
11. OAuth 2.1              (needs: Streamable HTTP)
12. Protocol Version Bump  (needs: everything else)
```

---

## Slice 1: Ping

**MCP method:** `ping` → `{}`

**Handler (`internal/mcp/handler.go`):** Add `ping` method that returns empty result `{}`. Both directions — server responds to client pings.

**SDKs:** No changes needed. Ping is handled entirely at the MCP JSON-RPC layer.

**Tests:**
- Unit: send `ping` JSON-RPC request, verify empty response
- Integration: ping via stdio and HTTP transports

**Docs:** Brief section in transport/connection docs.

---

## Slice 2: Rich Content Types

**Problem:** `ContentItem` in `internal/mcp/types.go` only has `Type` and `Text` fields. MCP spec defines text, image, audio, and embedded resource content types for tool results and prompt messages.

### Types changes (`internal/mcp/types.go`)

```go
type ContentItem struct {
    Type     string           `json:"type"`
    Text     string           `json:"text,omitempty"`
    Data     string           `json:"data,omitempty"`      // base64 for image/audio
    MimeType string           `json:"mimeType,omitempty"`  // for image/audio/resource
    Resource *ResourceContent `json:"resource,omitempty"`  // for embedded resources
}

type ResourceContent struct {
    URI      string `json:"uri"`
    MimeType string `json:"mimeType,omitempty"`
    Text     string `json:"text,omitempty"`
    Blob     string `json:"blob,omitempty"`  // base64
}
```

### Proto changes

Update internal protocol to carry structured content items instead of flat `result_json` string. Add content type variants to support image/audio/resource in tool responses.

### SDK APIs (all 4)

Builder methods on tool results:
- `ToolResult::text("...")`
- `ToolResult::image(data, mime_type)`
- `ToolResult::audio(data, mime_type)`
- `ToolResult::resource(uri, mime_type, text_or_blob)`
- `ToolResult::mixed([...])` for multi-content responses

### Tests

Round-trip each content type through tool call → response. Verify JSON structure matches MCP spec.

### Docs

Content types guide with examples in each SDK.

---

## Slice 3: Pagination

**MCP spec:** Cursor-based pagination for `tools/list`, `resources/list`, `prompts/list`, `resources/templates/list`. Opaque cursor, server-determined page size, `nextCursor` in response.

### Handler

- Server-side cursor: base64-encoded offset
- Page size configurable (default 100)
- When result set exceeds page size, include `nextCursor` in response
- No proto changes — pagination is at the MCP JSON-RPC layer. SDK process sends full list; Go handler handles windowing.

### Tests

- Register >100 tools, verify cursor pagination
- Verify missing cursor returns first page
- Verify invalid cursor returns error `-32602`

### Docs

Pagination guide.

---

## Slice 4: Logging (`logging/setLevel`)

**Current state:** SDKs have 8-level logging via `notifications/message`. No MCP method for clients to control the level.

### Handler

- Add `logging/setLevel` method
- Store current level per session
- Filter `notifications/message` to only forward messages at or above the set level

### Tests

- Set level to `error`, verify `info` messages are filtered
- Set level to `debug`, verify all messages pass through
- Multiple sessions with different levels

### Docs

Logging guide covering SDK logging API + client-side level control.

---

## Slice 5: Resources

### Proto changes (`proto/protomcp.proto`)

New messages added to `Envelope.msg` oneof:

```protobuf
message ResourceDefinition {
  string uri = 1;
  string name = 2;
  string description = 3;
  string mime_type = 4;
  int64 size = 5;
}

message ResourceTemplateDefinition {
  string uri_template = 1;
  string name = 2;
  string description = 3;
  string mime_type = 4;
}

message ListResourcesRequest {}
message ResourceListResponse {
  repeated ResourceDefinition resources = 1;
}

message ListResourceTemplatesRequest {}
message ResourceTemplateListResponse {
  repeated ResourceTemplateDefinition templates = 1;
}

message ReadResourceRequest {
  string uri = 1;
}

message ReadResourceResponse {
  repeated ResourceContent contents = 1;
}

message ResourceContent {
  string uri = 1;
  string mime_type = 2;
  string text = 3;
  bytes blob = 4;
}
```

### Handler (`internal/mcp/handler.go`)

New MCP methods:
- `resources/list` — sends `ListResourcesRequest` to SDK process, returns `ResourceListResponse`. Paginated.
- `resources/read` — sends `ReadResourceRequest` to SDK process, returns contents.
- `resources/templates/list` — sends `ListResourceTemplatesRequest` to SDK process, returns `ResourceTemplateListResponse`. Paginated.
- `resources/subscribe` — registers URI in per-session subscription set.
- `resources/unsubscribe` — removes URI from subscription set.

Notifications (server → client):
- `notifications/resources/list_changed` — sent when resource list changes
- `notifications/resources/updated` — sent when subscribed resource changes

### File watcher (`internal/resource/watcher.go`)

- Uses `fsnotify` to watch `file://` resource URIs
- On file change, checks subscription set, sends `notifications/resources/updated` to subscribed sessions
- Automatic for `file://` URIs — SDK users don't need to do anything
- Max watch limit: 1024 URIs (configurable). Log warning when approaching limit. Return error on subscribe if limit exceeded.

### SDK APIs (all 4)

**Python example:**
```python
@resource(
    uri="file:///config/settings.json",
    name="Settings",
    description="Application settings",
    mime_type="application/json"
)
def read_settings() -> str:
    return open("/config/settings.json").read()

@resource_template(
    uri_template="db://tables/{table_name}/schema",
    name="Table Schema",
    arguments=[
        Arg("table_name", completions=lambda v: search_tables(v))
    ]
)
def read_table_schema(table_name: str) -> str:
    return get_schema(table_name)
```

Change notification:
```python
protomcp.notify_resource_changed("file:///config/settings.json")
```

SDK runner loop handles `ListResourcesRequest` and `ReadResourceRequest`.

### Initialize capability

```json
"resources": { "subscribe": true, "listChanged": true }
```

### Tests

- Unit: register resource, list, read, verify content
- Unit: register template, read with URI matching
- Unit: subscribe, trigger change, verify notification
- Integration: file watcher (create temp file, subscribe, modify, verify notification)
- Integration: binary resource (image blob round-trip)

### Docs & examples

- `docs/resources.md` — static resources, templates, subscriptions, file watching
- `examples/resources/` per SDK — file server example

---

## Slice 6: Prompts

### Proto changes

```protobuf
message PromptArgument {
  string name = 1;
  string description = 2;
  bool required = 3;
}

message PromptDefinition {
  string name = 1;
  string description = 2;
  repeated PromptArgument arguments = 3;
}

message ListPromptsRequest {}
message PromptListResponse {
  repeated PromptDefinition prompts = 1;
}

message GetPromptRequest {
  string name = 1;
  string arguments_json = 2;
}

message PromptMessage {
  string role = 1;
  string content_json = 2;  // JSON array of content items
}

message GetPromptResponse {
  string description = 1;
  repeated PromptMessage messages = 2;
}
```

### Handler

- `prompts/list` — paginated, proxies to SDK process
- `prompts/get` — sends name + arguments, returns messages
- `notifications/prompts/list_changed` — sent on prompt list changes

### SDK APIs

**Python example:**
```python
@prompt(
    name="code_review",
    description="Review code quality",
    arguments=[
        Arg("language", required=True,
            completions=["python", "typescript", "go", "rust"]),
        Arg("style", required=False,
            completions=["security", "performance", "readability"]),
    ]
)
def code_review(language: str, style: str = "readability") -> list[Message]:
    return [
        Message(role="user", content=Text(f"Review this {language} code for {style}...")),
    ]
```

Message content supports all rich content types (text, image, audio, embedded resource).

### Initialize capability

```json
"prompts": { "listChanged": true }
```

### Tests

- Unit: register prompt, list, get with arguments
- Unit: prompt with multi-content messages
- Unit: list changed notification

### Docs & examples

- `docs/prompts.md`
- `examples/prompts/` per SDK — code review prompt

---

## Slice 7: Completions

### Proto changes

```protobuf
message CompletionRequest {
  string ref_type = 1;       // "ref/prompt" or "ref/resource"
  string ref_name = 2;       // prompt name or resource URI
  string argument_name = 3;
  string argument_value = 4; // current partial value
}

message CompletionResponse {
  repeated string values = 1;
  int32 total = 2;
  bool has_more = 3;
}
```

### Handler

- `completion/complete` — parses ref type, sends `CompletionRequest` to SDK process, returns suggestions

### SDK implementation

No new registration API — completions are already defined inline on prompt/resource arguments. Runner:
1. Receives `CompletionRequest`
2. Looks up prompt or resource template by ref name
3. Finds argument by name
4. Static list → prefix-filter against `argument_value`
5. Function → call with `argument_value`
6. Cap at 100 results, set `has_more`

### Initialize capability

```json
"completions": {}
```

### Tests

- Static list completions with prefix filtering
- Function-based completions
- Both ref/prompt and ref/resource
- >100 results returns has_more=true

### Docs

- `docs/completions.md`
- Completions shown in resource/prompt examples

---

## Slice 8: Sampling

Introduces bidirectional request/response: SDK process → Go handler → MCP client → Go handler → SDK process.

### Proto changes

```protobuf
message SamplingRequest {
  string request_id = 1;
  string messages_json = 2;
  string model_preferences_json = 3;
  string system_prompt = 4;
  int32 max_tokens = 5;
  string session_id = 6;  // empty = current session
}

message SamplingResponse {
  string request_id = 1;
  string role = 2;
  string content_json = 3;
  string model = 4;
  string stop_reason = 5;
  string error = 6;
}
```

### Handler

- Receives `SamplingRequest` from SDK process
- Constructs `sampling/createMessage` JSON-RPC request
- Sends to MCP client (new capability: handler can send requests, not just responses)
- Waits for response via correlation map (request ID → response channel)
- Sends `SamplingResponse` back to SDK process

Must verify client declared `sampling` capability during initialize.

### Timeout and cancellation

- Default timeout: 30 seconds (configurable via `PROTOMCP_SAMPLING_TIMEOUT` or per-call)
- If client doesn't respond within timeout, Go handler removes the correlation entry and sends `SamplingResponse` with error back to SDK process
- If the tool call is cancelled (via `notifications/cancelled`), Go handler sends `notifications/cancelled` for the pending sampling request to the client and cleans up the correlation channel
- SDK `ctx.sample()` raises/returns a timeout or cancellation error
- Per-call timeout override: `ctx.sample(messages=[...], timeout=60)`

### SDK APIs

**Context API (inside tool handler):**
```python
@tool(name="smart_search")
def smart_search(ctx: ToolContext, query: str) -> str:
    result = ctx.sample(
        messages=[{"role": "user", "content": {"type": "text", "text": f"Refine: {query}"}}],
        max_tokens=200,
    )
    return search(result.text)
```

**Server API (outside tool handler):**
```python
result = protomcp.sample(
    session_id="...",
    messages=[...],
    max_tokens=100,
)
```

For stdio (single client), session_id can be omitted.

For HTTP (multiple clients), SDK users can list active sessions via `protomcp.list_sessions()` to discover valid session IDs.

### Tests

- Mock client that responds to sampling requests
- Tool handler calls ctx.sample(), verify round-trip
- server.sample() outside tool context
- Error: client doesn't support sampling capability

### Docs & examples

- `docs/sampling.md`
- `examples/sampling/` — smart search tool

---

## Slice 9: Roots

Client provides filesystem roots to server.

### Handler

- Store client `roots` capability during initialize
- Send `roots/list` JSON-RPC request to client (reuses bidirectional plumbing from slice 8)
- Handle `notifications/roots/list_changed` from client — update stored roots, notify SDK process

### Proto changes

```protobuf
message RootsListResponse {
  string roots_json = 1;  // JSON array of {uri, name}
}

message RootsChangedNotification {}
```

### SDK APIs

```python
@tool(name="find_files")
def find_files(ctx: ToolContext, pattern: str) -> str:
    roots = ctx.get_roots()
    # ...

# Server-level
roots = protomcp.get_roots(session_id="...")

@protomcp.on_roots_changed
def handle_roots_changed(roots):
    rebuild_index(roots)
```

### Tests

- Mock client responds to `roots/list`
- Client sends `notifications/roots/list_changed`, verify callback
- Error: client doesn't support roots

### Docs & examples

- `docs/roots.md`
- `examples/roots/` — file search scoped to roots

---

## Slice 10: Streamable HTTP Transport

Replaces `internal/transport/http.go` and `internal/transport/sse.go` with a single spec-compliant transport.

### New file: `internal/transport/streamable.go`

**POST endpoint:**
- Receives JSON-RPC requests, notifications, responses
- Notifications/responses only → 202 Accepted
- Requests → respond with `application/json` or `text/event-stream`
- SSE stream can include server notifications/requests before the response

**GET endpoint:**
- Opens SSE stream for server-initiated messages
- Supports `Last-Event-ID` for resumability

**DELETE endpoint:**
- Terminates session

### Session management

- Generate `Mcp-Session-Id` on initialize (cryptographically secure UUID)
- Per-session state: subscriptions, log level, roots, active streams
- Validate on all requests (400 if missing, 404 if expired)
- Cleanup on DELETE or idle timeout (default 30 minutes, configurable)

### Resumability

- Monotonically increasing SSE event IDs per stream
- Bounded buffer of recent events per stream (default 1000 events, configurable)
- Replay on reconnect with `Last-Event-ID`

### Deleted files

- `internal/transport/http.go` — replaced by `streamable.go`
- `internal/transport/sse.go` — replaced by `streamable.go`
- CLI: `--transport http` becomes Streamable HTTP; `--transport sse` removed

### Breaking change / migration

This is a breaking change for anyone using `--transport http` or `--transport sse`. The old custom NDJSON/SSE streaming is replaced with spec-compliant Streamable HTTP. No deprecation period — the old transports were custom and not MCP-spec-compliant, so no real MCP client depends on them. The stdio transport is unchanged.

### Preserved transports

- `internal/transport/ws.go` (WebSocket) — kept as-is. Not part of MCP spec but useful as a custom transport.
- `internal/transport/grpc.go` (gRPC) — kept as-is. Same rationale.
- `internal/transport/stdio.go` — kept as-is. Required by MCP spec.

### Tests

- POST → JSON response
- POST → SSE stream response
- GET → SSE stream with server messages
- Session lifecycle (create, use, delete, 404 after)
- Resumability (disconnect, reconnect, replay)
- Concurrent requests on same session

### Docs

- `docs/transports.md` — stdio + Streamable HTTP

---

## Slice 11: OAuth 2.1

Resource server / token validation only. Authorization server is external.

### New file: `internal/middleware/oauth.go`

- **JWT validation** with JWKS endpoint
- **Opaque token introspection** endpoint
- **401 response** when auth required but no valid bearer token
- **Metadata endpoint** — serve `/.well-known/oauth-authorization-server`

### Configuration

```yaml
auth:
  required: true
  jwks_url: "https://auth.example.com/.well-known/jwks.json"
  issuer: "https://auth.example.com"
  audience: "my-mcp-server"
```

Or opaque tokens:
```yaml
auth:
  required: true
  introspection_url: "https://auth.example.com/introspect"
  client_id: "my-server"
  client_secret_env: "OAUTH_CLIENT_SECRET"
```

Existing simple token/API key auth stays as lightweight option.

### JWKS caching

- Cache JWKS keys in memory with configurable TTL (default 1 hour)
- On cache miss or expired entry, fetch from JWKS URL
- If JWKS endpoint is unreachable, use cached keys for up to 24 hours past their TTL (grace period)
- On unknown `kid` in JWT header, force a JWKS refresh (handles key rotation)

### Scope checking

Scope-based access control (e.g. restricting which tools a token can call) is explicitly out of scope for this spec. protomcp validates token authenticity (signature, expiry, issuer, audience). Application-level authorization is left to middleware or SDK user code.

### Tests

- Valid JWT → 200
- Expired JWT → 401
- Missing token → 401
- Invalid audience → 401
- Metadata endpoint structure

### Docs

- `docs/authentication.md` — simple auth vs OAuth

---

## Slice 12: Protocol Version Bump + Kitchen Sink

### Protocol version

Change `ProtocolVersion` from `"2024-11-05"` to `"2025-03-26"`.

### Version negotiation

Per the MCP spec: if a client sends `initialize` with `protocolVersion: "2024-11-05"`, the server responds with `"2025-03-26"` (the latest it supports). The client can then decide whether to accept or disconnect. protomcp supports only `2025-03-26` — it does not maintain backwards compatibility with `2024-11-05` feature gaps.

Full capability advertisement:
```json
{
  "protocolVersion": "2025-03-26",
  "capabilities": {
    "tools": { "listChanged": true },
    "resources": { "subscribe": true, "listChanged": true },
    "prompts": { "listChanged": true },
    "logging": {},
    "completions": {}
  }
}
```

### Kitchen sink example (all 4 SDKs)

One comprehensive example per SDK demonstrating every feature:
- Tools (all content types)
- Resources (static + templates + file watcher)
- Prompts (with arguments)
- Completions (static + dynamic)
- Sampling (tool that calls LLM)
- Logging
- Middleware

### Final docs

- README update with feature matrix showing 100% MCP spec coverage
- `docs/mcp-compliance.md` — mapping of every spec method to protomcp implementation

### Tests

- Full integration test exercising every MCP method in sequence
- Protocol version negotiation

---

## MCP Spec Coverage Matrix (Post-Implementation)

| Spec Feature | Method | Slice |
|-------------|--------|-------|
| Initialize | `initialize` | Existing + Slice 12 (version bump) |
| Initialized | `notifications/initialized` | Existing |
| Ping | `ping` | Slice 1 |
| Cancellation | `notifications/cancelled` | Existing |
| Progress | `notifications/progress` | Existing |
| Tool List | `tools/list` | Existing + Slice 3 (pagination) |
| Tool Call | `tools/call` | Existing + Slice 2 (rich content) |
| Tool List Changed | `notifications/tools/list_changed` | Existing |
| Resource List | `resources/list` | Slice 5 |
| Resource Read | `resources/read` | Slice 5 |
| Resource Templates List | `resources/templates/list` | Slice 5 |
| Resource Subscribe | `resources/subscribe` | Slice 5 |
| Resource Unsubscribe | `resources/unsubscribe` | Slice 5 |
| Resource List Changed | `notifications/resources/list_changed` | Slice 5 |
| Resource Updated | `notifications/resources/updated` | Slice 5 |
| Prompt List | `prompts/list` | Slice 6 |
| Prompt Get | `prompts/get` | Slice 6 |
| Prompt List Changed | `notifications/prompts/list_changed` | Slice 6 |
| Completion | `completion/complete` | Slice 7 |
| Sampling | `sampling/createMessage` | Slice 8 |
| Roots List | `roots/list` | Slice 9 |
| Roots List Changed | `notifications/roots/list_changed` | Slice 9 |
| Logging Set Level | `logging/setLevel` | Slice 4 |
| Log Messages | `notifications/message` | Existing + Slice 4 (filtering) |
| Streamable HTTP | POST/GET/DELETE | Slice 10 |
| OAuth 2.1 | Bearer token / JWKS / Introspection | Slice 11 |
| Content: Text | `type: "text"` | Existing |
| Content: Image | `type: "image"` | Slice 2 |
| Content: Audio | `type: "audio"` | Slice 2 |
| Content: Resource | `type: "resource"` | Slice 2 |
| Pagination | Cursor-based | Slice 3 |
