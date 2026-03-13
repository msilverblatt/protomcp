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
