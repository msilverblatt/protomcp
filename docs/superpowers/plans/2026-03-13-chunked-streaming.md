# Chunked Streaming Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate protomcp's large-payload performance gap and build streaming content transfer across the internal protocol and MCP transports.

**Architecture:** Three phases — (C) zero-copy JSON passthrough in the handler to skip redundant parse/re-serialize, (A) chunked protobuf streaming over the internal unix socket for large payloads, (B) end-to-end streaming to MCP hosts via a JSON-RPC extension with a true streaming pipeline (no full reassembly). Each phase is backward compatible and independently shippable.

**Tech Stack:** Go (handler, process manager, transports), Protocol Buffers, Python SDK, TypeScript SDK

**Spec:** `docs/superpowers/specs/2026-03-13-chunked-streaming-design.md`

---

## Chunk 1: Phase C — Zero-Copy JSON Passthrough

### Task 1: Add RawToolsCallResult type

**Files:**
- Modify: `internal/mcp/types.go:83-92`

- [ ] **Step 1: Add the new type to types.go**

Add after the existing `ToolsCallResult` (line 87):

```go
// RawToolsCallResult is like ToolsCallResult but passes content through
// as raw JSON bytes, avoiding a parse/re-serialize round trip.
type RawToolsCallResult struct {
	Content           json.RawMessage `json:"content"`
	IsError           bool            `json:"isError,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/mcp/...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/types.go
git commit -m "feat: add RawToolsCallResult type for JSON passthrough"
```

---

### Task 2: Modify handleToolsCall for raw passthrough

**Files:**
- Modify: `internal/mcp/handler.go:139-188`
- Test: `internal/mcp/handler_test.go`

Currently `handleToolsCall` (lines 170-183) parses `result_json` into `[]ContentItem` then re-serializes. Replace with a fast-path that checks if `result_json` starts with `[` and passes it through as raw bytes.

- [ ] **Step 1: Write tests**

Add to `internal/mcp/handler_test.go`:

```go
func TestHandleToolsCall_RawPassthrough(t *testing.T) {
	// Verify that a large content array passes through without re-parsing.
	largeText := strings.Repeat("X", 100000)
	contentJSON := fmt.Sprintf(`[{"type":"text","text":"%s"}]`, largeText)

	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{{Name: "generate", InputSchemaJson: `{"type":"object"}`}},
		callResult: &pb.CallToolResponse{
			ResultJson: contentJSON,
		},
	}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "generate"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %s", resp.Error.Message)
	}

	// The content array should be embedded directly, not re-serialized.
	var result struct {
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)

	if string(result.Content) != contentJSON {
		t.Errorf("content was re-serialized instead of passed through.\nGot length: %d\nWant length: %d", len(result.Content), len(contentJSON))
	}
}

func TestHandleToolsCall_RawPassthrough_Fallback(t *testing.T) {
	// When result_json is NOT a JSON array, fall back to wrapping as text.
	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{{Name: "echo", InputSchemaJson: `{"type":"object"}`}},
		callResult: &pb.CallToolResponse{
			ResultJson: `"just a string"`,
		},
	}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "echo"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Content []mcp.ContentItem `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)

	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Errorf("expected fallback to text content, got: %+v", result.Content)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/mcp/ -run TestHandleToolsCall_RawPassthrough`
Expected: `TestHandleToolsCall_RawPassthrough` fails (content gets re-serialized)

- [ ] **Step 3: Implement the raw passthrough in handleToolsCall**

Replace lines 170-187 of `handler.go` (the content parsing block) with:

```go
	// Fast path: if result_json starts with '[', it's already a valid content
	// array — pass it through as raw bytes to avoid parse/re-serialize overhead.
	resultJSON := resp.ResultJson
	trimmed := strings.TrimSpace(resultJSON)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		result := RawToolsCallResult{
			Content: json.RawMessage(resultJSON),
			IsError: resp.IsError,
		}
		if resp.StructuredContentJson != "" {
			result.StructuredContent = json.RawMessage(resp.StructuredContentJson)
		}
		return h.success(req.ID, result)
	}

	// Fallback: result_json is not a JSON array — wrap as text content.
	var content []ContentItem
	if resultJSON != "" {
		if err := json.Unmarshal([]byte(resultJSON), &content); err != nil {
			content = []ContentItem{{Type: "text", Text: resultJSON}}
		}
	}
	result := ToolsCallResult{
		Content: content,
		IsError: resp.IsError,
	}
	if resp.StructuredContentJson != "" {
		result.StructuredContent = json.RawMessage(resp.StructuredContentJson)
	}
	return h.success(req.ID, result)
```

Add `"strings"` to the import block if not already present.

- [ ] **Step 4: Run all handler tests**

Run: `go test -v ./internal/mcp/`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/handler.go internal/mcp/handler_test.go internal/mcp/types.go
git commit -m "feat: zero-copy JSON passthrough for tool results

Skip json.Unmarshal/json.Marshal round trip when result_json is already
a valid content array (starts with '['). Falls back to old parse path
for non-array results. Eliminates 2 payload copies on the hot path."
```

---

### Task 3: Rebuild and run D4 payload benchmark

- [ ] **Step 1: Rebuild pmcp binary**

Run: `make build`

- [ ] **Step 2: Run D4 payload benchmark**

Run: `go test -v -timeout 120s ./tests/bench/comparison/ -run TestDeepFastMCPComparison/D4`
Expected: Improvement at 10KB+ sizes vs previous (protomcp was 3.5ms at 100KB, 17ms at 500KB)

- [ ] **Step 3: Run full test suite**

Run: `go test ./internal/mcp/ ./internal/process/ ./internal/envelope/`
Expected: All pass

---

## Chunk 2: Phase A — Chunked Internal Transfer

### Task 4: Add StreamHeader and StreamChunk to protobuf schema

**Files:**
- Modify: `proto/protomcp.proto`

- [ ] **Step 1: Add message definitions and oneof fields**

In `Envelope.oneof msg`, after `middleware_intercept_response = 27` (line 42), add:

```protobuf
    // Streaming
    StreamHeader stream_header = 28;
    StreamChunk stream_chunk = 29;
```

At the end of the file, add:

```protobuf
// StreamHeader initiates a chunked transfer for a large field.
message StreamHeader {
  string field_name = 1;     // field being streamed, e.g. "result_json"
  uint64 total_size = 2;     // total bytes if known, 0 if unknown
  uint32 chunk_size = 3;     // bytes per chunk
}

