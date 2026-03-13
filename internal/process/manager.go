package process

import (
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
	pending  map[string]chan *pb.Envelope
	tools    []*pb.ToolDefinition
	crashCh  chan error
	stopCh   chan struct{}
	readWg   sync.WaitGroup
	nextID   int

	// handshakeCh receives unsolicited ToolListResponse messages (no request_id).
	handshakeCh chan *pb.Envelope
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

	if err := envelope.Write(m.conn, env); err != nil {
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

	if err := envelope.Write(m.conn, env); err != nil {
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

	// Wait for the unsolicited ToolListResponse (no request_id).
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, fmt.Errorf("waiting for tool list after reload timed out")
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

	if err := envelope.Write(m.conn, env); err != nil {
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

		env, err := envelope.Read(m.conn)
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

		reqID := env.GetRequestId()
		if reqID == "" {
			// Unsolicited message (e.g., ToolListResponse after reload).
			select {
			case m.handshakeCh <- env:
			default:
			}
			continue
		}

		m.mu.Lock()
		ch, ok := m.pending[reqID]
		m.mu.Unlock()

		if ok {
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
