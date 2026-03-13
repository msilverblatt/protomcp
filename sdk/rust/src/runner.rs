use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use crate::proto;
use crate::transport::Transport;
use crate::tool::with_registry;
use crate::context::ToolContext;

pub async fn run() {
    let socket_path = std::env::var("PROTOMCP_SOCKET")
        .expect("PROTOMCP_SOCKET not set — run via 'pmcp dev'");

    let transport = Transport::connect(&socket_path).await
        .expect("failed to connect to socket");

    loop {
        let env = match transport.recv().await {
            Ok(env) => env,
            Err(_) => break,
        };

        let request_id = env.request_id.clone();

        match env.msg {
            Some(proto::envelope::Msg::ListTools(_)) => {
                handle_list_tools(&transport, &request_id).await;
                send_handshake_complete(&transport).await;
            }
            Some(proto::envelope::Msg::CallTool(req)) => {
                handle_call_tool(&transport, &req, &request_id).await;
            }
            Some(proto::envelope::Msg::Reload(_)) => {
                handle_reload(&transport, &request_id).await;
            }
            Some(proto::envelope::Msg::MiddlewareIntercept(req)) => {
                handle_middleware_intercept(&transport, &req, &request_id).await;
            }
            _ => {}
        }
    }
}

async fn handle_list_tools(transport: &Transport, request_id: &str) {
    with_registry(|tools| {
        let defs: Vec<proto::ToolDefinition> = tools.iter().map(|t| {
            proto::ToolDefinition {
                name: t.name.clone(),
                description: t.description.clone(),
                input_schema_json: t.input_schema.to_string(),
                destructive_hint: t.destructive,
                idempotent_hint: t.idempotent,
                read_only_hint: t.read_only,
                open_world_hint: t.open_world,
                task_support: t.task_support,
                ..Default::default()
            }
        }).collect();

        let resp = proto::Envelope {
            request_id: request_id.to_string(),
            msg: Some(proto::envelope::Msg::ToolList(proto::ToolListResponse { tools: defs })),
            ..Default::default()
        };
        let tp = transport.clone();
        tokio::spawn(async move { let _ = tp.send(&resp).await; });
    });
}

async fn send_handshake_complete(transport: &Transport) {
    let resp = proto::Envelope {
        request_id: String::new(),
        msg: Some(proto::envelope::Msg::ReloadResponse(proto::ReloadResponse {
            success: true,
            error: String::new(),
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_call_tool(transport: &Transport, req: &proto::CallToolRequest, request_id: &str) {
    let args: serde_json::Value = if req.arguments_json.is_empty() {
        serde_json::json!({})
    } else {
        serde_json::from_str(&req.arguments_json).unwrap_or(serde_json::json!({}))
    };

    let result = with_registry(|tools| {
        if let Some(tool_def) = tools.iter().find(|t| t.name == req.name) {
            let cancelled = Arc::new(AtomicBool::new(false));
            let ctx = ToolContext::new(
                req.progress_token.clone(),
                cancelled,
                Box::new(|_| {}),
            );
            (tool_def.handler)(ctx, args.clone())
        } else {
            crate::result::ToolResult::error(
                format!("Tool not found: {}", req.name),
                "NOT_FOUND",
                "",
                false,
            )
        }
    });

    let mut resp = proto::CallToolResponse {
        is_error: result.is_error,
        result_json: format!(r#"[{{"type":"text","text":"{}"}}]"#, result.result_text),
        enable_tools: result.enable_tools,
        disable_tools: result.disable_tools,
        ..Default::default()
    };

    if result.is_error && !result.error_code.is_empty() {
        resp.error = Some(proto::ToolError {
            error_code: result.error_code,
            message: result.message,
            suggestion: result.suggestion,
            retryable: result.retryable,
        });
    }

    let env = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::CallResult(resp)),
        ..Default::default()
    };
    let _ = transport.send(&env).await;
}

async fn handle_reload(transport: &Transport, request_id: &str) {
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::ReloadResponse(proto::ReloadResponse {
            success: true,
            error: String::new(),
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
    handle_list_tools(transport, "").await;
    send_handshake_complete(transport).await;
}

async fn handle_middleware_intercept(transport: &Transport, req: &proto::MiddlewareInterceptRequest, request_id: &str) {
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::MiddlewareInterceptResponse(proto::MiddlewareInterceptResponse {
            arguments_json: req.arguments_json.clone(),
            result_json: req.result_json.clone(),
            reject: false,
            reject_reason: String::new(),
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}
