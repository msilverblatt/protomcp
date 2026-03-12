import * as process from 'process';
import { Transport } from './transport.js';
import { getRegisteredTools } from './tool.js';
import { ToolResult } from './result.js';
import { toolManager } from './manager.js';

export async function run(): Promise<void> {
  const socketPath = process.env['PROTOMCP_SOCKET'];
  if (!socketPath) {
    process.stderr.write("PROTOMCP_SOCKET not set — run via 'protomcp dev'\n");
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

  async function sendListTools(requestId: string): Promise<void> {
    const tools = getRegisteredTools();
    const toolDefs = tools.map((t) =>
      ToolDefinition.create({
        name: t.name,
        description: t.description,
        inputSchemaJson: t.inputSchemaJson,
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
        const result = await toolDef.handler(args);

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

      const resp = Envelope.create({ callResult: respMsg, requestId });
      await transport.send(resp);
    } else if (env['msg'] === 'reload') {
      const reloadResp = Envelope.create({
        reloadResponse: ReloadResponse.create({ success: true }),
        requestId,
      });
      await transport.send(reloadResp);
      await sendListTools(requestId);
    }
  }
}
