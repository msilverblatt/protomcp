# Chunked Streaming for Large Payloads

## Goal

Eliminate protomcp's large-payload performance gap vs FastMCP and build a streaming content transfer system that no other MCP implementation offers — enabling agents to efficiently exchange large data (files, datasets, images) over MCP.

## Architecture

Three phases, each building on the previous:

1. **Phase C:** Zero-copy JSON passthrough — eliminate redundant parse/re-serialize in the handler
2. **Phase A:** Chunked internal transfer — stream large payloads over the unix socket in fixed-size chunks
3. **Phase B:** End-to-end streaming — stream large results all the way to the MCP host via a protomcp extension

Each phase is independently shippable and backward compatible.

## Current Bottleneck

The tool result data path performs 6-7 copies of the payload:

```
Python SDK:  json.dumps(result) → protobuf serialize → socket sendall
Go proxy:    socket read → protobuf deserialize → json.Unmarshal(result_json)
             → json.Marshal(ToolsCallResult) → json.Marshal(JSONRPCResponse)
```

Benchmark results at 500KB: protomcp 17ms vs FastMCP 9ms. The gap grows with payload size.

---

## Phase C: Zero-Copy JSON Passthrough

### Problem

`handler.go:handleToolsCall` parses `result_json` into `[]ContentItem` structs, then re-serializes them to JSON for the JSON-RPC response. For the common case (tool returns a valid content array), this parse/re-serialize is pure waste.

### Design

Replace the parse-and-rebuild with a direct `json.RawMessage` passthrough:

**Before:**
```go
var content []ContentItem
json.Unmarshal([]byte(resp.ResultJson), &content)
result := ToolsCallResult{Content: content, IsError: resp.IsError}
data, _ := json.Marshal(result)
```

**After:**
```go
result := RawToolsCallResult{
    Content: json.RawMessage(resp.ResultJson),
    IsError: resp.IsError,
}
data, _ := json.Marshal(result)
```

The `result_json` bytes pass through untouched. Use a fast check (first non-whitespace byte is `[`) to determine if the JSON is already a valid content array. If not, fall back to wrapping as a text content item — same as today's fallback path.

### Files

- `internal/mcp/handler.go` — modify `handleToolsCall` to use raw passthrough
- `internal/mcp/types.go` — add `RawToolsCallResult` with `json.RawMessage` content field

### Validation

- All existing tests pass (behavior unchanged for well-formed results)
- D4 payload benchmark (already exists in `fastmcp_deep_comparison_test.go`) shows improvement at 10KB+ sizes

---

## Phase A: Chunked Internal Transfer

### Problem

Even with Phase C, the protobuf layer serializes/deserializes the entire payload as one message. A 5MB result means ~15MB peak memory (serialized + deserialized + working copies). The length-prefixed framing also means the Go `readLoop` blocks until the entire message is received.

The existing `maxMessageSize` in `envelope.go` caps messages at 10MB, which limits non-chunked payloads. With chunking, individual messages stay small regardless of total payload size.

### Design

Add streaming message types to the internal protobuf protocol. When a tool result exceeds a configurable threshold, the SDK sends it as a stream of chunks.

#### New Protobuf Messages

```protobuf
message StreamHeader {
  string field_name = 1;     // which field is being streamed ("result_json")
  uint64 total_size = 2;     // total bytes if known, 0 if unknown
  uint32 chunk_size = 3;     // bytes per chunk
}

message StreamChunk {
  bytes data = 1;            // chunk payload
  bool final = 2;            // true on last chunk
}
```

Added to `Envelope.oneof msg` at the next available field numbers:
```protobuf
StreamHeader stream_header = 28;
StreamChunk stream_chunk = 29;
```

#### Wire Flow

For a 512KB `result_json` with 64KB chunks:

```
Tool → Go:  Envelope { request_id: "req-5", stream_header: { field_name: "result_json", total_size: 524288, chunk_size: 65536 } }
Tool → Go:  Envelope { request_id: "req-5", stream_chunk: { data: [65536 bytes], final: false } }
Tool → Go:  Envelope { request_id: "req-5", stream_chunk: { data: [65536 bytes], final: false } }
            ... (6 more chunks) ...
Tool → Go:  Envelope { request_id: "req-5", stream_chunk: { data: [remaining], final: true } }
```

