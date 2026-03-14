import * as process from 'process';
import { Transport } from './transport.js';
import { getRegisteredTools } from './tool.js';
import { ToolResult } from './result.js';
import { ToolContext } from './context.js';
import { toolManager } from './manager.js';
import { getRegisteredMiddleware, type MiddlewareDef } from './middleware.js';
import { getRegisteredResources, getRegisteredResourceTemplates, type ResourceContent } from './resource.js';
import { getRegisteredPrompts } from './prompt.js';
import { getCompletionHandler } from './completion.js';
import { resolveContexts } from './serverContext.js';
import { buildMiddlewareChain } from './localMiddleware.js';
import { emitTelemetry } from './telemetry.js';
import { startSidecars } from './sidecar.js';
import { discoverHandlers } from './discovery.js';

export async function run(): Promise<void> {
  const socketPath = process.env['PROTOMCP_SOCKET'];
  if (!socketPath) {
    process.stderr.write("PROTOMCP_SOCKET not set — run via 'pmcp dev'\n");
    process.exit(1);
  }

  const transport = new Transport(socketPath);
  await transport.connect();
  toolManager._init(transport as any);

  // Discover handlers and start server_start sidecars
  await discoverHandlers();
  await startSidecars('server_start');

  const root = await transport.getRoot();
  const Envelope = root.lookupType('protomcp.Envelope');
  const ToolDefinition = root.lookupType('protomcp.ToolDefinition');
  const ToolListResponse = root.lookupType('protomcp.ToolListResponse');
  const CallToolResponse = root.lookupType('protomcp.CallToolResponse');
  const ToolError = root.lookupType('protomcp.ToolError');
  const ReloadResponse = root.lookupType('protomcp.ReloadResponse');
  const RegisterMiddlewareRequest = root.lookupType('protomcp.RegisterMiddlewareRequest');
  const MiddlewareInterceptResponse = root.lookupType('protomcp.MiddlewareInterceptResponse');

  // Map middleware names to handlers for intercept dispatch
  const mwHandlers = new Map<string, MiddlewareDef['handler']>();

  async function sendMiddlewareRegistrations(): Promise<void> {
    const mwDefs = getRegisteredMiddleware();
    for (const mw of mwDefs) {
      mwHandlers.set(mw.name, mw.handler);
      const reg = Envelope.create({
        registerMiddleware: RegisterMiddlewareRequest.create({
          name: mw.name,
          priority: mw.priority,
        }),
      });
      await transport.send(reg);
      // Wait for acknowledgment
      try {
        await transport.recv();
      } catch {
        return;
      }
    }
    // Send handshake-complete signal
    const complete = Envelope.create({
      reloadResponse: ReloadResponse.create({ success: true }),
    });
    await transport.send(complete);
  }

  async function sendListTools(requestId: string): Promise<void> {
    const tools = getRegisteredTools();
    const toolDefs = tools.map((t) =>
      ToolDefinition.create({
        name: t.name,
        description: t.description,
        inputSchemaJson: t.inputSchemaJson,
        outputSchemaJson: t.outputSchemaJson,
        title: t.title,
        destructiveHint: t.destructiveHint,
        idempotentHint: t.idempotentHint,
        readOnlyHint: t.readOnlyHint,
        openWorldHint: t.openWorldHint,
        taskSupport: t.taskSupport,
      })
    );
    const resp = Envelope.create({
      toolList: ToolListResponse.create({ tools: toolDefs }),
      requestId,
    });
    await transport.send(resp);
  }

  while (true) {
    let env: Record<string, any>;
    try {
      env = await transport.recv();
    } catch {
      break;
    }

    const requestId: string = (env as any).requestId ?? '';

    if (env['msg'] === 'listTools') {
      await sendListTools(requestId);
      await sendMiddlewareRegistrations();
    } else if (env['msg'] === 'middlewareIntercept') {
      const req = env['middlewareIntercept'] ?? {};
      const mwName: string = req['middlewareName'] ?? '';
      const handler = mwHandlers.get(mwName);

      let respFields: Record<string, any> = {};
      if (handler) {
        try {
          const result = handler(
            req['phase'] ?? '',
            req['toolName'] ?? '',
            req['argumentsJson'] ?? '',
            req['resultJson'] ?? '',
            req['isError'] ?? false
          );
          if (result) respFields = result;
        } catch (err: any) {
          respFields = { reject: true, rejectReason: String(err?.message ?? err) };
        }
      }

      const resp = Envelope.create({
        middlewareInterceptResponse: MiddlewareInterceptResponse.create({
          argumentsJson: respFields['argumentsJson'] ?? respFields['arguments_json'] ?? '',
          resultJson: respFields['resultJson'] ?? respFields['result_json'] ?? '',
          reject: respFields['reject'] ?? false,
          rejectReason: respFields['rejectReason'] ?? respFields['reject_reason'] ?? '',
        }),
        requestId,
      });
      await transport.send(resp);
    } else if (env['msg'] === 'callTool') {
      const req = env['callTool'] ?? {};
      const toolName: string = req['name'] ?? '';
      const argumentsJson: string = req['argumentsJson'] ?? '{}';

      const tools = getRegisteredTools();
      const toolDef = tools.find((t) => t.name === toolName);

      if (!toolDef) {
        const resp = Envelope.create({
          callResult: CallToolResponse.create({
            isError: true,
            resultJson: JSON.stringify([{ type: 'text', text: `Tool not found: ${toolName}` }]),
          }),
          requestId,
        });
        await transport.send(resp);
        continue;
      }

      let respMsg;
      try {
        // Start first_tool_call sidecars
        await startSidecars('first_tool_call');

        let args = argumentsJson ? JSON.parse(argumentsJson) : {};
        const progressToken: string = req['progressToken'] ?? '';
        const ctx = new ToolContext(progressToken, (msg: any) => transport.send(Envelope.create(msg)));

        // Resolve server contexts
        const ctxValues = resolveContexts(args);
        args = { ...args, ...ctxValues };

        // Emit telemetry start
        const startTime = Date.now();
        emitTelemetry({ toolName, action: '', phase: 'start', args });

        // Build local middleware chain around the handler
        const chain = buildMiddlewareChain(toolName, toolDef.handler);
        const result = await chain(ctx, args);

        if (result instanceof ToolResult) {
          const callResp: Record<string, any> = {
            isError: result.isError,
            resultJson: JSON.stringify([{ type: 'text', text: String(result.result) }]),
            enableTools: result.enableTools ?? [],
            disableTools: result.disableTools ?? [],
          };
          if (result.isError && result.errorCode) {
            callResp['error'] = ToolError.create({
              errorCode: result.errorCode,
              message: result.message ?? '',
              suggestion: result.suggestion ?? '',
              retryable: result.retryable,
            });
          }
          respMsg = CallToolResponse.create(callResp);
        } else {
          respMsg = CallToolResponse.create({
            resultJson: JSON.stringify([{ type: 'text', text: String(result) }]),
          });
        }
        emitTelemetry({ toolName, action: '', phase: 'success', args, result: String(result), durationMs: Date.now() - startTime });
      } catch (err: any) {
        respMsg = CallToolResponse.create({
          isError: true,
          resultJson: JSON.stringify([{ type: 'text', text: String(err?.message ?? err) }]),
        });
        emitTelemetry({ toolName, action: '', phase: 'error', args: {}, error: err instanceof Error ? err : new Error(String(err)) });
      }

      // Check if result_json exceeds chunk threshold — stream if so.
      const chunkThreshold = parseInt(process.env['PROTOMCP_CHUNK_THRESHOLD'] ?? '65536', 10);
      const resultJson: string = (respMsg as any).resultJson ?? '';
      const resultBytes = Buffer.from(resultJson, 'utf-8');

      if (resultBytes.length > chunkThreshold) {
        await transport.sendRaw(requestId, 'result_json', resultBytes);
      } else {
        const resp = Envelope.create({ callResult: respMsg, requestId });
        await transport.send(resp);
      }
    } else if (env['msg'] === 'listResourcesRequest') {
      const resources = getRegisteredResources();
      const defs = resources.map(r => ({ uri: r.uri, name: r.name, description: r.description, mimeType: r.mimeType ?? '', size: r.size ?? 0 }));
      const resp = Envelope.create({ resourceListResponse: { resources: defs }, requestId });
      await transport.send(resp);
    } else if (env['msg'] === 'listResourceTemplatesRequest') {
      const templates = getRegisteredResourceTemplates();
      const defs = templates.map(t => ({ uriTemplate: t.uriTemplate, name: t.name, description: t.description, mimeType: t.mimeType ?? '' }));
      const resp = Envelope.create({ resourceTemplateListResponse: { templates: defs }, requestId });
      await transport.send(resp);
    } else if (env['msg'] === 'readResourceRequest') {
      const uri: string = env['readResourceRequest']?.['uri'] ?? '';
      const allResources = getRegisteredResources();
      const allTemplates = getRegisteredResourceTemplates();
      const resDef = allResources.find(r => r.uri === uri);
      const handler = resDef?.handler ?? allTemplates[0]?.handler;
      if (!handler) {
        const resp = Envelope.create({ readResourceResponse: { contents: [{ uri, text: `Resource not found: ${uri}`, mimeType: 'text/plain' }] }, requestId });
        await transport.send(resp);
      } else {
        try {
          let result = handler(uri);
          if (typeof result === 'string') result = [{ uri, text: result }];
          if (!Array.isArray(result)) result = [result];
          const contents = (result as ResourceContent[]).map(c => ({ uri: c.uri, text: c.text ?? '', blob: c.blob ?? new Uint8Array(), mimeType: c.mimeType ?? '' }));
          const resp = Envelope.create({ readResourceResponse: { contents }, requestId });
          await transport.send(resp);
        } catch (err: any) {
          const resp = Envelope.create({ readResourceResponse: { contents: [{ uri, text: String(err?.message ?? err), mimeType: 'text/plain' }] }, requestId });
          await transport.send(resp);
        }
      }
    } else if (env['msg'] === 'listPromptsRequest') {
      const prompts = getRegisteredPrompts();
      const defs = prompts.map(p => ({ name: p.name, description: p.description, arguments: p.arguments.map(a => ({ name: a.name, description: a.description ?? '', required: a.required ?? false })) }));
      const resp = Envelope.create({ promptListResponse: { prompts: defs }, requestId });
      await transport.send(resp);
    } else if (env['msg'] === 'getPromptRequest') {
      const req = env['getPromptRequest'] ?? {};
      const promptName: string = req['name'] ?? '';
      const argsJson: string = req['argumentsJson'] ?? '{}';
      const prompts = getRegisteredPrompts();
      const promptDef = prompts.find(p => p.name === promptName);
      if (!promptDef) {
        const resp = Envelope.create({ getPromptResponse: { description: '', messages: [{ role: 'assistant', contentJson: JSON.stringify({ type: 'text', text: `Prompt not found: ${promptName}` }) }] }, requestId });
        await transport.send(resp);
      } else {
        try {
          const args = JSON.parse(argsJson);
          let result = promptDef.handler(args);
          if (!Array.isArray(result)) result = [result];
          const messages = result.map(m => ({ role: m.role, contentJson: JSON.stringify({ type: 'text', text: m.content }) }));
          const resp = Envelope.create({ getPromptResponse: { description: '', messages }, requestId });
          await transport.send(resp);
        } catch (err: any) {
          const resp = Envelope.create({ getPromptResponse: { description: 'Error', messages: [{ role: 'assistant', contentJson: JSON.stringify({ type: 'text', text: String(err?.message ?? err) }) }] }, requestId });
          await transport.send(resp);
        }
      }
    } else if (env['msg'] === 'completionRequest') {
      const req = env['completionRequest'] ?? {};
      const handler = getCompletionHandler(req['refType'] ?? '', req['refName'] ?? '', req['argumentName'] ?? '');
      if (!handler) {
        const resp = Envelope.create({ completionResponse: { values: [], total: 0, hasMore: false }, requestId });
        await transport.send(resp);
      } else {
        try {
          const result = handler(req['argumentValue'] ?? '');
          if (Array.isArray(result)) {
            const resp = Envelope.create({ completionResponse: { values: result, total: result.length, hasMore: false }, requestId });
            await transport.send(resp);
          } else {
            const resp = Envelope.create({ completionResponse: { values: result.values, total: result.total ?? result.values.length, hasMore: result.hasMore ?? false }, requestId });
            await transport.send(resp);
          }
        } catch {
          const resp = Envelope.create({ completionResponse: { values: [], total: 0, hasMore: false }, requestId });
          await transport.send(resp);
        }
      }
    } else if (env['msg'] === 'reload') {
      // Full ESM module cache invalidation is deferred — same as Python SDK.
      // For now, acknowledge reload and re-send current tool list.
      const reloadResp = Envelope.create({
        reloadResponse: ReloadResponse.create({ success: true }),
        requestId,
      });
      await transport.send(reloadResp);
      await sendListTools(requestId);
      await sendMiddlewareRegistrations();
    }
  }
}
