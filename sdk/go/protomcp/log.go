package protomcp

import (
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

type ServerLogger struct {
	sendFn func(*pb.Envelope) error
	logger string
}

func NewServerLogger(sendFn func(*pb.Envelope) error, logger string) *ServerLogger {
	return &ServerLogger{sendFn: sendFn, logger: logger}
}

func (l *ServerLogger) log(level, dataJSON string) {
	if l.sendFn == nil {
		return
	}
	l.sendFn(&pb.Envelope{
		Msg: &pb.Envelope_Log{
			Log: &pb.LogMessage{
				Level:    level,
				Logger:   l.logger,
				DataJson: dataJSON,
			},
		},
	})
}

func (l *ServerLogger) Debug(msg string)     { l.log("debug", msg) }
func (l *ServerLogger) Info(msg string)      { l.log("info", msg) }
func (l *ServerLogger) Notice(msg string)    { l.log("notice", msg) }
func (l *ServerLogger) Warning(msg string)   { l.log("warning", msg) }
func (l *ServerLogger) Error(msg string)     { l.log("error", msg) }
func (l *ServerLogger) Critical(msg string)  { l.log("critical", msg) }
func (l *ServerLogger) Alert(msg string)     { l.log("alert", msg) }
func (l *ServerLogger) Emergency(msg string) { l.log("emergency", msg) }
