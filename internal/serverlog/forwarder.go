package serverlog

import (
	"encoding/json"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

var levelPriority = map[string]int{
	"debug": 0, "info": 1, "notice": 2, "warning": 3,
	"error": 4, "critical": 5, "alert": 6, "emergency": 7,
}

type NotifySender func(notification map[string]any)

type Forwarder struct {
	minLevel int
	send     NotifySender
}

func NewForwarder(minLevelStr string, send NotifySender) *Forwarder {
	return &Forwarder{minLevel: levelPriority[minLevelStr], send: send}
}

func (f *Forwarder) HandleLog(msg *pb.LogMessage) {
	priority, ok := levelPriority[msg.Level]
	if !ok {
		priority = 1
	}
	if priority < f.minLevel {
		return
	}
	params := map[string]any{"level": msg.Level}
	if msg.Logger != "" {
		params["logger"] = msg.Logger
	}
	if msg.DataJson != "" {
		var data any
		if err := json.Unmarshal([]byte(msg.DataJson), &data); err == nil {
			params["data"] = data
		} else {
			params["data"] = msg.DataJson
		}
	}
	f.send(map[string]any{
		"method": "notifications/message",
		"params": params,
	})
}
