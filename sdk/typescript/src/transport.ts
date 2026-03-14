import * as net from 'net';
import * as path from 'path';
import * as url from 'url';
import protobuf from 'protobufjs';

const __dirname = path.dirname(url.fileURLToPath(import.meta.url));
const PROTO_PATH = path.resolve(__dirname, '../../../proto/protomcp.proto');

export class Transport {
  private socketPath: string;
  private socket: net.Socket | null = null;
  private root: protobuf.Root | null = null;
  private buffer: Buffer = Buffer.alloc(0);
  private pendingReads: Array<{ resolve: (buf: Buffer) => void; reject: (err: Error) => void; n: number }> = [];
  private closed = false;
  private closeError: Error | null = null;

  constructor(socketPath: string) {
    this.socketPath = socketPath;
  }

  async getRoot(): Promise<protobuf.Root> {
    if (!this.root) {
      this.root = await protobuf.load(PROTO_PATH);
    }
    return this.root;
  }

  async connect(): Promise<void> {
    const root = await this.getRoot();
    this.root = root;

    await new Promise<void>((resolve, reject) => {
      const sock = new net.Socket();
      sock.connect(this.socketPath, () => resolve());
      sock.on('error', reject);

      sock.on('data', (chunk: Buffer) => {
        this.buffer = Buffer.concat([this.buffer, chunk]);
        this._drainPending();
      });

      sock.on('close', () => {
        this.closed = true;
        if (!this.closeError) {
          this.closeError = new Error('socket closed');
        }
        this._drainPending();
      });

      sock.on('error', (err) => {
        this.closed = true;
        this.closeError = err;
        this._drainPending();
      });

      this.socket = sock;
    });
  }

  private _drainPending(): void {
    while (this.pendingReads.length > 0) {
      const { resolve, reject, n } = this.pendingReads[0];
      if (this.buffer.length >= n) {
        const data = this.buffer.subarray(0, n);
        this.buffer = this.buffer.subarray(n);
        this.pendingReads.shift();
        resolve(Buffer.from(data));
      } else if (this.closed) {
        this.pendingReads.shift();
        reject(this.closeError ?? new Error('socket closed'));
      } else {
        break;
      }
    }
  }

  private _readExactly(n: number): Promise<Buffer> {
    return new Promise<Buffer>((resolve, reject) => {
      // Check if we already have enough data
      if (this.buffer.length >= n) {
        const data = this.buffer.subarray(0, n);
        this.buffer = this.buffer.subarray(n);
        resolve(Buffer.from(data));
        return;
      }
      if (this.closed) {
        reject(this.closeError ?? new Error('socket closed'));
        return;
      }
      this.pendingReads.push({ resolve, reject, n });
    });
  }

  async send(envelope: protobuf.Message): Promise<void> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');
    const data = Buffer.from(Envelope.encode(envelope).finish());
    const length = Buffer.alloc(4);
    length.writeUInt32BE(data.length, 0);
    const frame = Buffer.concat([length, data]);
    await new Promise<void>((resolve, reject) => {
      this.socket!.write(frame, (err) => {
        if (err) reject(err);
        else resolve();
      });
    });
  }

  async sendChunked(requestId: string, fieldName: string, data: Buffer, chunkSize: number = 65536): Promise<void> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');
    const StreamHeader = root.lookupType('protomcp.StreamHeader');
    const StreamChunk = root.lookupType('protomcp.StreamChunk');

    // Send header
    const header = Envelope.create({
      requestId,
      streamHeader: StreamHeader.create({
        fieldName,
        totalSize: data.length,
        chunkSize,
      }),
    });
    await this.send(header);

    // Send chunks
    let offset = 0;
    while (offset < data.length) {
      const end = Math.min(offset + chunkSize, data.length);
      const isFinal = end >= data.length;
      const chunk = Envelope.create({
        requestId,
        streamChunk: StreamChunk.create({
          data: data.subarray(offset, end),
          final: isFinal,
        }),
      });
      await this.send(chunk);
      offset = end;
    }
  }

  async sendRaw(requestId: string, fieldName: string, data: Buffer): Promise<void> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');
    const RawHeader = root.lookupType('protomcp.RawHeader');

    let compression = '';
    let uncompressedSize = 0;
    const threshold = parseInt(process.env.PROTOMCP_COMPRESS_THRESHOLD || '65536', 10);
    if (data.length > threshold) {
      const { compressSync } = await import('fzstd');
      uncompressedSize = data.length;
      data = Buffer.from(compressSync(data));
      compression = 'zstd';
    }

    const header = Envelope.create({
      rawHeader: RawHeader.create({
        requestId,
        fieldName,
        size: data.length,
        compression,
        uncompressedSize,
      }),
    });

    // Encode the protobuf header
    const headerBytes = Buffer.from(Envelope.encode(header).finish());
    const lengthBuf = Buffer.alloc(4);
    lengthBuf.writeUInt32BE(headerBytes.length, 0);

    // Write header frame + raw payload in one call
    const frame = Buffer.concat([lengthBuf, headerBytes, data]);
    await new Promise<void>((resolve, reject) => {
      this.socket!.write(frame, (err) => {
        if (err) reject(err);
        else resolve();
      });
    });
  }

  async recv(): Promise<Record<string, any>> {
    const root = await this.getRoot();
    const Envelope = root.lookupType('protomcp.Envelope');

    const lengthBuf = await this._readExactly(4);
    const length = lengthBuf.readUInt32BE(0);
    const data = await this._readExactly(length);
    const decoded = Envelope.decode(data);
    // Get the oneof discriminator before converting to plain object
    const msgField = (decoded as any)['msg'] as string | undefined;
    const obj = Envelope.toObject(decoded, { longs: Number, enums: String, defaults: false }) as Record<string, any>;
    if (msgField) {
      obj['msg'] = msgField;
    }
    return obj;
  }

  async close(): Promise<void> {
    if (this.socket) {
      this.socket.destroy();
      this.socket = null;
    }
  }
}
