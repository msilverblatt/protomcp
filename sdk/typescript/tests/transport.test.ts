import { describe, it, expect, afterEach } from 'vitest';
import * as net from 'net';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';
import { Transport } from '../src/transport.js';

function tempSocketPath(): string {
  return path.join(os.tmpdir(), `protomcp-test-${process.pid}-${Date.now()}.sock`);
}

describe('Transport', () => {
  const sockets: string[] = [];
  const servers: net.Server[] = [];

  afterEach(() => {
    for (const s of servers) s.close();
    for (const p of sockets) {
      try { fs.unlinkSync(p); } catch {}
    }
    servers.length = 0;
    sockets.length = 0;
  });

  it('sends and receives a length-prefixed envelope', async () => {
    const sockPath = tempSocketPath();
    sockets.push(sockPath);

    const received: Buffer[] = [];

    await new Promise<void>((resolve, reject) => {
      const server = net.createServer((conn) => {
        conn.on('data', (chunk) => {
          received.push(chunk);
          // Echo it back
          conn.write(chunk);
        });
        conn.on('error', reject);
      });
      server.on('error', reject);
      servers.push(server);
      server.listen(sockPath, resolve);
    });

    const transport = new Transport(sockPath);
    await transport.connect();

    // Build a minimal envelope: just a reload_response with success=true
    // We'll manually craft the protobuf bytes using protobufjs
    const proto = await transport.getRoot();
    const Envelope = proto.lookupType('protomcp.Envelope');
    const ReloadResponse = proto.lookupType('protomcp.ReloadResponse');

    const env = Envelope.create({
      reloadResponse: ReloadResponse.create({ success: true }),
      requestId: 'test-123',
    });

    await transport.send(env);
    const recvd = await transport.recv();

    expect(recvd.requestId).toBe('test-123');
    // reloadResponse is field 4 in the oneof
    expect((recvd as any).msg).toBe('reloadResponse');

    await transport.close();
  });

  it('throws on socket closed', async () => {
    const sockPath = tempSocketPath();
    sockets.push(sockPath);

    await new Promise<void>((resolve, reject) => {
      const server = net.createServer((conn) => {
        // Immediately close the connection
        conn.destroy();
      });
      server.on('error', reject);
      servers.push(server);
      server.listen(sockPath, resolve);
    });

    const transport = new Transport(sockPath);
    await transport.connect();

    await expect(transport.recv()).rejects.toThrow('socket closed');
    await transport.close();
  });
});