// StreamChunk carries one chunk of a streamed field.
message StreamChunk {
  bytes data = 1;            // chunk payload
  bool final = 2;            // true on last chunk
}
```

- [ ] **Step 2: Regenerate all protobuf code (Go, Python, TypeScript)**

Run: `make proto`

This runs the Makefile `proto` target which generates:
- Go: `gen/proto/protomcp/protomcp.pb.go`
- Python: `sdk/python/gen/protomcp_pb2.py`
- TypeScript: `sdk/typescript/gen/` (via `protoc-gen-ts`)

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add proto/protomcp.proto gen/ sdk/python/gen/ sdk/typescript/gen/
git commit -m "proto: add StreamHeader and StreamChunk messages for chunked transfer"
```

---

### Task 5: Implement stream reassembly in readLoop

**Files:**
- Modify: `internal/process/manager.go`
- Test: `internal/process/manager_test.go`

- [ ] **Step 1: Write tests for stream reassembly (happy path + error cases)**

Add to `internal/process/manager_test.go`. These tests use a connected socket pair to simulate the tool process, writing protobuf envelopes directly.

```go
// testStreamSetup creates a unix socket pair and a Manager wired to
// the server side. Returns the Manager, the tool-side conn, and a cleanup func.
func testStreamSetup(t *testing.T) (*process.Manager, net.Conn) {
	t.Helper()
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pmcp-test-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	t.Cleanup(func() { os.Remove(socketPath) })

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	toolConn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { toolConn.Close() })

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { serverConn.Close() })

	cfg := process.ManagerConfig{
		SocketPath:  socketPath,
		CallTimeout: 5 * time.Second,
	}
	mgr := process.NewManagerForTest(cfg, serverConn)
	go mgr.StartReadLoop()

	return mgr, toolConn
}

func TestStreamReassembly(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh := mgr.RegisterPending("req-1")

	// 200KB payload, 64KB chunks
	fullPayload := strings.Repeat("A", 200*1024)
	fullResultJSON := fmt.Sprintf(`[{"type":"text","text":"%s"}]`, fullPayload)
	chunkSize := 64 * 1024

	// Send stream_header
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{
			StreamHeader: &pb.StreamHeader{
				FieldName: "result_json",
				TotalSize: uint64(len(fullResultJSON)),
				ChunkSize: uint32(chunkSize),
			},
		},
	})

	// Send chunks
	remaining := []byte(fullResultJSON)
	for len(remaining) > 0 {
		sz := chunkSize
		if sz > len(remaining) {
			sz = len(remaining)
		}
		envelope.Write(toolConn, &pb.Envelope{
			RequestId: "req-1",
			Msg: &pb.Envelope_StreamChunk{
				StreamChunk: &pb.StreamChunk{
					Data:  remaining[:sz],
					Final: sz >= len(remaining),
				},
			},
		})
		remaining = remaining[sz:]
	}

	select {
	case resp := <-respCh:
		result := resp.GetCallResult()
		if result == nil {
			t.Fatal("expected CallToolResponse")
		}
		if result.ResultJson != fullResultJSON {
			t.Errorf("reassembled length = %d, want %d", len(result.ResultJson), len(fullResultJSON))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestStreamReassembly_UnknownSize(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh := mgr.RegisterPending("req-1")

	payload := `[{"type":"text","text":"hello"}]`

	// total_size = 0 (unknown)
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{
			StreamHeader: &pb.StreamHeader{
				FieldName: "result_json",
				TotalSize: 0,
				ChunkSize: 1024,
			},
		},
	})
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamChunk{
			StreamChunk: &pb.StreamChunk{Data: []byte(payload), Final: true},
		},
	})

	select {
	case resp := <-respCh:
		if resp.GetCallResult().ResultJson != payload {
			t.Errorf("got %q, want %q", resp.GetCallResult().ResultJson, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestStreamReassembly_UnknownRequestID(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	_ = mgr // readLoop is running

	// Send a chunk with no matching stream_header — should be silently discarded.
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-orphan",
		Msg: &pb.Envelope_StreamChunk{
			StreamChunk: &pb.StreamChunk{Data: []byte("hello"), Final: true},
		},
	})

	// Send a normal response to verify readLoop is still working.
	respCh := mgr.RegisterPending("req-2")
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-2",
		Msg: &pb.Envelope_CallResult{
			CallResult: &pb.CallToolResponse{ResultJson: `[{"type":"text","text":"ok"}]`},
		},
	})

	select {
	case resp := <-respCh:
		if resp.GetCallResult().ResultJson != `[{"type":"text","text":"ok"}]` {
			t.Error("unexpected result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout — readLoop may have crashed on orphan chunk")
	}
}

func TestStreamReassembly_InterleavedStreams(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh1 := mgr.RegisterPending("req-1")
	respCh2 := mgr.RegisterPending("req-2")

	payload1 := `[{"type":"text","text":"AAAA"}]`
	payload2 := `[{"type":"text","text":"BBBB"}]`

	// Start both streams
	for _, id := range []string{"req-1", "req-2"} {
		envelope.Write(toolConn, &pb.Envelope{
			RequestId: id,
			Msg: &pb.Envelope_StreamHeader{
				StreamHeader: &pb.StreamHeader{FieldName: "result_json", ChunkSize: 1024},
			},
		})
	}

	// Interleave chunks: req-1 chunk, req-2 chunk, req-1 final, req-2 final
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg:       &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload1[:15])}},
	})
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-2",
		Msg:       &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload2[:15])}},
	})
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg:       &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload1[15:]), Final: true}},
	})
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-2",
		Msg:       &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload2[15:]), Final: true}},
	})

	for i, ch := range []chan *pb.Envelope{respCh1, respCh2} {
		want := payload1
		if i == 1 {
			want = payload2
		}
		select {
		case resp := <-ch:
			if resp.GetCallResult().ResultJson != want {
				t.Errorf("stream %d: got %q, want %q", i+1, resp.GetCallResult().ResultJson, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("stream %d: timeout", i+1)
		}
	}
}

func TestStreamReassembly_CrashMidStream(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pmcp-crash-%d.sock", os.Getpid()))
	defer os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	toolConn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}

	cfg := process.ManagerConfig{SocketPath: socketPath, CallTimeout: 2 * time.Second}
	mgr := process.NewManagerForTest(cfg, serverConn)
	done := make(chan struct{})
	go func() { mgr.StartReadLoop(); close(done) }()

	respCh := mgr.RegisterPending("req-1")

	// Send header but no chunks, then close the tool connection.
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{
			StreamHeader: &pb.StreamHeader{FieldName: "result_json", TotalSize: 100000, ChunkSize: 1024},
		},
	})
	toolConn.Close()

	// readLoop should exit. The caller should NOT receive a response.
	select {
	case <-done:
		// readLoop exited — good
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop didn't exit after tool crash")
	}

	// The pending channel should have no message; caller times out.
	select {
	case resp := <-respCh:
		t.Fatalf("unexpected response after crash: %v", resp)
	default:
		// Expected: no response
	}
}
```

