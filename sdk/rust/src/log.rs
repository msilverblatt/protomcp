use crate::proto;
use prost::Message;

pub struct ServerLogger {
    send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>,
    logger: String,
}

impl ServerLogger {
    pub fn new(send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>, logger: &str) -> Self {
        Self { send_fn, logger: logger.to_string() }
    }

    fn log(&self, level: &str, data_json: &str) {
        let env = proto::Envelope {
            msg: Some(proto::envelope::Msg::Log(proto::LogMessage {
                level: level.to_string(),
                logger: self.logger.clone(),
                data_json: data_json.to_string(),
            })),
            ..Default::default()
        };
        let buf = env.encode_to_vec();
        (self.send_fn)(buf);
    }

    pub fn debug(&self, msg: &str)     { self.log("debug", msg) }
    pub fn info(&self, msg: &str)      { self.log("info", msg) }
    pub fn notice(&self, msg: &str)    { self.log("notice", msg) }
    pub fn warning(&self, msg: &str)   { self.log("warning", msg) }
    pub fn error(&self, msg: &str)     { self.log("error", msg) }
    pub fn critical(&self, msg: &str)  { self.log("critical", msg) }
    pub fn alert(&self, msg: &str)     { self.log("alert", msg) }
    pub fn emergency(&self, msg: &str) { self.log("emergency", msg) }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{Arc, Mutex};

    #[test]
    fn test_log_levels() {
        let levels = ["debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"];
        for level in &levels {
            let captured: Arc<Mutex<Option<Vec<u8>>>> = Arc::new(Mutex::new(None));
            let cap = captured.clone();
            let logger = ServerLogger::new(
                Box::new(move |data| { *cap.lock().unwrap() = Some(data); }),
                "test",
            );
            match *level {
                "debug" => logger.debug("msg"),
                "info" => logger.info("msg"),
                "notice" => logger.notice("msg"),
                "warning" => logger.warning("msg"),
                "error" => logger.error("msg"),
                "critical" => logger.critical("msg"),
                "alert" => logger.alert("msg"),
                "emergency" => logger.emergency("msg"),
                _ => {}
            }
            let data = captured.lock().unwrap();
            assert!(data.is_some(), "expected data for level {}", level);
            // Verify we can decode the envelope
            let env = proto::Envelope::decode(data.as_ref().unwrap().as_slice()).unwrap();
            if let Some(proto::envelope::Msg::Log(log)) = env.msg {
                assert_eq!(log.level, *level);
                assert_eq!(log.logger, "test");
                assert_eq!(log.data_json, "msg");
            } else {
                panic!("expected Log message for level {}", level);
            }
        }
    }
}
