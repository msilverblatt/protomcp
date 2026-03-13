package protomcp

import (
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestServerLogger_Info(t *testing.T) {
	var sent *pb.Envelope
	logger := NewServerLogger(func(env *pb.Envelope) error {
		sent = env
		return nil
	}, "test-logger")

	logger.Info("hello")

	if sent == nil {
		t.Fatal("expected envelope to be sent")
	}
	logMsg := sent.GetLog()
	if logMsg == nil {
		t.Fatal("expected LogMessage")
	}
	if logMsg.Level != "info" {
		t.Fatalf("expected level info, got %s", logMsg.Level)
	}
	if logMsg.Logger != "test-logger" {
		t.Fatalf("expected logger test-logger, got %s", logMsg.Logger)
	}
	if logMsg.DataJson != "hello" {
		t.Fatalf("expected data hello, got %s", logMsg.DataJson)
	}
}

func TestServerLogger_AllLevels(t *testing.T) {
	levels := []struct {
		name string
		fn   func(*ServerLogger, string)
	}{
		{"debug", func(l *ServerLogger, m string) { l.Debug(m) }},
		{"info", func(l *ServerLogger, m string) { l.Info(m) }},
		{"notice", func(l *ServerLogger, m string) { l.Notice(m) }},
		{"warning", func(l *ServerLogger, m string) { l.Warning(m) }},
		{"error", func(l *ServerLogger, m string) { l.Error(m) }},
		{"critical", func(l *ServerLogger, m string) { l.Critical(m) }},
		{"alert", func(l *ServerLogger, m string) { l.Alert(m) }},
		{"emergency", func(l *ServerLogger, m string) { l.Emergency(m) }},
	}

	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			var sent *pb.Envelope
			logger := NewServerLogger(func(env *pb.Envelope) error {
				sent = env
				return nil
			}, "test")
			tc.fn(logger, "msg")
			if sent == nil {
				t.Fatal("expected envelope")
			}
			if sent.GetLog().Level != tc.name {
				t.Fatalf("expected %s, got %s", tc.name, sent.GetLog().Level)
			}
		})
	}
}