Each chunk is still a length-prefixed protobuf Envelope — the framing layer doesn't change. The chunks are just smaller messages.

#### Go Side: readLoop Reassembly

The `readLoop` is a single goroutine, so it naturally handles interleaved chunks from concurrent streams (multiple in-flight tool calls) without additional synchronization.

When `readLoop` receives a `stream_header`:
1. Create a `streamAssembly` struct keyed by `request_id`: `{ fieldName string, buf bytes.Buffer, totalSize uint64, created time.Time }`
2. If `total_size > 0`, pre-allocate the buffer with `buf.Grow(totalSize)` to avoid repeated reallocation
3. If `total_size == 0` (unknown), the buffer grows dynamically via `bytes.Buffer`'s internal doubling
4. On each `stream_chunk` with matching `request_id`, append `data` to `buf`
5. On `final: true`, construct a `CallToolResponse` with the assembled field, dispatch to the pending channel, remove from assembly map
6. If `total_size > 0` and assembled size differs, log a warning but still dispatch (non-fatal)

The `CallTool` caller sees no difference — it still receives a complete `*pb.CallToolResponse` from the channel.

#### Error Handling and Cleanup

- **Tool crash mid-stream:** The `readLoop` exits on socket EOF/error. All in-progress assemblies are abandoned. The `CallTool` callers time out via their existing `timer` and return an error. No leaked goroutines.
- **Orphaned assemblies:** The `readLoop` periodically checks (on each envelope read) for assemblies older than `CallTimeout` and removes them. This handles edge cases where a tool sends a `stream_header` but never sends chunks.
- **Unknown request_id on chunk:** Discard the chunk silently (same behavior as today for unmatched response envelopes).
- **No explicit abort message:** The SDK can signal failure by sending a `CallToolResponse` with `is_error: true` instead of continuing chunks. The `readLoop` checks for a pending assembly on that `request_id` and cleans it up before dispatching the error response.

#### SDK Side: Transparent Chunking

The SDK's transport layer checks the serialized size of `result_json` before sending. If it exceeds the threshold (default 64KB), it:
1. Sends a `StreamHeader` envelope with `total_size` set to the known length
2. Sends N `StreamChunk` envelopes, each with up to `chunk_size` bytes
3. The final chunk has `final: true`
4. Tool author code is unchanged — chunking is entirely within the transport layer

#### Threshold

- Default: 64KB (configurable via environment variable `PROTOMCP_CHUNK_THRESHOLD`)
- Below threshold: current single-message path, zero overhead
- The threshold applies to the `result_json` field length, not the entire Envelope

### Files

- `proto/protomcp.proto` — add `StreamHeader`, `StreamChunk` to Envelope oneof (fields 28, 29)
- `internal/process/manager.go` — `readLoop` stream assembly logic, orphan cleanup
- `sdk/python/src/protomcp/transport.py` — chunked send logic
- `sdk/typescript/src/transport.ts` — chunked send logic
- Regenerate protobuf code for Go, Python, TypeScript

### Validation

- Existing tests pass (payloads under threshold use old path)
- New unit tests for stream assembly in `manager_test.go` (happy path, crash mid-stream, unknown-size, interleaved streams)
- D4 payload benchmark shows improvement at 100KB+ sizes

---

## Phase B: End-to-End Streaming

### Problem

Even with internal chunking, the Go proxy must reassemble the entire result before writing the JSON-RPC response. For a 50MB result, the proxy buffers 50MB in memory. The MCP host also blocks until the full response arrives.

No MCP implementation supports streaming tool results today. The spec doesn't address it. This is where protomcp can differentiate.

### Design

A protomcp-specific MCP extension — `x-protomcp-stream` — that lets hosts opt into chunked content delivery.

#### Capability Negotiation

During `initialize`, the host advertises streaming support:

```json
{
  "method": "initialize",
  "params": {
    "capabilities": {
      "x-protomcp-stream": { "maxChunkSize": 65536 }
    }
  }
}
```

If the host does not advertise `x-protomcp-stream`, the proxy falls back to full buffering (Phase A reassembly + single JSON-RPC response). Full backward compatibility.

The host's `maxChunkSize` is forwarded to the SDK as the chunk size for internal streaming (Phase A), aligning internal and external chunk boundaries to avoid re-buffering.

#### Streaming JSON-RPC Response Format

