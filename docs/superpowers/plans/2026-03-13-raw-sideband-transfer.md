# Raw Sideband Transfer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bypass protobuf serialization for large payloads by sending raw bytes directly over the unix socket, eliminating two full copies of the data (protobuf serialize + deserialize).

**Architecture:** A new wire-level transfer mode where the SDK sends a small protobuf `RawHeader` message ("here comes N raw bytes for field X on request Y"), then writes the raw bytes directly to the socket without protobuf wrapping. The Go `readLoop` sees the `RawHeader`, switches to raw read mode for exactly N bytes, then resumes normal protobuf framing. This replaces the current chunked path for large payloads — the data goes from Python's `json.dumps` output through socket I/O directly into Go's `[]byte`, with zero protobuf overhead on the payload itself.

**Tech Stack:** Go (envelope reader, process manager), Protocol Buffers (header message only), Python SDK, TypeScript SDK

**Spec:** This plan extends `docs/superpowers/specs/2026-03-13-chunked-streaming-design.md`

---

## Wire Protocol

Current chunked path for a 5MB `result_json`:
```
SDK:    protobuf(StreamHeader) + N × protobuf(StreamChunk{65KB})  ← ~80 protobuf messages
Go:     N × protobuf deserialize → reassemble → []byte
```

New raw sideband path:
```
SDK:    protobuf(RawHeader{size=5MB, field="result_json", req="req-5"})  ← 1 protobuf message (~40 bytes)
SDK:    raw_write(5MB bytes)                                             ← zero protobuf overhead
Go:     protobuf deserialize RawHeader (~40 bytes)
Go:     io.ReadFull(conn, 5MB)                                          ← zero protobuf overhead
Go:     construct CallToolResponse with result_json = string(buf)
```

The key constraint: the unix socket is a single byte stream shared by all concurrent requests. The `readLoop` is a single goroutine, so after reading a `RawHeader`, it can safely read the raw bytes before processing the next message — no interleaving risk.

## File Structure

| File | Role |
|------|------|
| `proto/protomcp.proto` | Add `RawHeader` message to Envelope oneof |
| `internal/envelope/envelope.go` | Add `ReadRaw` — reads envelope, and if it's a RawHeader, reads the raw bytes too |
| `internal/process/manager.go` | Use `ReadRaw` in readLoop, construct CallToolResponse from raw bytes |
| `internal/process/manager_test.go` | Unit tests for raw sideband reassembly |
| `sdk/python/src/protomcp/transport.py` | Add `send_raw` method |
| `sdk/python/src/protomcp/runner.py` | Use `send_raw` instead of `send_chunked` when above threshold |
| `sdk/typescript/src/transport.ts` | Add `sendRaw` method |
| `sdk/typescript/src/runner.ts` | Use `sendRaw` instead of `sendChunked` when above threshold |
| `tests/bench/comparison/fastmcp_deep_comparison_test.go` | D7 already covers this — just re-run |

---

## Chunk 1: Wire Protocol and Go Side

### Task 1: Add RawHeader to protobuf schema

**Files:**
- Modify: `proto/protomcp.proto`

- [ ] **Step 1: Add RawHeader message and Envelope field**

Add to the bottom of `proto/protomcp.proto` (after `StreamChunk`):

```protobuf
// RawHeader signals that the next N bytes on the socket are raw (not protobuf-wrapped).
// This avoids protobuf serialization overhead for large payloads.
message RawHeader {
  string request_id = 1;   // which request this payload belongs to
  string field_name = 2;   // which field, e.g. "result_json"
  uint64 size = 3;          // exact byte count that follows
}
```

Add to the `Envelope.oneof msg` block (after `stream_chunk = 29`):

```protobuf
    RawHeader raw_header = 30;
```

- [ ] **Step 2: Regenerate protobuf code**

Run: `make proto`

- [ ] **Step 3: Verify compilation**

Run: `go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add proto/protomcp.proto gen/ sdk/python/gen/ sdk/typescript/gen/
git commit -m "proto: add RawHeader message for sideband transfer"
```

---

### Task 2: Add raw-aware envelope reader

**Files:**
- Modify: `internal/envelope/envelope.go`
- Test: `internal/envelope/envelope_test.go`

The key insight: after `envelope.Read` returns a `RawHeader`, the *next bytes on the wire* are raw payload, not another length-prefixed protobuf. The readLoop needs to read exactly `RawHeader.Size` bytes before resuming normal framing.

We add a `ReadRaw` function that wraps `Read` and handles this automatically.

- [ ] **Step 1: Write the test**

Add to `internal/envelope/envelope_test.go`:

