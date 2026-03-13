use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;
use crate::proto;
use crate::transport::Transport;
use crate::tool::with_registry;
use crate::context::ToolContext;
use crate::middleware::with_middleware_registry;

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
                send_middleware_registrations(&transport).await;
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
    send_middleware_registrations(transport).await;
}

async fn send_middleware_registrations(transport: &Transport) {
    let names_and_priorities: Vec<(String, i32)> = with_middleware_registry(|mws| {
        mws.iter().map(|mw| (mw.name.clone(), mw.priority)).collect()
    });

    for (name, priority) in &names_and_priorities {
        let reg = proto::Envelope {
            msg: Some(proto::envelope::Msg::RegisterMiddleware(proto::RegisterMiddlewareRequest {
                name: name.clone(),
                priority: *priority,
            })),
            ..Default::default()
        };
        let _ = transport.send(&reg).await;
        // Wait for acknowledgment
        if transport.recv().await.is_err() {
            return;
        }
    }
    send_handshake_complete(transport).await;
}

async fn handle_middleware_intercept(transport: &Transport, req: &proto::MiddlewareInterceptRequest, request_id: &str) {
    let mut args_json = req.arguments_json.clone();
    let mut result_json = req.result_json.clone();
    let mut reject = false;
    let mut reject_reason = String::new();

    let mw_result: Option<HashMap<String, String>> = with_middleware_registry(|mws| {
        mws.iter()
            .find(|mw| mw.name == req.middleware_name)
            .map(|mw| (mw.handler)(&req.phase, &req.tool_name, &req.arguments_json, &req.result_json, req.is_error))
    });

    if let Some(result) = mw_result {
        if result.get("reject").is_some_and(|v| v == "true") {
            reject = true;
            if let Some(reason) = result.get("reject_reason") {
                reject_reason = reason.clone();
            }
        }
        if let Some(v) = result.get("arguments_json") {
            args_json = v.clone();
        }
        if let Some(v) = result.get("result_json") {
            result_json = v.clone();
        }
    }

    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::MiddlewareInterceptResponse(proto::MiddlewareInterceptResponse {
            arguments_json: args_json,
            result_json,
            reject,
            reject_reason,
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}