When streaming is enabled, large tool results are delivered as a sequence of JSON-RPC **notifications** (no `id` field) bracketed by the original response:

```json
{"jsonrpc":"2.0","method":"x-protomcp-stream/start","params":{"id":1,"streamId":"s-42","totalSize":524288}}
{"jsonrpc":"2.0","method":"x-protomcp-stream/chunk","params":{"streamId":"s-42","data":"<base64 or raw text>"}}
{"jsonrpc":"2.0","method":"x-protomcp-stream/chunk","params":{"streamId":"s-42","data":"<base64 or raw text>"}}
{"jsonrpc":"2.0","id":1,"result":{"content":[],"isError":false,"x-stream-complete":"s-42"}}
```

This format is valid JSON-RPC 2.0: the `start` and `chunk` messages are notifications (no `id`), and the final message is a standard response with `id` and `result`. Hosts that don't understand the notifications ignore them per the JSON-RPC spec. The final response contains metadata; the actual content was delivered via chunks.

#### Stream ID Generation

Stream IDs are generated as `s-{monotonic counter}` scoped to the connection (transport instance). The counter is an atomic uint64 on the handler, ensuring uniqueness across concurrent tool calls on the same connection.

#### Handler Architecture

The current `Handler.Handle()` returns `(*JSONRPCResponse, error)`. For streaming, a new method is introduced:

```go
type StreamWriter interface {
    WriteNotification(method string, params interface{}) error
    WriteResponse(resp *JSONRPCResponse) error
    Flush() error
}
```

The handler detects streaming capability (stored during `initialize`) and, for large results, calls `StreamWriter` methods instead of returning a single response. The transport implementations (`stdio`, `http`, `sse`) each implement `StreamWriter`.

For non-streaming hosts, the existing return-based interface is unchanged.

#### Transport-Specific Delivery

- **HTTP (streamable-http):** Chunked transfer encoding. Response headers sent on first write, chunks flushed as they arrive. Connection stays open until final response.
- **SSE:** Each notification is an SSE `data:` event. Standard SSE framing.
- **Stdio:** Each notification/response is a newline-delimited JSON line. Standard stdio framing.

#### Backpressure

The Go proxy reads the next internal chunk (Phase A) only after the previous chunk has been flushed to the transport via `StreamWriter.Flush()`. A slow HTTP client naturally throttles the tool process through the unix socket. No unbounded buffering anywhere in the pipeline.

#### What This Enables

- Tool processes return arbitrarily large results without OOM on any component
- Hosts see first bytes immediately (time-to-first-byte improves UX for large results)
- Agent-to-agent communication over MCP becomes viable for real data exchange
- File transfer, dataset streaming, log tailing — all possible through standard MCP tools
- Path to proposing this as a formal MCP spec extension

### Files

- `internal/mcp/handler.go` — detect streaming capability, use `StreamWriter` for large results
- `internal/mcp/types.go` — `StreamWriter` interface, streaming notification types
- `internal/transport/http.go` — implement `StreamWriter` with chunked transfer encoding
- `internal/transport/stdio.go` — implement `StreamWriter` with NDJSON
- `internal/transport/sse.go` — implement `StreamWriter` with SSE events

### Validation

- Non-streaming hosts get identical behavior to today (no `x-protomcp-stream` capability = full buffering)
- New integration tests with a streaming-capable test client
- Benchmark: time-to-first-byte for 1MB+ payloads
- Memory profile: proxy RSS stays flat regardless of payload size

---

## Phase Order and Independence

| Phase | Depends On | Ships Independently | Breaking Changes |
|-------|-----------|---------------------|-----------------|
| C     | Nothing   | Yes                 | None            |
| A     | Nothing   | Yes                 | None (proto additive) |
| B     | A         | Yes (with A)        | None (opt-in extension) |

Phase C and Phase A can be implemented in parallel. Phase B requires Phase A's chunking infrastructure on the internal side.

## Success Criteria

- **Phase C:** D4 benchmark at 100KB drops below FastMCP's 2.6ms
- **Phase A:** D4 benchmark at 500KB drops below FastMCP's 9ms; memory profile shows no single allocation > chunk size for large payloads
- **Phase B:** 50MB tool result streams to host with <100MB proxy RSS; time-to-first-byte < 10ms regardless of total payload size