```go
func TestReadRaw(t *testing.T) {
	// Simulate: protobuf envelope with RawHeader, followed by raw bytes
	var buf bytes.Buffer

	// Write a RawHeader envelope
	payload := []byte(strings.Repeat("X", 100000))
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId: "req-1",
				FieldName: "result_json",
				Size:      uint64(len(payload)),
			},
		},
	}
	if err := Write(&buf, header); err != nil {
		t.Fatal(err)
	}

	// Write raw bytes directly (no protobuf framing)
	buf.Write(payload)

	// Write a normal envelope after the raw bytes
	normal := &pb.Envelope{
		RequestId: "req-2",
		Msg: &pb.Envelope_CallResult{
			CallResult: &pb.CallToolResponse{ResultJson: `[{"type":"text","text":"ok"}]`},
		},
	}
	if err := Write(&buf, normal); err != nil {
		t.Fatal(err)
	}

	reader := &buf

	// ReadRaw should return the RawHeader envelope + the raw payload
	env, raw, err := ReadRaw(reader)
	if err != nil {
		t.Fatal(err)
	}
	rh := env.GetRawHeader()
	if rh == nil {
		t.Fatal("expected RawHeader")
	}
	if rh.RequestId != "req-1" {
		t.Errorf("request_id = %q, want req-1", rh.RequestId)
	}
	if len(raw) != len(payload) {
		t.Errorf("raw length = %d, want %d", len(raw), len(payload))
	}
	if string(raw) != string(payload) {
		t.Error("raw bytes don't match")
	}

	// Next read should return the normal envelope with no raw bytes
	env2, raw2, err := ReadRaw(reader)
	if err != nil {
		t.Fatal(err)
	}
	if env2.GetCallResult() == nil {
		t.Fatal("expected CallResult")
	}
	if raw2 != nil {
		t.Error("expected nil raw for non-RawHeader envelope")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/envelope/ -run TestReadRaw`
Expected: FAIL — `ReadRaw` doesn't exist yet

- [ ] **Step 3: Implement ReadRaw**

Add to `internal/envelope/envelope.go`:

```go
// ReadRaw reads a length-prefixed Envelope. If the envelope contains a
// RawHeader, it also reads the subsequent raw bytes from the reader.
// Returns (envelope, rawBytes, error). rawBytes is nil for non-RawHeader messages.
func ReadRaw(r io.Reader) (*pb.Envelope, []byte, error) {
	env, err := Read(r)
	if err != nil {
		return nil, nil, err
	}

	rh := env.GetRawHeader()
	if rh == nil {
		return env, nil, nil
	}

	// Read raw bytes that follow the RawHeader
	raw := make([]byte, rh.Size)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, nil, fmt.Errorf("read raw payload (%d bytes): %w", rh.Size, err)
	}

	return env, raw, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/envelope/ -run TestReadRaw`
Expected: PASS

- [ ] **Step 5: Run all envelope tests**

Run: `go test -v ./internal/envelope/`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add internal/envelope/envelope.go internal/envelope/envelope_test.go
git commit -m "feat: ReadRaw for sideband raw byte transfer

