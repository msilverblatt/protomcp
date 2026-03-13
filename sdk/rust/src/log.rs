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