- [ ] **Step 2: Add test helpers to manager.go**

Add to `internal/process/manager.go`:

```go
// NewManagerForTest creates a Manager with a pre-established connection.
// Used by tests that drive the protocol directly without spawning a process.
func NewManagerForTest(cfg ManagerConfig, conn net.Conn) *Manager {
	m := NewManager(cfg)
	m.conn = conn
	return m
}

// RegisterPending registers a pending request channel and returns it.
// Used by tests to receive responses from readLoop.
func (m *Manager) RegisterPending(reqID string) chan *pb.Envelope {
	ch := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = ch
	m.mu.Unlock()
	return ch
}

// StartReadLoop starts the readLoop (blocking). Used by tests.
func (m *Manager) StartReadLoop() {
	m.readWg.Add(1)
	m.readLoop()
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test -v ./internal/process/ -run TestStream`
Expected: Fail — readLoop doesn't handle `stream_header`/`stream_chunk`

- [ ] **Step 4: Implement stream reassembly in readLoop**

Add to `manager.go`:

```go
// streamAssembly tracks an in-progress chunked transfer.
type streamAssembly struct {
	fieldName string
	buf       bytes.Buffer
	totalSize uint64
	created   time.Time
}
```

Add field to `Manager` struct:

```go
	streams map[string]*streamAssembly
```

Initialize in `NewManager`:

```go
	streams: make(map[string]*streamAssembly),
```

In `readLoop`, after the `stopCh` select and before `envelope.Read`, add orphan cleanup:

```go
		// Clean up orphaned stream assemblies.
		now := time.Now()
		for id, asm := range m.streams {
			if now.Sub(asm.created) > m.cfg.CallTimeout {
				delete(m.streams, id)
			}
		}
```

After reading the envelope and extracting `reqID` (after the unsolicited message routing, before the `m.pending` dispatch), add stream handling:

```go
		// Stream reassembly.
		if sh := env.GetStreamHeader(); sh != nil {
			assembly := &streamAssembly{
				fieldName: sh.FieldName,
				totalSize: sh.TotalSize,
				created:   time.Now(),
			}
			if sh.TotalSize > 0 {
				assembly.buf.Grow(int(sh.TotalSize))
			}
			m.streams[reqID] = assembly
			continue
		}

		if sc := env.GetStreamChunk(); sc != nil {
			assembly, ok := m.streams[reqID]
			if !ok {
				continue // no header — discard silently
			}
			assembly.buf.Write(sc.Data)

			if sc.Final {
				delete(m.streams, reqID)
				result := &pb.Envelope{
					RequestId: reqID,
					Msg: &pb.Envelope_CallResult{
						CallResult: &pb.CallToolResponse{},
					},
				}
				switch assembly.fieldName {
				case "result_json":
					result.GetCallResult().ResultJson = assembly.buf.String()
				case "structured_content_json":
					result.GetCallResult().StructuredContentJson = assembly.buf.String()
				}

				m.mu.Lock()
				ch, chOk := m.pending[reqID]
				m.mu.Unlock()
				if chOk {
					select {
					case ch <- result:
					default:
					}
				}
			}
			continue
		}
```

Add `"bytes"` to imports if needed (`"time"` should already be there).

- [ ] **Step 5: Run all stream tests**

Run: `go test -v ./internal/process/ -run TestStream`
Expected: All 5 tests pass

- [ ] **Step 6: Run all process tests**

Run: `go test -v ./internal/process/`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add internal/process/manager.go internal/process/manager_test.go
git commit -m "feat: stream reassembly in readLoop for chunked transfers

Handles StreamHeader/StreamChunk envelopes, accumulating chunks into a
bytes.Buffer and dispatching a complete CallToolResponse on final chunk.
Includes orphan cleanup, interleaved stream support, and crash handling."
```

---

### Task 6: Implement chunked send in Python SDK

**Files:**
- Modify: `sdk/python/src/protomcp/transport.py`
- Modify: `sdk/python/src/protomcp/runner.py`

- [ ] **Step 1: Add `send_chunked` to transport.py**

Add after the existing `send()` method (line 22 of `transport.py`):

```python
    def send_chunked(self, request_id: str, field_name: str, data: bytes,
                     chunk_size: int = 65536):
        """Send a large field as StreamHeader + StreamChunk messages."""
        header = pb.Envelope(
            request_id=request_id,
            stream_header=pb.StreamHeader(
                field_name=field_name,
                total_size=len(data),
                chunk_size=chunk_size,
            ),
        )
        self.send(header)

        offset = 0
        while offset < len(data):
            end = min(offset + chunk_size, len(data))
            is_final = (end >= len(data))
            chunk_env = pb.Envelope(
                request_id=request_id,
                stream_chunk=pb.StreamChunk(
                    data=data[offset:end],
                    final=is_final,
                ),
            )
            self.send(chunk_env)
            offset = end
```

- [ ] **Step 2: Modify runner.py to use chunked send for large results**

In `_handle_call_tool()` (around line 120-131 of `runner.py`), replace the response send with:

```python
    # Check if result_json exceeds chunk threshold — stream if so.
    chunk_threshold = int(os.environ.get('PROTOMCP_CHUNK_THRESHOLD', '65536'))
    result_json_str = resp_msg.result_json
    result_json_bytes = result_json_str.encode('utf-8') if result_json_str else b''

    if len(result_json_bytes) > chunk_threshold:
        transport.send_chunked(
            request_id=env.request_id,
            field_name='result_json',
            data=result_json_bytes,
        )
    else:
        resp = pb.Envelope(call_result=resp_msg, request_id=env.request_id)
        transport.send(resp)
```

Add `import os` at the top if not already present.

- [ ] **Step 3: Run existing integration tests**

Run: `go test -v ./internal/process/`
Expected: All pass (test payloads are small, below default 64KB threshold)

- [ ] **Step 4: Commit**

```bash
git add sdk/python/src/protomcp/transport.py sdk/python/src/protomcp/runner.py
git commit -m "feat: chunked send in Python SDK for large payloads

