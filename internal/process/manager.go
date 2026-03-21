package process

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/envelope"
)

// streamAssembly tracks an in-progress chunked transfer.
type streamAssembly struct {
	fieldName string
	buf       bytes.Buffer
	totalSize uint64
	created   time.Time
}

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

// RegisteredMiddleware represents a middleware registered by the tool process during handshake.
type RegisteredMiddleware struct {
	Name     string
	Priority int32
}

// ManagerConfig configures how the process manager spawns and communicates
// with a tool process.
type ManagerConfig struct {
	File        string
	RuntimeCmd  string
	RuntimeArgs []string
	SocketPath  string
	MaxRetries  int
	CallTimeout time.Duration
}

// Manager spawns a tool process, communicates via protobuf over a unix socket,
// and handles handshake, tool calls, reload, and crash detection.
type Manager struct {
	cfg      ManagerConfig
	cmd      *exec.Cmd
	conn     net.Conn
	listener net.Listener
	mu       sync.Mutex
	writeMu  sync.Mutex // protects concurrent writes to conn
	pending   map[string]chan *pb.Envelope
	streams   map[string]*streamAssembly
	streamChs map[string]chan StreamEvent
	tools       []*pb.ToolDefinition
	middlewares []RegisteredMiddleware
	crashCh     chan error
	stopCh   chan struct{}
	readWg   sync.WaitGroup
	nextID   int

	// handshakeCh receives unsolicited ToolListResponse messages (no request_id).
	handshakeCh chan *pb.Envelope

	// Callbacks for unsolicited messages from the tool process
	onProgress     func(*pb.ProgressNotification)
	onLog          func(*pb.LogMessage)
	onEnableTools  func([]string)
	onDisableTools func([]string)

	// Callbacks for reverse requests from the SDK process (SDK → Go)
	onSampling  func(*pb.SamplingRequest, string) // receives sampling request + request_id
	onListRoots func(string)                      // receives request_id
}

// NewManager creates a new process manager with the given configuration.
func NewManager(cfg ManagerConfig) *Manager {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 5 * time.Minute
	}
	return &Manager{
		cfg:         cfg,
		pending:     make(map[string]chan *pb.Envelope),
		streams:     make(map[string]*streamAssembly),
		streamChs:   make(map[string]chan StreamEvent),
		crashCh:     make(chan error, 1),
		stopCh:      make(chan struct{}),
		handshakeCh: make(chan *pb.Envelope, 4),
	}
}

// Start spawns the child process, performs the handshake (ListToolsRequest),
// and returns the list of tools the process provides.
func (m *Manager) Start(ctx context.Context) ([]*pb.ToolDefinition, error) {
	// Ensure socket directory exists.
	if err := os.MkdirAll(filepath.Dir(m.cfg.SocketPath), 0o755); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	// Remove stale socket file if it exists.
	os.Remove(m.cfg.SocketPath)

	// Create unix socket listener.
	var err error
	m.listener, err = net.Listen("unix", m.cfg.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on socket: %w", err)
	}

	// Spawn child process.
	runtimeCmd := m.cfg.RuntimeCmd
	runtimeArgs := m.cfg.RuntimeArgs
	if runtimeCmd == "" {
		runtimeCmd = "python3"
		runtimeArgs = []string{m.cfg.File}
	}

	m.cmd = exec.CommandContext(ctx, runtimeCmd, runtimeArgs...)
	m.cmd.Env = append(os.Environ(), fmt.Sprintf("PROTOMCP_SOCKET=%s", m.cfg.SocketPath))
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		m.listener.Close()
		return nil, fmt.Errorf("start process: %w", err)
	}

	// Monitor for crashes.
	go func() {
		err := m.cmd.Wait()
		select {
		case <-m.stopCh:
			// Intentional stop, don't signal crash.
		default:
			if err != nil {
				select {
				case m.crashCh <- err:
				default:
				}
			}
		}
	}()

	// Accept connection from child.
	acceptCh := make(chan net.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := m.listener.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		acceptCh <- conn
	}()

	select {
	case <-ctx.Done():
		m.cleanup()
		return nil, ctx.Err()
	case err := <-acceptErrCh:
		m.cleanup()
		return nil, fmt.Errorf("accept connection: %w", err)
	case conn := <-acceptCh:
		// Increase socket buffers to handle concurrent protobuf messages
		// without deadlocking when both sides saturate small default buffers.
		if uc, ok := conn.(*net.UnixConn); ok {
			uc.SetReadBuffer(1024 * 1024)
			uc.SetWriteBuffer(1024 * 1024)
		}
		m.conn = conn
	}

	// Start background reader.
	m.readWg.Add(1)
	go m.readLoop()

	// Perform handshake: send ListToolsRequest and wait for ToolListResponse.
	tools, err := m.listTools(ctx)
	if err != nil {
		m.cleanup()
		return nil, fmt.Errorf("handshake: %w", err)
	}

	m.mu.Lock()
	m.tools = tools
	m.mu.Unlock()

	// Wait for optional middleware registrations + handshake-complete signal.
	middlewares, err := m.awaitHandshakeComplete(ctx)
	if err != nil {
		m.cleanup()
		return nil, fmt.Errorf("handshake middleware: %w", err)
	}
	m.mu.Lock()
	m.middlewares = middlewares
	m.mu.Unlock()

	return tools, nil
}

