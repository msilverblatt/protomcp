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

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    #[test]
    fn test_enable() {
        let captured: Arc<Mutex<Option<Vec<u8>>>> = Arc::new(Mutex::new(None));
        let cap = captured.clone();
        let mgr = ToolManager::new(Box::new(move |data| {
            *cap.lock().unwrap() = Some(data);
        }));

        mgr.enable(&["tool_a", "tool_b"]);

        let data = captured.lock().unwrap();
        assert!(data.is_some());
        let env = proto::Envelope::decode(data.as_ref().unwrap().as_slice()).unwrap();
        if let Some(proto::envelope::Msg::EnableTools(req)) = env.msg {
            assert_eq!(req.tool_names, vec!["tool_a", "tool_b"]);
        } else {
            panic!("expected EnableTools message");
        }
    }

    #[test]
    fn test_disable() {
        let captured: Arc<Mutex<Option<Vec<u8>>>> = Arc::new(Mutex::new(None));
        let cap = captured.clone();
        let mgr = ToolManager::new(Box::new(move |data| {
            *cap.lock().unwrap() = Some(data);
        }));

        mgr.disable(&["admin_panel"]);

        let data = captured.lock().unwrap();
        assert!(data.is_some());
        let env = proto::Envelope::decode(data.as_ref().unwrap().as_slice()).unwrap();
        if let Some(proto::envelope::Msg::DisableTools(req)) = env.msg {
            assert_eq!(req.tool_names, vec!["admin_panel"]);
        } else {
            panic!("expected DisableTools message");
        }
    }
}
