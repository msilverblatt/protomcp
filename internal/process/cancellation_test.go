package process_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/envelope"
	"github.com/msilverblatt/protomcp/internal/process"
)

func TestToolCancellationForwarded(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, timeout := range []bool{false, true} {
			name := "call"
			if stream {
				name = "stream"
			}
			if timeout {
				name += "-timeout"
			} else {
				name += "-cancel"
			}
			t.Run(name, func(t *testing.T) {
				parent, child := net.Pipe()
				defer parent.Close()
				defer child.Close()
				manager := process.NewManagerForTest(process.ManagerConfig{CallTimeout: 100 * time.Millisecond}, parent)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				returned := make(chan error, 1)
				go func() {
					if stream {
						events, err := manager.CallToolStream(ctx, "wait", "{}")
						if err == nil {
							for range events {
							}
						}
						returned <- err
					} else {
						_, err := manager.CallTool(ctx, "wait", "{}")
						returned <- err
					}
				}()
				child.SetReadDeadline(time.Now().Add(3 * time.Second))
				request, err := envelope.Read(child)
				if err != nil {
					t.Fatal(err)
				}
				if !timeout {
					cancel()
				}
				notification, err := envelope.Read(child)
				if err != nil {
					t.Fatal(err)
				}
				if notification.GetCancel() == nil || notification.GetCancel().RequestId != request.RequestId {
					t.Fatalf("missing or mismatched cancellation: %v", notification)
				}
				select {
				case err := <-returned:
					if !stream && err == nil {
						t.Fatal("cancelled tool succeeded")
					}
				case <-time.After(time.Second):
					t.Fatal("cancelled caller stuck")
				}
			})
		}
	}
}