// Stop kills the child process and cleans up resources.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.cleanup()
}

// CallTool sends a CallToolRequest and waits for the matching CallToolResponse.
func (m *Manager) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
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

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write CallToolRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("call tool %q timed out after %v", name, timeout)
	case resp := <-respCh:
		result := resp.GetCallResult()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for CallTool")
		}
		return result, nil
	}
}

// ListResources sends a ListResourcesRequest and waits for the matching ResourceListResponse.
func (m *Manager) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ListResourcesRequest{
			ListResourcesRequest: &pb.ListResourcesRequest{},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ListResourcesRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("list resources timed out after %v", timeout)
	case resp := <-respCh:
		result := resp.GetResourceListResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for ListResources")
		}
		return result.Resources, nil
	}
}

// ListResourceTemplates sends a ListResourceTemplatesRequest and waits for the matching ResourceTemplateListResponse.
func (m *Manager) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ListResourceTemplatesRequest{
			ListResourceTemplatesRequest: &pb.ListResourceTemplatesRequest{},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ListResourceTemplatesRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("list resource templates timed out after %v", timeout)
	case resp := <-respCh:
		result := resp.GetResourceTemplateListResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for ListResourceTemplates")
		}
		return result.Templates, nil
	}
}

// ReadResource sends a ReadResourceRequest and waits for the matching ReadResourceResponse.
func (m *Manager) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ReadResourceRequest{
			ReadResourceRequest: &pb.ReadResourceRequest{
				Uri: uri,
			},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ReadResourceRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("read resource %q timed out after %v", uri, timeout)
	case resp := <-respCh:
		result := resp.GetReadResourceResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for ReadResource")
		}
		return result, nil
	}
}

// ListPrompts sends a ListPromptsRequest and waits for the matching PromptListResponse.
func (m *Manager) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ListPromptsRequest{
			ListPromptsRequest: &pb.ListPromptsRequest{},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ListPromptsRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("list prompts timed out after %v", timeout)
	case resp := <-respCh:
		result := resp.GetPromptListResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for ListPrompts")
		}
		return result.Prompts, nil
	}
}

// GetPrompt sends a GetPromptRequest and waits for the matching GetPromptResponse.
func (m *Manager) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_GetPromptRequest{
			GetPromptRequest: &pb.GetPromptRequest{
				Name:          name,
				ArgumentsJson: argsJSON,
			},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write GetPromptRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("get prompt %q timed out after %v", name, timeout)
	case resp := <-respCh:
		result := resp.GetGetPromptResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for GetPrompt")
		}
		return result, nil
	}
}

// Complete sends a CompletionRequest and waits for the matching CompletionResponse.
func (m *Manager) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_CompletionRequest{
			CompletionRequest: &pb.CompletionRequest{
				RefType:       refType,
				RefName:       refName,
				ArgumentName:  argName,
				ArgumentValue: argValue,
			},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write CompletionRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("complete timed out after %v", timeout)
	case resp := <-respCh:
		result := resp.GetCompletionResponse()
		if result == nil {
			return nil, fmt.Errorf("unexpected response type for Complete")
		}
		return result, nil
	}
}

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