When result_json exceeds PROTOMCP_CHUNK_THRESHOLD (default 64KB),
sends StreamHeader + StreamChunk messages instead of a single envelope."
```

---

### Task 7: Implement chunked send in TypeScript SDK

**Files:**
- Modify: `sdk/typescript/src/transport.ts`
- Modify: `sdk/typescript/src/runner.ts`

- [ ] **Step 1: Add `sendChunked` to transport.ts**

Add after the existing `send()` method (line 108 of `transport.ts`):

```typescript
  async sendChunked(requestId: string, fieldName: string, data: Buffer, chunkSize: number = 65536): Promise<void> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');
    const StreamHeader = root.lookupType('protomcp.StreamHeader');
    const StreamChunk = root.lookupType('protomcp.StreamChunk');

    // Send header
    const header = Envelope.create({
      requestId,
      streamHeader: StreamHeader.create({
        fieldName,
        totalSize: data.length,
        chunkSize,
      }),
    });
    await this.send(header);

    // Send chunks
    let offset = 0;
    while (offset < data.length) {
      const end = Math.min(offset + chunkSize, data.length);
      const isFinal = end >= data.length;
      const chunk = Envelope.create({
        requestId,
        streamChunk: StreamChunk.create({
          data: data.subarray(offset, end),
          final: isFinal,
        }),
      });
      await this.send(chunk);
      offset = end;
    }
  }
```

- [ ] **Step 2: Modify runner.ts to use chunked send for large results**

In `runner.ts`, find the block where `callTool` responses are sent (around line 180: `const resp = Envelope.create({ callResult: respMsg, requestId });` and `await transport.send(resp);`).

Replace that block with:

```typescript
      // Check if result_json exceeds chunk threshold — stream if so.
      const chunkThreshold = parseInt(process.env['PROTOMCP_CHUNK_THRESHOLD'] ?? '65536', 10);
      const resultJson = (respMsg as any).resultJson as string ?? '';
      const resultBytes = Buffer.from(resultJson, 'utf-8');

      if (resultBytes.length > chunkThreshold) {
        await transport.sendChunked(requestId, 'result_json', resultBytes);
      } else {
        const resp = Envelope.create({ callResult: respMsg, requestId });
        await transport.send(resp);
      }
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd sdk/typescript && npm run build`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add sdk/typescript/src/transport.ts sdk/typescript/src/runner.ts
git commit -m "feat: chunked send in TypeScript SDK for large payloads"
```

---

### Task 8: Integration test — large payload through full Python stack

**Files:**
- Create: `internal/process/stream_integration_test.go`

The `echo_tool.py` fixture supports a `generate` tool that returns a string of requested size. This test spawns it, calls `generate` with a payload above the chunk threshold, and verifies the Go side reassembles correctly.

- [ ] **Step 1: Write integration test**

```go
package process_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

func TestChunkedStreamIntegration(t *testing.T) {
	testutil.SetupPythonPath()

	// Set chunk threshold to 1KB to force chunking for moderate payloads.
	os.Setenv("PROTOMCP_CHUNK_THRESHOLD", "1024")
	defer os.Unsetenv("PROTOMCP_CHUNK_THRESHOLD")

	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pmcp-stream-integ-%d.sock", os.Getpid()))

	cfg := process.ManagerConfig{
		File:        testutil.FixturePath("tests/bench/fixtures/echo_tool.py"),
		SocketPath:  socketPath,
		CallTimeout: 10 * time.Second,
	}

	mgr := process.NewManager(cfg)
	ctx := context.Background()
	tools, err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Stop()

	if len(tools) == 0 {
		t.Fatal("no tools returned")
	}

	// Call generate with 100KB — well above 1KB threshold.
	args, _ := json.Marshal(map[string]int{"size": 100 * 1024})
	resp, err := mgr.CallTool(ctx, "generate", string(args))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	if resp.IsError {
		t.Fatalf("tool returned error: %s", resp.ResultJson)
	}

	var content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(resp.ResultJson), &content); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(content) != 1 || len(content[0].Text) != 100*1024 {
		t.Errorf("expected 100KB text, got %d bytes", len(content[0].Text))
	}
}
```

Note: The `echo_tool.py` fixture uses the raw protobuf protocol directly (not the SDK runner), so chunking will need to be tested either by modifying it or by using a fixture that imports the SDK. If `echo_tool.py` doesn't go through the SDK's `runner.py`, this test should instead use a fixture that does. Check which fixtures use the SDK runner vs raw protocol and adjust accordingly.

- [ ] **Step 2: Run integration test**

Run: `go test -v ./internal/process/ -run TestChunkedStreamIntegration -timeout 30s`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/process/stream_integration_test.go
git commit -m "test: integration test for chunked stream through full Python stack"
```

---

### Task 9: Rebuild and run D4 benchmark

- [ ] **Step 1: Rebuild**

Run: `make build`

- [ ] **Step 2: Run D4 benchmark**

Run: `PROTOMCP_CHUNK_THRESHOLD=65536 go test -v -timeout 120s ./tests/bench/comparison/ -run TestDeepFastMCPComparison/D4`
Expected: Improvement at 100KB+ sizes. Target: beat FastMCP at 500KB.

---

## Chunk 3: Phase B — End-to-End Streaming

### Task 10: Add StreamWriter interface and transport implementations

**Files:**
- Modify: `internal/mcp/types.go`
- Modify: `internal/transport/stdio.go`
- Modify: `internal/transport/http.go`
- Modify: `internal/transport/sse.go`

- [ ] **Step 1: Define StreamWriter interface in types.go**

Add to `internal/mcp/types.go`:

```go
// StreamWriter supports writing streaming responses to a transport.
type StreamWriter interface {
	WriteNotification(method string, params interface{}) error
	WriteResponse(resp *JSONRPCResponse) error
	Flush() error
}

// StreamStartParams is sent as the first notification in a streaming response.
type StreamStartParams struct {
	ID        json.RawMessage `json:"id"`
	StreamID  string          `json:"streamId"`
	TotalSize int             `json:"totalSize,omitempty"`
}

// StreamChunkParams carries one chunk of streaming content.
type StreamChunkParams struct {
	StreamID string `json:"streamId"`
	Data     string `json:"data"`
}

// StreamCompleteResult is the final response with stream metadata.
type StreamCompleteResult struct {
	Content         json.RawMessage `json:"content,omitempty"`
	IsError         bool            `json:"isError,omitempty"`
	XStreamComplete string          `json:"x-stream-complete,omitempty"`
}
```

- [ ] **Step 2: Implement StreamWriter for StdioTransport**

Add to `internal/transport/stdio.go`:

```go
// NewStreamWriter returns a StreamWriter for the stdio transport.
func (s *StdioTransport) NewStreamWriter() mcp.StreamWriter {
	return &stdioStreamWriter{s: s}
}

type stdioStreamWriter struct {
	s *StdioTransport
}

func (w *stdioStreamWriter) WriteNotification(method string, params interface{}) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return w.s.SendNotification(mcp.JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	})
}

func (w *stdioStreamWriter) WriteResponse(resp *mcp.JSONRPCResponse) error {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = w.s.writer.Write(append(data, '\n'))
	return err
}

