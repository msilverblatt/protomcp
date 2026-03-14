use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};

pub struct ToolContext {
    pub progress_token: String,
    cancelled: Arc<AtomicBool>,
    send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>,
}

impl ToolContext {
    pub(crate) fn new(
        progress_token: String,
        cancelled: Arc<AtomicBool>,
        send_fn: Box<dyn Fn(Vec<u8>) + Send + Sync>,
    ) -> Self {
        Self { progress_token, cancelled, send_fn }
    }

    pub fn is_cancelled(&self) -> bool {
        self.cancelled.load(Ordering::Relaxed)
    }

    pub fn report_progress(&self, progress: i64, total: i64) {
        if self.progress_token.is_empty() {
            return;
        }
        use crate::proto;
        use prost::Message;
        let env = proto::Envelope {
            msg: Some(proto::envelope::Msg::Progress(proto::ProgressNotification {
                progress_token: self.progress_token.clone(),
                progress,
                total,
                message: String::new(),
            })),
            ..Default::default()
        };
        (self.send_fn)(env.encode_to_vec());
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_cancelled() {
        let cancelled = Arc::new(AtomicBool::new(false));
        let ctx = ToolContext::new(
            "tok".to_string(),
            cancelled.clone(),
            Box::new(|_| {}),
        );
        assert!(!ctx.is_cancelled());
        cancelled.store(true, Ordering::Relaxed);
        assert!(ctx.is_cancelled());
    }
}
