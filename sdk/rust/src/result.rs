pub struct ToolResult {
    pub result_text: String,
    pub is_error: bool,
    pub error_code: String,
    pub message: String,
    pub suggestion: String,
    pub retryable: bool,
    pub enable_tools: Vec<String>,
    pub disable_tools: Vec<String>,
}

impl ToolResult {
    pub fn new(text: impl Into<String>) -> Self {
        Self {
            result_text: text.into(),
            is_error: false,
            error_code: String::new(),
            message: String::new(),
            suggestion: String::new(),
            retryable: false,
            enable_tools: Vec::new(),
            disable_tools: Vec::new(),
        }
    }

    pub fn error(text: impl Into<String>, code: impl Into<String>, suggestion: impl Into<String>, retryable: bool) -> Self {
        let text = text.into();
        Self {
            result_text: text.clone(),
            is_error: true,
            error_code: code.into(),
            message: text,
            suggestion: suggestion.into(),
            retryable,
            enable_tools: Vec::new(),
            disable_tools: Vec::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tool_result_basic() {
        let r = ToolResult::new("hello");
        assert_eq!(r.result_text, "hello");
        assert!(!r.is_error);
    }

    #[test]
    fn test_tool_result_error() {
        let r = ToolResult::error("failed", "INVALID", "try again", true);
        assert!(r.is_error);
        assert_eq!(r.error_code, "INVALID");
    }
}