// Reload sends a ReloadRequest, waits for ReloadResponse, then receives the
// updated ToolListResponse.
func (m *Manager) Reload(ctx context.Context) ([]*pb.ToolDefinition, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_Reload{
			Reload: &pb.ReloadRequest{},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ReloadRequest: %w", err)
	}

	// Wait for ReloadResponse (matched by request_id).
	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("reload timed out after %v", timeout)
	case resp := <-respCh:
		reloadResp := resp.GetReloadResponse()
		if reloadResp == nil {
			return nil, fmt.Errorf("unexpected response type for Reload")
		}
		if !reloadResp.Success {
			return nil, fmt.Errorf("reload failed: %s", reloadResp.Error)
		}
	}

	// Wait for the ToolListResponse. The Python SDK sends this with the same
	// request_id, so it arrives on respCh. It may also arrive on handshakeCh
	// if sent without a request_id.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("waiting for tool list after reload timed out")
	case toolEnv := <-respCh:
		toolList := toolEnv.GetToolList()
		if toolList == nil {
			return nil, fmt.Errorf("unexpected response type for tool list after reload")
		}
		m.mu.Lock()
		m.tools = toolList.Tools
		m.mu.Unlock()
		// Drain the handshake-complete signal if present.
		select {
		case <-m.handshakeCh:
		case <-time.After(2 * time.Second):
		}
		return toolList.Tools, nil
	case toolEnv := <-m.handshakeCh:
		toolList := toolEnv.GetToolList()
		if toolList == nil {
			return nil, fmt.Errorf("unexpected message type after reload")
		}
		m.mu.Lock()
		m.tools = toolList.Tools
		m.mu.Unlock()
		return toolList.Tools, nil
	}
}

// OnCrash returns a channel that receives an error when the child process
// exits unexpectedly.
func (m *Manager) OnCrash() <-chan error {
	return m.crashCh
}

// Tools returns the current list of tool definitions.
func (m *Manager) Tools() []*pb.ToolDefinition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tools
}

// Middlewares returns the list of middleware registered during handshake.
func (m *Manager) Middlewares() []RegisteredMiddleware {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.middlewares
}

// OnProgress sets a callback for progress notifications from the tool process.
func (m *Manager) OnProgress(fn func(*pb.ProgressNotification)) { m.onProgress = fn }

// OnLog sets a callback for log messages from the tool process.
func (m *Manager) OnLog(fn func(*pb.LogMessage)) { m.onLog = fn }

// OnEnableTools sets a callback for enable-tools requests from the tool process.
func (m *Manager) OnEnableTools(fn func([]string)) { m.onEnableTools = fn }

// OnDisableTools sets a callback for disable-tools requests from the tool process.
func (m *Manager) OnDisableTools(fn func([]string)) { m.onDisableTools = fn }

// OnSampling sets a callback for sampling requests from the SDK process.
func (m *Manager) OnSampling(fn func(*pb.SamplingRequest, string)) { m.onSampling = fn }

// OnListRoots sets a callback for list-roots requests from the SDK process.
func (m *Manager) OnListRoots(fn func(string)) { m.onListRoots = fn }

// SendSamplingResponse sends a SamplingResponse back to the SDK process.
func (m *Manager) SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return envelope.Write(m.conn, &pb.Envelope{
		RequestId: reqID,
		Msg:       &pb.Envelope_SamplingResponse{SamplingResponse: resp},
	})
}

// SendListRootsResponse sends a ListRootsResponse back to the SDK process.
func (m *Manager) SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	return envelope.Write(m.conn, &pb.Envelope{
		RequestId: reqID,
		Msg:       &pb.Envelope_ListRootsResponse{ListRootsResponse: resp},
	})
}

func (m *Manager) awaitHandshakeComplete(ctx context.Context) ([]RegisteredMiddleware, error) {
	var middlewares []RegisteredMiddleware
	// Short timeout for handshake-complete signal. If v1.0 SDKs don't send it,
	// we treat "no message within 500ms" as handshake-complete (backward compat).
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return middlewares, nil
		case env := <-m.handshakeCh:
			if rr := env.GetReloadResponse(); rr != nil {
				return middlewares, nil
			}
			if rm := env.GetRegisterMiddleware(); rm != nil {
				middlewares = append(middlewares, RegisteredMiddleware{
					Name:     rm.Name,
					Priority: rm.Priority,
				})
				resp := &pb.Envelope{
					Msg: &pb.Envelope_RegisterMiddlewareResponse{
						RegisterMiddlewareResponse: &pb.RegisterMiddlewareResponse{Success: true},
					},
				}
				m.writeMu.Lock()
				writeErr := envelope.Write(m.conn, resp)
				m.writeMu.Unlock()
				if writeErr != nil {
					return nil, fmt.Errorf("write RegisterMiddlewareResponse: %w", writeErr)
				}
				timer.Reset(500 * time.Millisecond)
				continue
			}
		}
	}
}

