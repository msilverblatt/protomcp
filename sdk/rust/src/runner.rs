use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::AtomicBool;
use crate::proto;
use crate::transport::Transport;
use crate::tool::with_registry;
use crate::context::ToolContext;
use crate::middleware::with_middleware_registry;
use crate::resource::{with_resources, with_resource_templates};
use crate::prompt::with_prompts;
use crate::completion::get_completion_handler;
use crate::server_context::resolve_contexts;
use crate::local_middleware::build_middleware_chain;
use crate::telemetry::{ToolCallEvent, emit_telemetry};
use crate::sidecar::start_sidecars;

fn build_result_json(text: &str) -> String {
    let content = serde_json::json!([{"type": "text", "text": text}]);
    content.to_string()
}

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
            Some(proto::envelope::Msg::ListResourcesRequest(_)) => {
                handle_list_resources(&transport, &request_id).await;
            }
            Some(proto::envelope::Msg::ListResourceTemplatesRequest(_)) => {
                handle_list_resource_templates(&transport, &request_id).await;
            }
            Some(proto::envelope::Msg::ReadResourceRequest(req)) => {
                handle_read_resource(&transport, &req, &request_id).await;
            }
            Some(proto::envelope::Msg::ListPromptsRequest(_)) => {
                handle_list_prompts(&transport, &request_id).await;
            }
            Some(proto::envelope::Msg::GetPromptRequest(req)) => {
                handle_get_prompt(&transport, &req, &request_id).await;
            }
            Some(proto::envelope::Msg::CompletionRequest(req)) => {
                handle_completion(&transport, &req, &request_id).await;
            }
            _ => {}
        }
    }
}