Reads a protobuf envelope normally. If it contains a RawHeader,
also reads the subsequent raw bytes from the wire. This avoids
protobuf serialization overhead for large payloads."
```

---

### Task 3: Use ReadRaw in readLoop

**Files:**
- Modify: `internal/process/manager.go`
- Test: `internal/process/manager_test.go`

The readLoop currently calls `envelope.Read(m.conn)`. Switch to `envelope.ReadRaw(m.conn)`. When `raw != nil`, construct a `CallToolResponse` from the raw bytes and dispatch it — same as current stream reassembly final dispatch, but without any assembly step.

- [ ] **Step 1: Write tests**

Add to `internal/process/manager_test.go`:

```go
func TestRawSidebandTransfer(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh := mgr.RegisterPending("req-1")

	payload := `[{"type":"text","text":"` + strings.Repeat("X", 200*1024) + `"}]`

	// Send a RawHeader envelope
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId: "req-1",
				FieldName: "result_json",
				Size:      uint64(len(payload)),
			},
		},
	}
	envelope.Write(toolConn, header)

	// Send raw bytes directly (no protobuf framing)
	toolConn.Write([]byte(payload))

	select {
	case resp := <-respCh:
		result := resp.GetCallResult()
		if result == nil {
			t.Fatal("expected CallToolResponse")
		}
		if result.ResultJson != payload {
			t.Errorf("result length = %d, want %d", len(result.ResultJson), len(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestRawSidebandTransfer_ThenNormalMessage(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh1 := mgr.RegisterPending("req-1")
	respCh2 := mgr.RegisterPending("req-2")

	payload := `[{"type":"text","text":"big data"}]`

	// Send raw sideband for req-1
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId: "req-1",
				FieldName: "result_json",
				Size:      uint64(len(payload)),
			},
		},
	}
	envelope.Write(toolConn, header)
	toolConn.Write([]byte(payload))

	// Send normal protobuf response for req-2
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-2",
		Msg: &pb.Envelope_CallResult{CallResult: &pb.CallToolResponse{
			ResultJson: `[{"type":"text","text":"normal"}]`,
		}},
	})

	// Both should arrive correctly
	select {
	case resp := <-respCh1:
		if resp.GetCallResult().ResultJson != payload {
			t.Error("req-1: wrong result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("req-1 timeout")
	}

	select {
	case resp := <-respCh2:
		if resp.GetCallResult().ResultJson != `[{"type":"text","text":"normal"}]` {
			t.Error("req-2: wrong result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("req-2 timeout")
	}
}

func TestRawSidebandTransfer_StreamChannel(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)

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

	payload := strings.Repeat("Y", 200*1024)

	// Send raw sideband
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId: reqID,
				FieldName: "result_json",
				Size:      uint64(len(payload)),
			},
		},
	}
	envelope.Write(toolConn, header)
	toolConn.Write([]byte(payload))

	// StreamEvent channel should receive the result directly
	var gotResult bool
	for evt := range ch {
		if evt.Result != nil {
			gotResult = true
			if evt.Result.ResultJson != payload {
				t.Errorf("result length = %d, want %d", len(evt.Result.ResultJson), len(payload))
			}
		}
	}
	if !gotResult {
		t.Error("never received result event")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/process/ -run TestRawSideband`
Expected: FAIL — readLoop doesn't handle RawHeader yet

- [ ] **Step 3: Modify readLoop to use ReadRaw**

In `internal/process/manager.go`, replace the `envelope.Read` call in `readLoop` with `envelope.ReadRaw`, and add RawHeader handling:

Replace:
```go
		env, err := envelope.Read(m.conn)
```

With:
```go
		env, rawPayload, err := envelope.ReadRaw(m.conn)
```

Then, right after the `reqID` extraction and unsolicited message routing block (before the stream reassembly block), add:

```go
		// Raw sideband transfer — payload arrived without protobuf wrapping.
		if rawPayload != nil {
			rh := env.GetRawHeader()
			rawReqID := rh.RequestId

			result := &pb.Envelope{
				RequestId: rawReqID,
				Msg: &pb.Envelope_CallResult{
					CallResult: &pb.CallToolResponse{},
				},
			}
			switch rh.FieldName {
			case "result_json":
				result.GetCallResult().ResultJson = string(rawPayload)
			case "structured_content_json":
				result.GetCallResult().StructuredContentJson = string(rawPayload)
			}

			// Check streamChs first, then pending.
			m.mu.Lock()
			sCh, isStream := m.streamChs[rawReqID]
			pendCh, isPending := m.pending[rawReqID]
			m.mu.Unlock()

			if isStream {
				select {
				case sCh <- StreamEvent{Result: result.GetCallResult()}:
				default:
				}
				m.mu.Lock()
				delete(m.streamChs, rawReqID)
				m.mu.Unlock()
				close(sCh)
			} else if isPending {
				select {
				case pendCh <- result:
				default:
				}
			}
			continue
		}
```

Note: the `RawHeader` has its own `request_id` field (not the Envelope's `request_id`), because the Envelope's `request_id` is inside the oneof and `RawHeader` is the oneof variant. So we read `rh.RequestId` from the `RawHeader` message itself.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/process/ -run TestRawSideband`
Expected: All 3 pass

- [ ] **Step 5: Run all process tests**

Run: `go test -v ./internal/process/`
Expected: All pass (existing stream tests still work — they use StreamHeader/StreamChunk, not RawHeader)

- [ ] **Step 6: Commit**

```bash
git add internal/process/manager.go internal/process/manager_test.go
git commit -m "feat: raw sideband transfer in readLoop

When readLoop receives a RawHeader, it reads the subsequent raw bytes
directly from the socket (no protobuf wrapping), constructs a
CallToolResponse, and dispatches it. This eliminates protobuf
serialize/deserialize overhead for large payloads."
```

---

## Chunk 2: SDK Implementations and Benchmark

### Task 4: Add send_raw to Python SDK

**Files:**
- Modify: `sdk/python/src/protomcp/transport.py`
- Modify: `sdk/python/src/protomcp/runner.py`

- [ ] **Step 1: Add `send_raw` to transport.py**

Add after the existing `send_chunked` method in `sdk/python/src/protomcp/transport.py`:

```python
    def send_raw(self, request_id: str, field_name: str, data: bytes):
        """Send a large field as a RawHeader + raw bytes (no protobuf wrapping on payload)."""
        header = pb.Envelope(
            raw_header=pb.RawHeader(
                request_id=request_id,
                field_name=field_name,
                size=len(data),
            ),
        )
        # Send the protobuf header normally
        header_bytes = header.SerializeToString()
        length = struct.pack(">I", len(header_bytes))
        # Send header + raw payload in one sendall to minimize syscalls
        self._sock.sendall(length + header_bytes + data)
```

- [ ] **Step 2: Switch runner.py to use `send_raw` instead of `send_chunked`**

In `sdk/python/src/protomcp/runner.py`, in `_handle_call_tool`, replace the chunk threshold block:

Replace:
```python
    if len(result_json_bytes) > chunk_threshold:
        transport.send_chunked(
            request_id=env.request_id,
            field_name='result_json',
            data=result_json_bytes,
        )
    else:
```

With:
```python
    if len(result_json_bytes) > chunk_threshold:
        transport.send_raw(
            request_id=env.request_id,
            field_name='result_json',
            data=result_json_bytes,
        )
    else:
```

- [ ] **Step 3: Verify the existing integration test still passes**

Run: `go test -v ./internal/process/ -run TestChunkedStreamIntegration`
Expected: PASS — the integration test uses `simple_tool.py` which goes through the runner, and now uses `send_raw` instead of `send_chunked`.

- [ ] **Step 4: Commit**

```bash
git add sdk/python/src/protomcp/transport.py sdk/python/src/protomcp/runner.py
git commit -m "feat: raw sideband transfer in Python SDK

Replaces send_chunked with send_raw for large payloads. The raw
payload bytes are written directly to the socket after a small
RawHeader protobuf envelope, eliminating protobuf serialization
overhead on the payload."
```

---

### Task 5: Add sendRaw to TypeScript SDK

**Files:**
- Modify: `sdk/typescript/src/transport.ts`
- Modify: `sdk/typescript/src/runner.ts`

- [ ] **Step 1: Add `sendRaw` to transport.ts**

Add after the existing `sendChunked` method:

```typescript
  async sendRaw(requestId: string, fieldName: string, data: Buffer): Promise<void> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');
    const RawHeader = root.lookupType('protomcp.RawHeader');

    const header = Envelope.create({
      rawHeader: RawHeader.create({
        requestId,
        fieldName,
        size: data.length,
      }),
    });

    // Encode the protobuf header
    const headerBytes = Buffer.from(Envelope.encode(header).finish());
    const lengthBuf = Buffer.alloc(4);
    lengthBuf.writeUInt32BE(headerBytes.length, 0);

    // Write header frame + raw payload in one call
    const frame = Buffer.concat([lengthBuf, headerBytes, data]);
    await new Promise<void>((resolve, reject) => {
      this.socket!.write(frame, (err) => {
        if (err) reject(err);
        else resolve();
      });
    });
  }
```

- [ ] **Step 2: Switch runner.ts to use `sendRaw`**

In `sdk/typescript/src/runner.ts`, find the chunk threshold block and replace `sendChunked` with `sendRaw`:

Replace any call to:
```typescript
await transport.sendChunked(requestId, 'result_json', Buffer.from(resultJson));
```

With:
```typescript
await transport.sendRaw(requestId, 'result_json', Buffer.from(resultJson));
```

- [ ] **Step 3: Verify compilation**

Run: `npx tsc --noEmit` (from `sdk/typescript/`)
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add sdk/typescript/src/transport.ts sdk/typescript/src/runner.ts
git commit -m "feat: raw sideband transfer in TypeScript SDK"
```

---

### Task 6: Rebuild and run D7 benchmark

**Files:**
- No changes — D7 benchmark already uses the SDK fixture which goes through the runner.

- [ ] **Step 1: Rebuild**

Run: `make build`

- [ ] **Step 2: Run D7 benchmark**

Run: `go test -v -timeout 300s ./tests/bench/comparison/ -run TestDeepFastMCPComparison/D7`

Expected: Significant improvement over the chunked results. The "chunked p50" column should drop because raw sideband eliminates ~80 protobuf serialize/deserialize cycles per 5MB transfer.

Target improvements vs chunked (previous results):
- 100KB: was 1.78ms → target <1.2ms
- 500KB: was 7.93ms → target <5ms
- 1MB: was 16.6ms → target <10ms
- 5MB: was 87.2ms → target <55ms

- [ ] **Step 3: Run all tests to verify no regressions**

Run: `go test ./internal/... && go test -timeout 300s ./tests/bench/comparison/ -run TestDeepFastMCPComparison`
Expected: All pass

- [ ] **Step 4: Commit benchmark results**

No code changes needed — results are in test output.

---

### Task 7: Run full test suite

- [ ] **Step 1: Run all internal tests**

Run: `go test ./internal/...`
Expected: All pass

- [ ] **Step 2: Run all benchmark tests**

Run: `go test -v -timeout 600s ./tests/bench/comparison/ -run TestDeepFastMCPComparison`
Expected: All sections (D1-D7) pass, no regressions

- [ ] **Step 3: Run process manager tests**

Run: `go test -v ./internal/process/`
Expected: All pass — both old stream tests and new raw sideband tests