func (w *stdioStreamWriter) Flush() error {
	return nil // stdio is unbuffered
}
```

- [ ] **Step 3: Implement StreamWriter for HTTPTransport**

Add to `internal/transport/http.go`:

```go
// httpStreamWriter writes streaming responses using HTTP chunked transfer encoding.
type httpStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	started bool
}

func newHTTPStreamWriter(w http.ResponseWriter) *httpStreamWriter {
	flusher, _ := w.(http.Flusher)
	return &httpStreamWriter{w: w, flusher: flusher}
}

func (sw *httpStreamWriter) ensureHeaders() {
	if !sw.started {
		sw.w.Header().Set("Content-Type", "application/x-ndjson")
		sw.w.WriteHeader(http.StatusOK)
		sw.started = true
	}
}

func (sw *httpStreamWriter) WriteNotification(method string, params interface{}) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	notif := mcp.JSONRPCNotification{JSONRPC: "2.0", Method: method, Params: p}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensureHeaders()
	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	_, err = sw.w.Write(append(data, '\n'))
	return err
}

func (sw *httpStreamWriter) WriteResponse(resp *mcp.JSONRPCResponse) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensureHeaders()
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = sw.w.Write(append(data, '\n'))
	return err
}

func (sw *httpStreamWriter) Flush() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return nil
}
```

- [ ] **Step 4: Implement StreamWriter for SSETransport**

Add to `internal/transport/sse.go`:

```go
// sseStreamWriter writes streaming responses as SSE events.
type sseStreamWriter struct {
	transport *SSETransport
}

// NewStreamWriter returns a StreamWriter that broadcasts via SSE.
func (s *SSETransport) NewStreamWriter() mcp.StreamWriter {
	return &sseStreamWriter{transport: s}
}

func (sw *sseStreamWriter) WriteNotification(method string, params interface{}) error {
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return sw.transport.SendNotification(mcp.JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	})
}

func (sw *sseStreamWriter) WriteResponse(resp *mcp.JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	sw.transport.broadcast(data)
	return nil
}

func (sw *sseStreamWriter) Flush() error {
	return nil // SSE flushes per-event in the broadcast handler
}
```

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/...`
Expected: Success

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/types.go internal/transport/stdio.go internal/transport/http.go internal/transport/sse.go
git commit -m "feat: StreamWriter interface with stdio, HTTP, and SSE implementations"
```

---

### Task 11: Store streaming capability during initialize

**Files:**
- Modify: `internal/mcp/handler.go`
- Test: `internal/mcp/handler_test.go`

- [ ] **Step 1: Add streaming fields to Handler struct**

```go
	streamCapable  bool
	streamMaxChunk int
	streamWriter   mcp.StreamWriter
	streamIDSeq    uint64
```

Add getter/setter methods:

```go
func (h *Handler) StreamCapable() bool   { return h.streamCapable }
func (h *Handler) StreamMaxChunk() int   { return h.streamMaxChunk }
func (h *Handler) SetStreamWriter(sw StreamWriter) { h.streamWriter = sw }
```

- [ ] **Step 2: Parse capability in handleInitialize**

At the start of `handleInitialize()`, before the existing response construction:

```go
	if req.Params != nil {
		var initParams struct {
			Capabilities map[string]json.RawMessage `json:"capabilities"`
		}
		if err := json.Unmarshal(req.Params, &initParams); err == nil {
			if streamCap, ok := initParams.Capabilities["x-protomcp-stream"]; ok {
				h.streamCapable = true
				var sc struct {
					MaxChunkSize int `json:"maxChunkSize"`
				}
				if json.Unmarshal(streamCap, &sc) == nil && sc.MaxChunkSize > 0 {
					h.streamMaxChunk = sc.MaxChunkSize
				} else {
					h.streamMaxChunk = 65536
				}
			}
		}
	}
```

- [ ] **Step 3: Write test**

```go
func TestHandleInitialize_StreamCapability(t *testing.T) {
	backend := &mockToolBackend{}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"x-protomcp-stream": map[string]int{"maxChunkSize": 32768},
		},
		"clientInfo": map[string]string{"name": "test"},
	})

	_, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize", Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !h.StreamCapable() {
		t.Error("expected stream capable")
	}
	if h.StreamMaxChunk() != 32768 {
		t.Errorf("expected maxChunk 32768, got %d", h.StreamMaxChunk())
	}
}

