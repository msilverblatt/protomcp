package progress

import pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"

type NotifySender func(notification map[string]any)

type Proxy struct {
	send NotifySender
}

func NewProxy(send NotifySender) *Proxy {
	return &Proxy{send: send}
}

func (p *Proxy) HandleProgress(msg *pb.ProgressNotification) {
	if msg.ProgressToken == "" {
		return
	}
	params := map[string]any{
		"progressToken": msg.ProgressToken,
		"progress":      msg.Progress,
	}
	if msg.Total > 0 {
		params["total"] = msg.Total
	}
	if msg.Message != "" {
		params["message"] = msg.Message
	}
	p.send(map[string]any{
		"method": "notifications/progress",
		"params": params,
	})
}
