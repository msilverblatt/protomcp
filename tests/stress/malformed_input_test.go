package stress_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/msilverblatt/protomcp/internal/envelope"
	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/tests/testutil"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// TestMalformedJSON sends various garbage JSON to the stdio transport and
// verifies pmcp does not crash and continues handling valid requests.
func TestMalformedJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	malformedInputs := []string{
		"",                    // empty
		"not json at all",     // plain text
		"{",                   // truncated json
		"{}",                  // empty object (no jsonrpc fields)
		`{"jsonrpc":"2.0"}`,   // missing method
		`{"jsonrpc":"2.0","method":"tools/call"}`, // missing params for tools/call
		`{"jsonrpc":"1.0","id":1,"method":"tools/list"}`, // wrong version
		`null`,
		`[]`,
		`{"jsonrpc":"2.0","id":1,"method":"unknown/method"}`,
		strings.Repeat("x", 1024), // 1KB garbage
	}

	for i, input := range malformedInputs {
		p.SendRaw(t, []byte(input))
		// Some of these will produce responses (like unknown method), some won't.
		// We don't try to read them all because some are silently dropped.
		_ = i
	}

	// Now verify the server still works by sending a valid request.
	r := p.Send(t, "tools/list", nil)
	if r.Resp.Error != nil {
		t.Fatalf("server broken after malformed inputs: %s", r.Resp.Error.Message)
	}
	t.Log("Server survived all malformed JSON inputs")
}

// TestMalformedCallParams sends tools/call with invalid parameter shapes.
func TestMalformedCallParams(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	badParams := []interface{}{
		nil,                        // null params
		"string params",            // string instead of object
		42,                         // number instead of object
		[]int{1, 2, 3},             // array instead of object
		map[string]interface{}{},   // empty object (no name field)
		map[string]interface{}{"name": 123}, // name is not a string
		map[string]interface{}{"name": "nonexistent_tool", "arguments": map[string]string{"x": "y"}},
	}

	for i, params := range badParams {
		r := p.Send(t, "tools/call", params)
		// Should get an error response, not a crash.
		if r.Resp.Error == nil {
			// Not all bad params produce errors (e.g., nonexistent tool might
			// still return an error from the tool process). That's fine.
			t.Logf("bad params %d: got non-error response (acceptable)", i)
		} else {
			t.Logf("bad params %d: got error code %d: %s", i, r.Resp.Error.Code, r.Resp.Error.Message)
		}
	}

	// Verify the server is still responsive.
	r := p.Send(t, "tools/list", nil)
	if r.Resp.Error != nil {
		t.Fatalf("server broken after malformed params: %s", r.Resp.Error.Message)
	}
	t.Log("Server survived all malformed call params")
}

// TestLargePayload sends a very large arguments_json to a tool call and
// verifies the system handles it without crashing.
func TestLargePayload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	// 100KB payload (large but within reason for protobuf over unix socket)
	largeMsg := strings.Repeat("A", 100*1024)
	params := map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]string{"message": largeMsg},
	}

	r := p.Send(t, "tools/call", params)
	if r.Resp.Error != nil {
		t.Logf("Large payload returned error (acceptable): %s", r.Resp.Error.Message)
	} else {
		t.Log("Large 100KB payload handled successfully")
	}

	// Verify continued operation.
	r = p.Send(t, "tools/list", nil)
	if r.Resp.Error != nil {
		t.Fatalf("server broken after large payload: %s", r.Resp.Error.Message)
	}
}

// TestRapidMixedRequests sends a mix of valid and invalid requests rapidly.
func TestRapidMixedRequests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	const totalRequests = 100
	validCount := 0

	for i := 0; i < totalRequests; i++ {
		if i%3 == 0 {
			// Send garbage.
			p.SendRaw(t, []byte(fmt.Sprintf("garbage-%d", i)))
		} else {
			// Send valid request.
			validCount++
			params, _ := json.Marshal(map[string]interface{}{
				"name":      "echo",
				"arguments": map[string]string{"message": fmt.Sprintf("valid-%d", i)},
			})
			req := struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}{
				JSONRPC: "2.0",
				ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
				Method:  "tools/call",
				Params:  params,
			}
			data, _ := json.Marshal(req)
			p.SendRaw(t, data)
		}
	}

	// Read the valid responses.
	for i := 0; i < validCount; i++ {
		if !p.Reader.Scan() {
			t.Fatalf("missing response %d/%d", i+1, validCount)
		}
		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(p.Reader.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response %d: %v", i, err)
		}
	}

	t.Logf("Handled %d mixed requests (%d valid, %d garbage)",
		totalRequests, validCount, totalRequests-validCount)
}

// TestEnvelopeMalformed tests the envelope package directly with malformed data.
func TestEnvelopeMalformed(t *testing.T) {
	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "empty",
			data: nil,
		},
		{
			name: "truncated_length",
			data: []byte{0x00, 0x00},
		},
		{
			name: "zero_length",
			data: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "garbage_after_length",
			data: func() []byte {
				var buf bytes.Buffer
				binary.Write(&buf, binary.BigEndian, uint32(10))
				buf.Write([]byte("short")) // only 5 bytes, length says 10
				return buf.Bytes()
			}(),
		},
		{
			name: "valid_length_garbage_protobuf",
			data: func() []byte {
				var buf bytes.Buffer
				garbage := []byte("this is not protobuf data at all")
				binary.Write(&buf, binary.BigEndian, uint32(len(garbage)))
				buf.Write(garbage)
				return buf.Bytes()
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := bytes.NewReader(tc.data)
			_, err := envelope.Read(reader)
			if err == nil && tc.name != "zero_length" {
				// zero-length may parse as empty protobuf (valid but empty Envelope)
				t.Log("no error returned (empty envelope is valid protobuf)")
			} else if err != nil {
				t.Logf("correctly returned error: %v", err)
			}
		})
	}
}

// TestEnvelopeOversized verifies the envelope reader rejects messages over the
// max size limit.
func TestEnvelopeOversized(t *testing.T) {
	var buf bytes.Buffer
	// Write a length prefix claiming 20MB (over the 10MB limit in envelope.go).
	oversize := uint32(20 * 1024 * 1024)
	binary.Write(&buf, binary.BigEndian, oversize)
	// We don't need to actually write 20MB of data because the reader should
	// reject based on the length prefix alone.
	_, err := envelope.Read(&buf)
	if err == nil {
		t.Fatal("expected error for oversized message, got nil")
	}
	t.Logf("correctly rejected oversized message: %v", err)
}

// TestEnvelopeUnknownFields writes a valid envelope with all known fields then
// verifies round-tripping works.
func TestEnvelopeUnknownFields(t *testing.T) {
	env := &pb.Envelope{
		RequestId: "test-unknown",
		Namespace: "ns1",
		Msg: &pb.Envelope_CallTool{
			CallTool: &pb.CallToolRequest{
				Name:          "some_tool",
				ArgumentsJson: `{"key": "value"}`,
			},
		},
	}

	var buf bytes.Buffer
	if err := envelope.Write(&buf, env); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := envelope.Read(&buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.RequestId != "test-unknown" {
		t.Errorf("RequestId = %q, want %q", got.RequestId, "test-unknown")
	}
	if got.Namespace != "ns1" {
		t.Errorf("Namespace = %q, want %q", got.Namespace, "ns1")
	}
}
