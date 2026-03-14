package process_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/envelope"
	"github.com/msilverblatt/protomcp/internal/process"
)

func TestStartAndHandshake(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	if len(tools) == 0 {
		t.Fatal("expected at least one tool from handshake")
	}
	if tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", tools[0].Name)
	}
}

func TestCallTool(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	resp, err := pm.CallTool(ctx, "echo", `{"message": "hello"}`)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %s", resp.ResultJson)
	}
}

func TestReload(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	tools, err := pm.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected tools after reload")
	}
}

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

	fullPayload := strings.Repeat("A", 200*1024)
	fullResultJSON := fmt.Sprintf(`[{"type":"text","text":"%s"}]`, fullPayload)
	chunkSize := 64 * 1024

	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{StreamHeader: &pb.StreamHeader{
			FieldName: "result_json", TotalSize: uint64(len(fullResultJSON)), ChunkSize: uint32(chunkSize),
		}},
	})

	remaining := []byte(fullResultJSON)
	for len(remaining) > 0 {
		sz := chunkSize
		if sz > len(remaining) {
			sz = len(remaining)
		}
		envelope.Write(toolConn, &pb.Envelope{
			RequestId: "req-1",
			Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{
				Data: remaining[:sz], Final: sz >= len(remaining),
			}},
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

	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{StreamHeader: &pb.StreamHeader{
			FieldName: "result_json", TotalSize: 0, ChunkSize: 1024,
		}},
	})
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload), Final: true}},
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

	// Send chunk with no matching header — should be discarded.
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-orphan",
		Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte("hello"), Final: true}},
	})

	// Send normal response to verify readLoop still works.
	respCh := mgr.RegisterPending("req-2")
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-2",
		Msg: &pb.Envelope_CallResult{CallResult: &pb.CallToolResponse{ResultJson: `[{"type":"text","text":"ok"}]`}},
	})

	select {
	case resp := <-respCh:
		if resp.GetCallResult().ResultJson != `[{"type":"text","text":"ok"}]` {
			t.Error("unexpected result")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout — readLoop may have crashed")
	}
}

func TestStreamReassembly_InterleavedStreams(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh1 := mgr.RegisterPending("req-1")
	respCh2 := mgr.RegisterPending("req-2")

	payload1 := `[{"type":"text","text":"AAAA"}]`
	payload2 := `[{"type":"text","text":"BBBB"}]`

	for _, id := range []string{"req-1", "req-2"} {
		envelope.Write(toolConn, &pb.Envelope{
			RequestId: id,
			Msg: &pb.Envelope_StreamHeader{StreamHeader: &pb.StreamHeader{FieldName: "result_json", ChunkSize: 1024}},
		})
	}

	// Interleave chunks
	envelope.Write(toolConn, &pb.Envelope{RequestId: "req-1", Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload1[:15])}}})
	envelope.Write(toolConn, &pb.Envelope{RequestId: "req-2", Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload2[:15])}}})
	envelope.Write(toolConn, &pb.Envelope{RequestId: "req-1", Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload1[15:]), Final: true}}})
	envelope.Write(toolConn, &pb.Envelope{RequestId: "req-2", Msg: &pb.Envelope_StreamChunk{StreamChunk: &pb.StreamChunk{Data: []byte(payload2[15:]), Final: true}}})

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

func TestCallToolStream(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)

	payload := `[{"type":"text","text":"` + strings.Repeat("X", 200*1024) + `"}]`
	chunkSize := 64 * 1024

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

func TestCallToolStream_NonChunked(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)

	ctx := context.Background()
	ch, err := mgr.CallToolStream(ctx, "echo", `{"message":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}

	// Read the CallToolRequest from the tool side
	toolEnv, err := envelope.Read(toolConn)
	if err != nil {
		t.Fatal(err)
	}
	reqID := toolEnv.GetRequestId()

	// Send a normal (non-chunked) response
	envelope.Write(toolConn, &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_CallResult{CallResult: &pb.CallToolResponse{
			ResultJson: `[{"type":"text","text":"hi"}]`,
		}},
	})

	// Channel should receive a single Result event
	var gotResult bool
	for evt := range ch {
		if evt.Result != nil {
			gotResult = true
			if evt.Result.ResultJson != `[{"type":"text","text":"hi"}]` {
				t.Errorf("unexpected result: %s", evt.Result.ResultJson)
			}
		}
	}
	if !gotResult {
		t.Error("never received result event")
	}
}

func TestStreamReassembly_CrashMidStream(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pmcp-crash-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
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

	envelope.Write(toolConn, &pb.Envelope{
		RequestId: "req-1",
		Msg: &pb.Envelope_StreamHeader{StreamHeader: &pb.StreamHeader{FieldName: "result_json", TotalSize: 100000, ChunkSize: 1024}},
	})
	toolConn.Close()

	select {
	case <-done: // readLoop exited
	case <-time.After(5 * time.Second):
		t.Fatal("readLoop didn't exit after tool crash")
	}

	select {
	case resp := <-respCh:
		t.Fatalf("unexpected response after crash: %v", resp)
	default: // expected — no response
	}
}

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

func TestRawSidebandTransfer_Compressed(t *testing.T) {
	mgr, toolConn := testStreamSetup(t)
	respCh := mgr.RegisterPending("req-zstd")

	original := `[{"type":"text","text":"` + strings.Repeat("COMPRESSED_DATA_", 20000) + `"}]`

	// Compress with zstd
	zstdEncoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := zstdEncoder.EncodeAll([]byte(original), nil)
	zstdEncoder.Close()

	// Send a RawHeader with compression metadata
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId:        "req-zstd",
				FieldName:        "result_json",
				Size:             uint64(len(compressed)),
				Compression:      "zstd",
				UncompressedSize: uint64(len(original)),
			},
		},
	}
	envelope.Write(toolConn, header)

	// Send compressed bytes
	toolConn.Write(compressed)

	select {
	case resp := <-respCh:
		result := resp.GetCallResult()
		if result == nil {
			t.Fatal("expected CallToolResponse")
		}
		if result.ResultJson != original {
			t.Errorf("decompressed result length = %d, want %d", len(result.ResultJson), len(original))
		}
		t.Logf("compressed %d -> %d bytes (%.1fx)", len(original), len(compressed), float64(len(original))/float64(len(compressed)))
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
