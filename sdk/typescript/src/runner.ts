import * as process from 'process';
import { Transport } from './transport.js';
import { getRegisteredTools } from './tool.js';
import { ToolResult } from './result.js';
import { ToolContext } from './context.js';
import { toolManager } from './manager.js';
import { getRegisteredMiddleware, type MiddlewareDef } from './middleware.js';

export async function run(): Promise<void> {
  const socketPath = process.env['PROTOMCP_SOCKET'];
  if (!socketPath) {
    process.stderr.write("PROTOMCP_SOCKET not set — run via 'pmcp dev'\n");
    process.exit(1);
  }

  const transport = new Transport(socketPath);
  await transport.connect();
  toolManager._init(transport as any);

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
        const args = argumentsJson ? JSON.parse(argumentsJson) : {};
        const progressToken: string = req['progressToken'] ?? '';
        const ctx = new ToolContext(progressToken, (msg: any) => transport.send(Envelope.create(msg)));
        const result = await toolDef.handler(args, ctx);

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
      } catch (err: any) {
        respMsg = CallToolResponse.create({
          isError: true,
          resultJson: JSON.stringify([{ type: 'text', text: String(err?.message ?? err) }]),
        });
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
