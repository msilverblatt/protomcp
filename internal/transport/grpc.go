package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GRPCTransport implements the Transport interface using gRPC.
// It exposes a unary RPC: Call(StringValue) → StringValue where the string
// values contain JSON-encoded JSON-RPC request and response payloads.
type GRPCTransport struct {
	host   string
	port   int
	server *grpc.Server
}

// NewGRPCTransport creates a new GRPCTransport.
func NewGRPCTransport(host string, port int) *GRPCTransport {
	return &GRPCTransport{host: host, port: port}
}

// MCPServiceServer is the interface that the gRPC service implementation must satisfy.
// RegisterService uses a pointer-to-interface as HandlerType.
type MCPServiceServer interface {
	call(ctx context.Context, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

// grpcMCPServer implements MCPServiceServer.
type grpcMCPServer struct {
	handler RequestHandler
}

// MCPServiceDesc describes the MCPService for manual gRPC registration.
// The Call RPC accepts a JSON-encoded JSON-RPC request (as StringValue.value)
// and returns the JSON-encoded JSON-RPC response (as StringValue.value).
var MCPServiceDesc = grpc.ServiceDesc{
	ServiceName: "protomcp.MCPService",
	HandlerType: (*MCPServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Call",
			Handler:    grpcCallHandler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "transport.proto",
}

// grpcCallHandler is the gRPC unary handler for MCPService.Call.
func grpcCallHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*grpcMCPServer).call(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/protomcp.MCPService/Call",
	}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*grpcMCPServer).call(ctx, req.(*wrapperspb.StringValue))
	})
}

// call processes a JSON-RPC request payload and returns the response payload.
func (s *grpcMCPServer) call(ctx context.Context, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	var jsonReq mcp.JSONRPCRequest
	if err := json.Unmarshal([]byte(req.GetValue()), &jsonReq); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON-RPC payload: %v", err)
	}

	resp, err := s.handler(ctx, jsonReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "handler error: %v", err)
	}

	if resp == nil {
		return wrapperspb.String(""), nil
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal response: %v", err)
	}

	return wrapperspb.String(string(data)), nil
}

// Start starts the gRPC server and blocks until ctx is cancelled.
func (g *GRPCTransport) Start(ctx context.Context, handler RequestHandler) error {
	addr := fmt.Sprintf("%s:%d", g.host, g.port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}

	g.server = grpc.NewServer()
	svc := &grpcMCPServer{handler: handler}
	g.server.RegisterService(&MCPServiceDesc, svc)

	errCh := make(chan error, 1)
	go func() {
		if err := g.server.Serve(lis); err != nil {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		g.server.GracefulStop()
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// SendNotification is a no-op for gRPC transport (unary RPC has no push channel).
func (g *GRPCTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	// Unary gRPC transport has no persistent client connections to push to.
	return nil
}

// Close shuts down the gRPC server.
func (g *GRPCTransport) Close() error {
	if g.server != nil {
		g.server.Stop()
	}
	return nil
}
