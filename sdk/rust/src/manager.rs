use crate::proto;
use prost::Message;

pub struct ToolManager {
    send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>,
}

impl ToolManager {
    pub fn new(send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>) -> Self {
        Self { send_fn }
    }

    pub fn enable(&self, names: &[&str]) {
        let env = proto::Envelope {
            msg: Some(proto::envelope::Msg::EnableTools(proto::EnableToolsRequest {
                tool_names: names.iter().map(|s| s.to_string()).collect(),
            })),
            ..Default::default()
        };
        (self.send_fn)(env.encode_to_vec());
    }

    pub fn disable(&self, names: &[&str]) {
        let env = proto::Envelope {
            msg: Some(proto::envelope::Msg::DisableTools(proto::DisableToolsRequest {
                tool_names: names.iter().map(|s| s.to_string()).collect(),
            })),
            ..Default::default()
        };
        (self.send_fn)(env.encode_to_vec());
    }
}