// SendMiddlewareIntercept sends a middleware intercept request and waits for the response.
func (m *Manager) SendMiddlewareIntercept(ctx context.Context, mwName, phase, toolName, argsJSON, resultJSON string, isError bool) (*pb.MiddlewareInterceptResponse, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_MiddlewareIntercept{
			MiddlewareIntercept: &pb.MiddlewareInterceptRequest{
				MiddlewareName: mwName,
				Phase:          phase,
				ToolName:       toolName,
				ArgumentsJson:  argsJSON,
				ResultJson:     resultJSON,
				IsError:        isError,
			},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write MiddlewareInterceptRequest: %w", err)
	}

	timer := time.NewTimer(m.cfg.CallTimeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("middleware intercept %q timed out", mwName)
	case resp := <-respCh:
		mir := resp.GetMiddlewareInterceptResponse()
		if mir == nil {
			return nil, fmt.Errorf("unexpected response type for MiddlewareIntercept")
		}
		return mir, nil
	}
}

// NewManagerForTest creates a Manager with a pre-established connection.
func NewManagerForTest(cfg ManagerConfig, conn net.Conn) *Manager {
	m := NewManager(cfg)
	m.conn = conn
	return m
}

// RegisterPending registers a pending request channel and returns it.
func (m *Manager) RegisterPending(reqID string) chan *pb.Envelope {
	ch := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = ch
	m.mu.Unlock()
	return ch
}

// StartReadLoop starts the readLoop (blocking).
func (m *Manager) StartReadLoop() {
	m.readWg.Add(1)
	m.readLoop()
}

func (m *Manager) nextRequestID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := fmt.Sprintf("req-%d", m.nextID)
	m.nextID++
	return id
}

func (m *Manager) listTools(ctx context.Context) ([]*pb.ToolDefinition, error) {
	reqID := m.nextRequestID()

	env := &pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ListTools{
			ListTools: &pb.ListToolsRequest{},
		},
	}

	respCh := make(chan *pb.Envelope, 1)
	m.mu.Lock()
	m.pending[reqID] = respCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.pending, reqID)
		m.mu.Unlock()
	}()

	m.writeMu.Lock()
	err := envelope.Write(m.conn, env)
	m.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write ListToolsRequest: %w", err)
	}

	timeout := m.cfg.CallTimeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("list tools timed out after %v", timeout)
	case resp := <-respCh:
		toolList := resp.GetToolList()
		if toolList == nil {
			return nil, fmt.Errorf("unexpected response type for ListTools")
		}
		return toolList.Tools, nil
	}
}

func (m *Manager) readLoop() {
	defer m.readWg.Done()
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}

		// Clean up orphaned stream assemblies.
		now := time.Now()
		for id, asm := range m.streams {
			if now.Sub(asm.created) > m.cfg.CallTimeout {
				delete(m.streams, id)
			}
		}

		env, rawPayload, err := envelope.ReadRaw(m.conn)
		if err != nil {
			select {
			case <-m.stopCh:
				// Intentional stop.
			default:
				if err != io.EOF {
					select {
					case m.crashCh <- fmt.Errorf("read error: %w", err):
					default:
					}
				}
			}
			return
		}

		// Raw sideband transfer — payload arrived without protobuf wrapping.
		// Check before reqID routing since RawHeader carries its own request_id.
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

		reqID := env.GetRequestId()
		if reqID == "" {
			// Route unsolicited messages by type
			switch {
			case env.GetToolList() != nil, env.GetRegisterMiddleware() != nil, env.GetReloadResponse() != nil:
				select {
				case m.handshakeCh <- env:
				default:
				}
			case env.GetProgress() != nil:
				if m.onProgress != nil {
					m.onProgress(env.GetProgress())
				}
			case env.GetLog() != nil:
				if m.onLog != nil {
					m.onLog(env.GetLog())
				}
			case env.GetEnableTools() != nil:
				if m.onEnableTools != nil {
					m.onEnableTools(env.GetEnableTools().ToolNames)
				}
			case env.GetDisableTools() != nil:
				if m.onDisableTools != nil {
					m.onDisableTools(env.GetDisableTools().ToolNames)
				}
			default:
				select {
				case m.handshakeCh <- env:
				default:
				}
			}
			continue
		}

		// Reverse requests from SDK process (SDK → Go).
		// These arrive WITH a request_id but are NOT responses to pending Go requests.
		if sr := env.GetSamplingRequest(); sr != nil {
			if m.onSampling != nil {
				go m.onSampling(sr, reqID)
			}
			continue
		}
		if env.GetListRootsRequest() != nil {
			if m.onListRoots != nil {
				go m.onListRoots(reqID)
			}
			continue
		}

		// Stream reassembly / forwarding.
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
	}
}

func (m *Manager) cleanup() {
	if m.conn != nil {
		m.conn.Close()
	}
	if m.listener != nil {
		m.listener.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
	if m.cfg.SocketPath != "" {
		os.Remove(m.cfg.SocketPath)
	}
}