async fn handle_list_tools(transport: &Transport, request_id: &str) {
    let defs: Vec<proto::ToolDefinition> = with_registry(|tools| {
        tools.iter().map(|t| {
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
        }).collect()
    });

    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::ToolList(proto::ToolListResponse { tools: defs })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
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
    let mut args: serde_json::Value = if req.arguments_json.is_empty() {
        serde_json::json!({})
    } else {
        serde_json::from_str(&req.arguments_json).unwrap_or(serde_json::json!({}))
    };

    // Start sidecars on first tool call
    start_sidecars("first_tool_call");

    // Resolve server contexts
    let ctx_values = resolve_contexts(&mut args);
    // Inject resolved context values into args
    if let Some(obj) = args.as_object_mut() {
        for (key, val) in ctx_values {
            obj.insert(key, val);
        }
    }

    let tool_name = req.name.clone();

    // Emit start telemetry
    let start_time = std::time::Instant::now();
    emit_telemetry(ToolCallEvent {
        tool_name: tool_name.clone(),
        phase: "start".to_string(),
        args_json: args.to_string(),
        ..ToolCallEvent::new(&tool_name, "start")
    });

    let handler_opt = with_registry(|tools| {
        tools.iter()
            .find(|t| t.name == req.name)
            .map(|t| t.handler.clone())
    });

    let result = if let Some(handler) = handler_opt {
        let cancelled = Arc::new(AtomicBool::new(false));
        let ctx = ToolContext::new(
            req.progress_token.clone(),
            cancelled,
            Box::new(|_| {}),
        );

        let chain_handler: Box<dyn Fn(ToolContext, serde_json::Value) -> crate::result::ToolResult + Send + Sync> =
            Box::new(move |ctx, args| {
                handler(ctx, args)
            });

        let chain = build_middleware_chain(&req.name, chain_handler);
        chain(ctx, args)
    } else {
        crate::result::ToolResult::error(
            format!("Tool not found: {}", req.name),
            "NOT_FOUND",
            "",
            false,
        )
    };

    // Emit completion telemetry
    let duration = start_time.elapsed().as_millis() as i64;
    let phase = if result.is_error { "error" } else { "success" };
    emit_telemetry(ToolCallEvent {
        tool_name: tool_name.clone(),
        phase: phase.to_string(),
        duration_ms: duration,
        result_text: result.result_text.clone(),
        is_error: result.is_error,
        error_code: result.error_code.clone(),
        ..ToolCallEvent::new(&tool_name, phase)
    });

    let mut resp = proto::CallToolResponse {
        is_error: result.is_error,
        result_json: build_result_json(&result.result_text),
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

    let chunk_threshold: usize = std::env::var("PROTOMCP_CHUNK_THRESHOLD")
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(65536);

    let result_bytes = resp.result_json.as_bytes();
    if result_bytes.len() > chunk_threshold {
        let _ = transport.send_raw(request_id, "result_json", result_bytes).await;
    } else {
        let env = proto::Envelope {
            request_id: request_id.to_string(),
            msg: Some(proto::envelope::Msg::CallResult(resp)),
            ..Default::default()
        };
        let _ = transport.send(&env).await;
    }
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

async fn handle_list_resources(transport: &Transport, request_id: &str) {
    let defs: Vec<proto::ResourceDefinition> = with_resources(|resources| {
        resources.iter().map(|r| proto::ResourceDefinition {
            uri: r.uri.clone(),
            name: r.name.clone(),
            description: r.description.clone(),
            mime_type: r.mime_type.clone(),
            ..Default::default()
        }).collect()
    });
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::ResourceListResponse(proto::ResourceListResponse { resources: defs })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_list_resource_templates(transport: &Transport, request_id: &str) {
    let defs: Vec<proto::ResourceTemplateDefinition> = with_resource_templates(|templates| {
        templates.iter().map(|t| proto::ResourceTemplateDefinition {
            uri_template: t.uri_template.clone(),
            name: t.name.clone(),
            description: t.description.clone(),
            mime_type: t.mime_type.clone(),
        }).collect()
    });
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::ResourceTemplateListResponse(proto::ResourceTemplateListResponse { templates: defs })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_read_resource(transport: &Transport, req: &proto::ReadResourceRequest, request_id: &str) {
    let uri = &req.uri;
    let contents: Vec<proto::ResourceContent> = with_resources(|resources| {
        for r in resources {
            if r.uri == *uri {
                return (r.handler)(uri).iter().map(|c| proto::ResourceContent {
                    uri: c.uri.clone(),
                    mime_type: c.mime_type.clone(),
                    text: c.text.clone(),
                    blob: c.blob.clone(),
                }).collect();
            }
        }
        Vec::new()
    });
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::ReadResourceResponse(proto::ReadResourceResponse { contents })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_list_prompts(transport: &Transport, request_id: &str) {
    let defs: Vec<proto::PromptDefinition> = with_prompts(|prompts| {
        prompts.iter().map(|p| proto::PromptDefinition {
            name: p.name.clone(),
            description: p.description.clone(),
            arguments: p.arguments.iter().map(|a| proto::PromptArgument {
                name: a.name.clone(),
                description: a.description.clone(),
                required: a.required,
            }).collect(),
        }).collect()
    });
    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::PromptListResponse(proto::PromptListResponse { prompts: defs })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_get_prompt(transport: &Transport, req: &proto::GetPromptRequest, request_id: &str) {
    let args: serde_json::Value = if req.arguments_json.is_empty() {
        serde_json::json!({})
    } else {
        serde_json::from_str(&req.arguments_json).unwrap_or(serde_json::json!({}))
    };

    let messages: Vec<proto::PromptMessage> = with_prompts(|prompts| {
        if let Some(p) = prompts.iter().find(|p| p.name == req.name) {
            (p.handler)(args.clone()).iter().map(|m| proto::PromptMessage {
                role: m.role.clone(),
                content_json: serde_json::json!({"type": "text", "text": m.content}).to_string(),
            }).collect()
        } else {
            Vec::new()
        }
    });

    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::GetPromptResponse(proto::GetPromptResponse {
            description: String::new(),
            messages,
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

async fn handle_completion(transport: &Transport, req: &proto::CompletionRequest, request_id: &str) {
    let result = get_completion_handler(&req.ref_type, &req.ref_name, &req.argument_name);
    let (values, total, has_more) = match result {
        Some(r) => (r.values, r.total, r.has_more),
        None => (Vec::new(), 0, false),
    };

    let resp = proto::Envelope {
        request_id: request_id.to_string(),
        msg: Some(proto::envelope::Msg::CompletionResponse(proto::CompletionResponse {
            values,
            total,
            has_more,
        })),
        ..Default::default()
    };
    let _ = transport.send(&resp).await;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_result_json_escaping() {
        let cases = vec![
            ("simple", "hello"),
            ("quotes", r#"He said "hello""#),
            ("backslash", r"path\to\file"),
            ("newline", "line1\nline2"),
        ];
        for (name, input) in cases {
            let json_str = build_result_json(input);
            let parsed: serde_json::Value = serde_json::from_str(&json_str)
                .unwrap_or_else(|e| panic!("{}: invalid JSON: {} — got: {}", name, e, json_str));
            assert_eq!(parsed[0]["text"].as_str().unwrap(), input, "case: {}", name);
        }
    }
}
