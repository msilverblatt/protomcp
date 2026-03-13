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

The `result_json` bytes pass through untouched. The only case we need to fall back to parsing is when `result_json` is not a valid JSON array (malformed input), in which case we wrap it as a text content item — same as today's fallback.

### Files

- `internal/mcp/handler.go` — modify `handleToolsCall` to use raw passthrough
- `internal/mcp/types.go` — add `RawToolsCallResult` with `json.RawMessage` content field

### Validation

- All existing tests pass (behavior unchanged for well-formed results)
- D4 payload benchmark shows improvement at 10KB+ sizes

---

## Phase A: Chunked Internal Transfer

### Problem

Even with Phase C, the protobuf layer serializes/deserializes the entire payload as one message. A 5MB result means ~15MB peak memory (serialized + deserialized + working copies). The length-prefixed framing also means the Go `readLoop` blocks until the entire message is received.

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

Added to `Envelope.oneof msg`:
```protobuf
StreamHeader stream_header = 16;
StreamChunk stream_chunk = 17;
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

When `readLoop` receives a `stream_header`:
1. Create a `streamAssembly` struct keyed by `request_id`: `{ fieldName string, buf bytes.Buffer, totalSize uint64 }`
2. On each `stream_chunk` with matching `request_id`, append `data` to `buf`
3. On `final: true`, construct a `CallToolResponse` with the assembled field, dispatch to the pending channel

The `CallTool` caller sees no difference — it still receives a complete `*pb.CallToolResponse` from the channel.

#### SDK Side: Transparent Chunking

The SDK's transport layer checks the serialized size of `result_json` before sending. If it exceeds the threshold (default 64KB), it:
1. Sends a `StreamHeader` envelope
2. Sends N `StreamChunk` envelopes
3. Tool author code is unchanged

#### Threshold

- Default: 64KB (configurable via environment variable `PROTOMCP_CHUNK_THRESHOLD`)
- Below threshold: current single-message path, zero overhead
- The threshold applies to the `result_json` field length, not the entire Envelope

### Files

- `proto/protomcp.proto` — add `StreamHeader`, `StreamChunk` to Envelope oneof
- `internal/process/manager.go` — `readLoop` stream assembly logic
- `sdk/python/src/protomcp/transport.py` — chunked send logic
- `sdk/typescript/src/transport.ts` — chunked send logic
- Regenerate protobuf code for Go, Python, TypeScript

### Validation

- Existing tests pass (payloads under threshold use old path)
- New unit tests for stream assembly in `manager_test.go`
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

#### Streaming JSON-RPC Response Format

When streaming is enabled, large tool results are delivered as a sequence of messages with a shared correlation ID:

```json
{"jsonrpc":"2.0","id":1,"x-stream":"start","x-stream-id":"s-1","x-total-size":524288}
{"jsonrpc":"2.0","x-stream-id":"s-1","x-stream":"chunk","x-data":"<base64 or raw text>"}
{"jsonrpc":"2.0","x-stream-id":"s-1","x-stream":"chunk","x-data":"<base64 or raw text>"}
{"jsonrpc":"2.0","id":1,"x-stream":"end","x-stream-id":"s-1","result":{"isError":false}}
```

The `start` message carries the original request `id`. Intermediate `chunk` messages omit `id` (they're not complete responses). The `end` message carries `id` again with the final metadata.

#### Transport-Specific Delivery

- **HTTP (streamable-http):** Chunked transfer encoding. Response headers sent immediately, chunks written as they arrive from the internal protocol. Connection stays open until `end`.
- **SSE:** Each chunk is an SSE `data:` event. The `event:` field carries `x-stream-chunk`. Correlation via `x-stream-id`.
- **Stdio:** Each chunk is a newline-delimited JSON message. Same format as above.

#### Backpressure

The Go proxy reads the next internal chunk only after the previous chunk has been flushed to the transport. A slow HTTP client naturally throttles the tool process through the unix socket. No unbounded buffering anywhere in the pipeline.

#### What This Enables

- Tool processes return arbitrarily large results without OOM on any component
- Hosts see first bytes immediately (time-to-first-byte improves UX for large results)
- Agent-to-agent communication over MCP becomes viable for real data exchange
- File transfer, dataset streaming, log tailing — all possible through standard MCP tools
- Path to proposing this as a formal MCP spec extension

### Files

- `internal/mcp/handler.go` — detect streaming capability, stream results when available
- `internal/mcp/types.go` — streaming message types
- `internal/transport/http.go` — chunked HTTP response writing
- `internal/transport/stdio.go` — streaming message format
- `internal/transport/sse.go` — streaming SSE events (if applicable)

### Validation

- Non-streaming hosts get identical behavior to today
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
