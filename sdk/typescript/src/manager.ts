import type { Transport } from './transport.js';

interface TransportLike {
  send(envelope: any): Promise<void> | void;
  recv(): Promise<any>;
}

interface BatchOptions {
  enable?: string[];
  disable?: string[];
  allow?: string[];
  block?: string[];
}

class ToolManager {
  private transport: TransportLike | null = null;

  _init(transport: TransportLike): void {
    this.transport = transport;
  }

  _reset(): void {
    this.transport = null;
  }

  private _getTransport(): TransportLike {
    if (!this.transport) {
      throw new Error('protomcp not connected');
    }
    return this.transport;
  }

  private async _sendRecv(envelope: any): Promise<string[]> {
    const t = this._getTransport();
    await t.send(envelope);
    const resp = await t.recv();
    return resp.activeTools?.toolNames ?? [];
  }

  async enable(toolNames: string[]): Promise<string[]> {
    return this._sendRecv({
      msg: 'enableTools',
      enableTools: { toolNames },
    });
  }

  async disable(toolNames: string[]): Promise<string[]> {
    return this._sendRecv({
      msg: 'disableTools',
      disableTools: { toolNames },
    });
  }

  async setAllowed(toolNames: string[]): Promise<string[]> {
    return this._sendRecv({
      msg: 'setAllowed',
      setAllowed: { toolNames },
    });
  }

  async setBlocked(toolNames: string[]): Promise<string[]> {
    return this._sendRecv({
      msg: 'setBlocked',
      setBlocked: { toolNames },
    });
  }

  async getActiveTools(): Promise<string[]> {
    return this._sendRecv({
      msg: 'getActiveTools',
      getActiveTools: {},
    });
  }

  async batch(options: BatchOptions = {}): Promise<string[]> {
    return this._sendRecv({
      msg: 'batch',
      batch: {
        enable: options.enable ?? [],
        disable: options.disable ?? [],
        allow: options.allow ?? [],
        block: options.block ?? [],
      },
    });
  }
}

export const toolManager = new ToolManager();