func TestHandleToolsCall_LargePayload_NoStreaming(t *testing.T) {
	// Without x-protomcp-stream capability, large payloads should still
	// work via the normal buffered path (raw passthrough from Phase C).
	largeText := strings.Repeat("X", 200000)
	contentJSON := fmt.Sprintf(`[{"type":"text","text":"%s"}]`, largeText)

	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{{Name: "generate", InputSchemaJson: `{"type":"object"}`}},
		callResult: &pb.CallToolResponse{ResultJson: contentJSON},
	}
	h := mcp.NewHandler(backend)

	// Initialize WITHOUT streaming capability
	initParams, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "test"},
	})
	h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`0`), Method: "initialize", Params: initParams,
	})

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "generate"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("error: %s", resp.Error.Message)
	}

	// Should get a normal (non-streamed) response with full content
	var result struct {
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)
	if len(result.Content) != len(contentJSON) {
		t.Errorf("expected full content in response, got length %d vs %d", len(result.Content), len(contentJSON))
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test -v ./internal/mcp/ -run TestHandleInitialize_StreamCapability`
Run: `go test -v ./internal/mcp/ -run TestHandleToolsCall_LargePayload_NoStreaming`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/handler.go internal/mcp/handler_test.go
git commit -m "feat: detect x-protomcp-stream capability during initialize"
```

---

### Task 12: Add streaming chunk channel to process manager

**Files:**
- Modify: `internal/process/manager.go`
- Test: `internal/process/manager_test.go`

For true end-to-end streaming (no full reassembly), the process manager needs a way to forward chunks directly to the handler instead of accumulating them. Add a `CallToolStream` method that returns a channel of stream events.

- [ ] **Step 1: Define StreamEvent type**

Add to `internal/process/manager.go`:

```go
// StreamEvent represents one event in a chunked tool call response.
type StreamEvent struct {
	// Header is set for the first event (stream start).
	Header *pb.StreamHeader
	// Chunk is set for data events.
	Chunk []byte
	// Final is true when this is the last chunk.
	Final bool
	// Result is set when the tool returns a non-streamed response
	// (payload was below threshold).
	Result *pb.CallToolResponse
}
```

- [ ] **Step 2: Add CallToolStream method**

```go
// CallToolStream sends a CallToolRequest and returns a channel that receives
// stream events. If the tool responds with a single (non-chunked) message,
// the channel receives one StreamEvent with Result set. If the tool streams,
// it receives a Header event followed by Chunk events.
func (m *Manager) CallToolStream(ctx context.Context, name, argsJSON string) (<-chan StreamEvent, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_CallTool{
			CallTool: &pb.CallToolRequest{
				Name:          name,
				ArgumentsJson: argsJSON,
			},
		},
	}

	ch := make(chan StreamEvent, 16)

	m.mu.Lock()
	m.streamChs[reqID] = ch
	m.mu.Unlock()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		m.mu.Lock()
		delete(m.streamChs, reqID)
		m.mu.Unlock()
		close(ch)
		return nil, fmt.Errorf("write CallToolRequest: %w", err)
	}

	// Cleanup on context cancellation or timeout.
	go func() {
		timeout := m.cfg.CallTimeout
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-ctx.Done():
		case <-timer.C:
		}

		m.mu.Lock()
		if _, ok := m.streamChs[reqID]; ok {
			delete(m.streamChs, reqID)
			close(ch)
		}
		m.mu.Unlock()
	}()

	return ch, nil
}
```

- [ ] **Step 3: Add streamChs map to Manager**

Add to `Manager` struct:

```go
	streamChs map[string]chan StreamEvent // for streaming tool call responses
```

Initialize in `NewManager`:

```go
	streamChs: make(map[string]chan StreamEvent),
```

- [ ] **Step 4: Modify readLoop to dispatch to streamChs**

In the readLoop, the stream handling code (from Task 5) needs to check `streamChs` first. If a stream channel exists for the request ID, dispatch events there instead of accumulating in `streamAssembly`.

Modify the `stream_header` handler:

```go
		if sh := env.GetStreamHeader(); sh != nil {
			m.mu.Lock()
			sCh, isStream := m.streamChs[reqID]
			m.mu.Unlock()

			if isStream {
				// Streaming mode — forward header to channel.
				select {
				case sCh <- StreamEvent{Header: sh}:
				default:
				}
			} else {
				// Reassembly mode (non-streaming host).
				assembly := &streamAssembly{
					fieldName: sh.FieldName,
					totalSize: sh.TotalSize,
					created:   time.Now(),
				}
				if sh.TotalSize > 0 {
					assembly.buf.Grow(int(sh.TotalSize))
				}
				m.streams[reqID] = assembly
			}
			continue
		}
```

Modify the `stream_chunk` handler:

```go
		if sc := env.GetStreamChunk(); sc != nil {
			m.mu.Lock()
			sCh, isStream := m.streamChs[reqID]
			m.mu.Unlock()

			if isStream {
				// Streaming mode — forward chunk to channel.
				evt := StreamEvent{Chunk: sc.Data, Final: sc.Final}
				select {
				case sCh <- evt:
				default:
				}
				if sc.Final {
					m.mu.Lock()
					delete(m.streamChs, reqID)
					m.mu.Unlock()
					close(sCh)
				}
			} else {
				// Reassembly mode.
				assembly, ok := m.streams[reqID]
				if !ok {
					continue
				}
				assembly.buf.Write(sc.Data)
				if sc.Final {
					delete(m.streams, reqID)
					result := &pb.Envelope{
						RequestId: reqID,
						Msg: &pb.Envelope_CallResult{
							CallResult: &pb.CallToolResponse{},
						},
					}
					switch assembly.fieldName {
					case "result_json":
						result.GetCallResult().ResultJson = assembly.buf.String()
					case "structured_content_json":
						result.GetCallResult().StructuredContentJson = assembly.buf.String()
					}
					m.mu.Lock()
					ch, chOk := m.pending[reqID]
					m.mu.Unlock()
					if chOk {
						select {
						case ch <- result:
						default:
						}
					}
				}
			}
			continue
		}
```

Also handle the case where a non-streamed response arrives for a `streamChs` request:

```go
		// Normal response dispatch — check streamChs first.
		m.mu.Lock()
		sCh, isStream := m.streamChs[reqID]
		ch, isPending := m.pending[reqID]
		m.mu.Unlock()

		if isStream {
			// Tool returned a non-chunked response to a streaming request.
			result := env.GetCallResult()
			if result != nil {
				select {
				case sCh <- StreamEvent{Result: result}:
				default:
				}
			}
			m.mu.Lock()
			delete(m.streamChs, reqID)
			m.mu.Unlock()
			close(sCh)
		} else if isPending {
			select {
			case ch <- env:
			default:
			}
		}
```

- [ ] **Step 5: Write test for CallToolStream**

```go
func TestCallToolStream(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)

	payload := `[{"type":"text","text":"` + strings.Repeat("X", 200*1024) + `"}]`
	chunkSize := 64 * 1024

	// Start streaming call in background
	ctx := context.Background()
	ch, err := mgr.CallToolStream(ctx, "generate", `{"size":204800}`)
	if err != nil {
		t.Fatal(err)
	}

	// Read the CallToolRequest from the tool side
	toolEnv, err := envelope.Read(toolConn)
	if err != nil {
		t.Fatal(err)
	}
	reqID := toolEnv.GetRequestId()

	// Send header + chunks from tool side
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_StreamHeader{
			StreamHeader: &pb.StreamHeader{
				FieldName: "result_json",
				TotalSize: uint64(len(payload)),
				ChunkSize: uint32(chunkSize),
			},
		},
	})

	remaining := []byte(payload)
	for len(remaining) > 0 {
		sz := chunkSize
		if sz > len(remaining) {
			sz = len(remaining)
		}
		envelope.Write(toolConn, &pb.Envelope{
			RequestId: reqID,
			Msg: &pb.Envelope_StreamChunk{
				StreamChunk: &pb.StreamChunk{
					Data:  remaining[:sz],
					Final: sz >= len(remaining),
				},
			},
		})
		remaining = remaining[sz:]
	}

	// Read events from channel
	var gotHeader bool
	var assembled []byte
	for evt := range ch {
		if evt.Header != nil {
			gotHeader = true
			if evt.Header.TotalSize != uint64(len(payload)) {
				t.Errorf("header total_size = %d, want %d", evt.Header.TotalSize, len(payload))
			}
		}
		if evt.Chunk != nil {
			assembled = append(assembled, evt.Chunk...)
		}
	}

	if !gotHeader {
		t.Error("never received header event")
	}
	if string(assembled) != payload {
		t.Errorf("assembled length = %d, want %d", len(assembled), len(payload))
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test -v ./internal/process/ -run TestCallToolStream`
Expected: PASS

Run: `go test -v ./internal/process/`
Expected: All tests still pass (reassembly tests use `RegisterPending`, not `CallToolStream`)

- [ ] **Step 7: Commit**

```bash
git add internal/process/manager.go internal/process/manager_test.go
git commit -m "feat: CallToolStream for true streaming without full reassembly

Adds StreamEvent type and CallToolStream method. readLoop dispatches
chunks directly to a channel when streaming mode is active, bypassing
the reassembly buffer. Enables bounded-memory streaming in Phase B."
```

---

### Task 13: Implement streaming tool call response in handler

**Files:**
- Modify: `internal/mcp/handler.go`
- Test: `internal/mcp/handler_test.go`

When the handler has a stream-capable client and a large result, it uses `CallToolStream` from the process manager to forward chunks directly to the `StreamWriter` — no full reassembly.

- [ ] **Step 1: Add StreamingBackend interface**

The handler needs access to `CallToolStream` in addition to `CallTool`. Add to `handler.go`:

```go
// StreamingBackend extends ToolBackend with streaming support.
type StreamingBackend interface {
	ToolBackend
	CallToolStream(ctx context.Context, name, argsJSON string) (<-chan process.StreamEvent, error)
}
```

- [ ] **Step 2: Add streaming tool call path to handleToolsCall**

After the existing `CallTool` call and the raw passthrough block, add a streaming path. The logic is:

1. If client is stream-capable AND backend supports streaming AND result is large → use streaming
2. Otherwise → use the existing raw passthrough path

Since we don't know the result size before calling, we need to either:
- Always use `CallToolStream` when streaming is available (it handles non-chunked responses too via `StreamEvent.Result`)
- Or call `CallTool` first and check size

The cleaner approach: when streaming is enabled, always use `CallToolStream`. If the tool returns a small result (below threshold), it comes as a single `StreamEvent.Result` and we return it normally.

```go
	// Streaming path: use CallToolStream if available.
	if h.streamCapable && h.streamWriter != nil {
		if sb, ok := h.backend.(StreamingBackend); ok {
			return h.handleToolsCallStreaming(ctx, req, sb, params)
		}
	}
```

Add the streaming handler method:

```go
func (h *Handler) handleToolsCallStreaming(ctx context.Context, req JSONRPCRequest, sb StreamingBackend, params ToolsCallParams) (*JSONRPCResponse, error) {
	argsJSON := "{}"
	if params.Arguments != nil {
		argsJSON = string(params.Arguments)
	}

	ch, err := sb.CallToolStream(ctx, params.Name, argsJSON)
	if err != nil {
		return h.internalError(req.ID, err.Error())
	}

	streamID := fmt.Sprintf("s-%d", atomic.AddUint64(&h.streamIDSeq, 1))
	var streaming bool

	for evt := range ch {
		// Non-chunked response — return normally via raw passthrough.
		if evt.Result != nil {
			resultJSON := evt.Result.ResultJson
			trimmed := strings.TrimSpace(resultJSON)
			if len(trimmed) > 0 && trimmed[0] == '[' {
				result := RawToolsCallResult{
					Content: json.RawMessage(resultJSON),
					IsError: evt.Result.IsError,
				}
				if evt.Result.StructuredContentJson != "" {
					result.StructuredContent = json.RawMessage(evt.Result.StructuredContentJson)
				}
				return h.success(req.ID, result)
			}
			var content []ContentItem
			if resultJSON != "" {
				if unmarshalErr := json.Unmarshal([]byte(resultJSON), &content); unmarshalErr != nil {
					content = []ContentItem{{Type: "text", Text: resultJSON}}
				}
			}
			return h.success(req.ID, ToolsCallResult{Content: content, IsError: evt.Result.IsError})
		}

		// Stream header — send start notification.
		if evt.Header != nil {
			streaming = true
			h.streamWriter.WriteNotification("x-protomcp-stream/start", StreamStartParams{
				ID:        req.ID,
				StreamID:  streamID,
				TotalSize: int(evt.Header.TotalSize),
			})
			h.streamWriter.Flush()
			continue
		}

		// Stream chunk — forward directly to transport.
		if evt.Chunk != nil && streaming {
			h.streamWriter.WriteNotification("x-protomcp-stream/chunk", StreamChunkParams{
				StreamID: streamID,
				Data:     string(evt.Chunk),
			})
			h.streamWriter.Flush()
		}
	}

	// Stream complete — send final response.
	if streaming {
		return h.success(req.ID, StreamCompleteResult{
			XStreamComplete: streamID,
		})
	}

	// Channel closed with no events — error.
	return h.internalError(req.ID, "tool stream closed without response")
}
```

Add `"sync/atomic"` to imports.

- [ ] **Step 3: Write test with mock streaming backend**

```go
type mockStreamingBackend struct {
	mockToolBackend
	streamEvents []process.StreamEvent
}

func (m *mockStreamingBackend) CallToolStream(ctx context.Context, name, argsJSON string) (<-chan process.StreamEvent, error) {
	ch := make(chan process.StreamEvent, len(m.streamEvents))
	for _, evt := range m.streamEvents {
		ch <- evt
	}
	close(ch)
	return ch, nil
}

func TestHandleToolsCall_StreamingPipeline(t *testing.T) {
	payload := `[{"type":"text","text":"` + strings.Repeat("X", 200000) + `"}]`
	chunkSize := 65536

	// Build stream events
	var events []process.StreamEvent
	events = append(events, process.StreamEvent{
		Header: &pb.StreamHeader{FieldName: "result_json", TotalSize: uint64(len(payload)), ChunkSize: uint32(chunkSize)},
	})
	data := []byte(payload)
	for offset := 0; offset < len(data); {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		events = append(events, process.StreamEvent{
			Chunk: data[offset:end],
			Final: end >= len(data),
		})
		offset = end
	}

	backend := &mockStreamingBackend{
		mockToolBackend: mockToolBackend{
			tools: []*pb.ToolDefinition{{Name: "generate", InputSchemaJson: `{"type":"object"}`}},
		},
		streamEvents: events,
	}

	h := mcp.NewHandler(backend)
	sw := &mockStreamWriter{}
	h.SetStreamWriter(sw)

	// Initialize with streaming
	initParams, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{"x-protomcp-stream": map[string]int{"maxChunkSize": chunkSize}},
		"clientInfo":      map[string]string{"name": "test"},
	})
	h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`0`), Method: "initialize", Params: initParams,
	})

	// Call tool
	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "generate"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify start + chunk notifications
	if len(sw.notifications) < 2 {
		t.Fatalf("expected >= 2 notifications, got %d", len(sw.notifications))
	}
	if sw.notifications[0].method != "x-protomcp-stream/start" {
		t.Errorf("first notification = %s, want x-protomcp-stream/start", sw.notifications[0].method)
	}

	// Reassemble chunks
	var assembled []byte
	for _, n := range sw.notifications {
		if n.method == "x-protomcp-stream/chunk" {
			var cp mcp.StreamChunkParams
			json.Unmarshal(n.params, &cp)
			assembled = append(assembled, []byte(cp.Data)...)
		}
	}
	if string(assembled) != payload {
		t.Errorf("assembled length = %d, want %d", len(assembled), len(payload))
	}

	// Final response should have x-stream-complete
	var finalResult mcp.StreamCompleteResult
	json.Unmarshal(resp.Result, &finalResult)
	if finalResult.XStreamComplete == "" {
		t.Error("missing x-stream-complete in final response")
	}
}

func TestHandleToolsCall_StreamingBackend_SmallPayload(t *testing.T) {
	// When streaming backend returns a small (non-chunked) result,
	// handler should return it normally via raw passthrough.
	backend := &mockStreamingBackend{
		mockToolBackend: mockToolBackend{
			tools: []*pb.ToolDefinition{{Name: "echo", InputSchemaJson: `{"type":"object"}`}},
		},
		streamEvents: []process.StreamEvent{
			{Result: &pb.CallToolResponse{ResultJson: `[{"type":"text","text":"hello"}]`}},
		},
	}

	h := mcp.NewHandler(backend)
	sw := &mockStreamWriter{}
	h.SetStreamWriter(sw)

	// Initialize with streaming
	initParams, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{"x-protomcp-stream": map[string]int{"maxChunkSize": 65536}},
		"clientInfo":      map[string]string{"name": "test"},
	})
	h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`0`), Method: "initialize", Params: initParams,
	})

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "echo"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params,
	})
	if err != nil {
		t.Fatal(err)
	}

	// No streaming notifications — small payload goes through normal path.
	if len(sw.notifications) != 0 {
		t.Errorf("expected 0 notifications for small payload, got %d", len(sw.notifications))
	}

	var result struct {
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)
	if string(result.Content) != `[{"type":"text","text":"hello"}]` {
		t.Errorf("unexpected content: %s", result.Content)
	}
}
```

- [ ] **Step 4: Add mockStreamWriter if not already in test file**

```go
type mockStreamWriter struct {
	notifications []struct {
		method string
		params json.RawMessage
	}
}

func (m *mockStreamWriter) WriteNotification(method string, params interface{}) error {
	p, _ := json.Marshal(params)
	m.notifications = append(m.notifications, struct {
		method string
		params json.RawMessage
	}{method: method, params: p})
	return nil
}

func (m *mockStreamWriter) WriteResponse(resp *mcp.JSONRPCResponse) error { return nil }
func (m *mockStreamWriter) Flush() error                                  { return nil }
```

- [ ] **Step 5: Run tests**

Run: `go test -v ./internal/mcp/ -run TestHandleToolsCall_Streaming`
Expected: All pass

Run: `go test -v ./internal/mcp/`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/handler.go internal/mcp/handler_test.go
git commit -m "feat: streaming tool call pipeline with true chunk forwarding

When client supports x-protomcp-stream and backend supports streaming,
chunks flow from tool process through readLoop to handler to transport
without full reassembly. Bounded memory regardless of payload size."
```

---

### Task 14: Wire StreamWriter into transports

**Files:**
- Modify: `internal/transport/http.go:62-109`
- Modify: `internal/transport/stdio.go:35-87`
- Modify: `internal/transport/transport.go`

The transports need to provide their `StreamWriter` to the handler before calling `Handle()`. Since the `RequestHandler` type is `func(ctx, req) (*resp, error)`, the cleanest approach is to extend the `Transport` interface.

- [ ] **Step 1: Add StreamWriterProvider to transport.go**

```go
// StreamWriterProvider is implemented by transports that support streaming.
type StreamWriterProvider interface {
	// NewStreamWriter creates a StreamWriter for a specific response.
	// For HTTP, this wraps the http.ResponseWriter.
	// For stdio/SSE, this wraps the transport's output.
	NewStreamWriter() mcp.StreamWriter
}
```

- [ ] **Step 2: Update HTTP transport to pass StreamWriter per request**

In `http.go`, the POST handler at line 86 calls `handler(ctx, req)`. Before this call, if the handler supports streaming, set its StreamWriter:

The challenge is that the `RequestHandler` is a function, not a struct — we can't call `SetStreamWriter` on it. The solution: wrap the handler in a closure that sets up the StreamWriter.

Add a `StreamingHandlerWrapper` that the HTTP transport's `Start()` uses:

```go
// streamingHandler wraps a handler to inject a per-request StreamWriter.
type streamingHandler struct {
	handler RequestHandler
	setup   func(w http.ResponseWriter) // called before handler to set StreamWriter
}
```

In the HTTP transport's `Start()`, modify the POST handler:

```go
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// ... existing request parsing ...

		// Set up per-request StreamWriter if handler supports it.
		if t.onRequest != nil {
			t.onRequest(w)
		}

		resp, err := handler(ctx, req)
		// ... existing response handling ...
```

Add a callback field to HTTPTransport:

```go
	onRequest func(w http.ResponseWriter) // set StreamWriter per request
```

Add a setter:

```go
func (t *HTTPTransport) OnRequest(fn func(w http.ResponseWriter)) {
	t.onRequest = fn
}
```

The wiring happens in `cmd/protomcp/main.go` (or wherever the transport and handler are connected):

```go
httpTransport.OnRequest(func(w http.ResponseWriter) {
    handler.SetStreamWriter(newHTTPStreamWriter(w))
})
```

- [ ] **Step 3: Update stdio transport similarly**

For stdio, the `StreamWriter` is the same for all requests (it writes to stdout). Set it once when the transport starts:

In `main.go` wiring:

```go
handler.SetStreamWriter(stdioTransport.NewStreamWriter())
```

- [ ] **Step 4: Update SSE transport**

Same as stdio — single StreamWriter wrapping the SSE broadcast:

```go
handler.SetStreamWriter(sseTransport.NewStreamWriter())
```

- [ ] **Step 5: Verify compilation and tests**

Run: `go build ./... && go test ./internal/...`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/transport/ cmd/
git commit -m "feat: wire StreamWriter into HTTP, stdio, and SSE transports"
```

---

### Task 15: Final benchmark and verification

- [ ] **Step 1: Rebuild**

Run: `make build`

- [ ] **Step 2: Run full D4 benchmark**

Run: `go test -v -timeout 120s ./tests/bench/comparison/ -run TestDeepFastMCPComparison/D4`
Expected: protomcp beats FastMCP at all payload sizes

- [ ] **Step 3: Run full test suite**

Run: `go test ./internal/... && go test -timeout 300s ./tests/...`
Expected: All pass

- [ ] **Step 4: Run full deep comparison**

Run: `go test -v -timeout 600s ./tests/bench/comparison/ -run TestDeepFastMCPComparison`
Expected: All sections pass, no regressions
