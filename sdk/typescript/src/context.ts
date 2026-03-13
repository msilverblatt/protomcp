export class ToolContext {
  private _cancelled = false;
  constructor(
    private readonly progressToken: string,
    private readonly sendFn: (msg: any) => void,
  ) {}

  reportProgress(progress: number, total?: number, message?: string) {
    if (!this.progressToken) return;
    this.sendFn({
      progress: {
        progressToken: this.progressToken,
        progress,
        ...(total !== undefined && { total }),
        ...(message !== undefined && { message }),
      },
    });
  }

  isCancelled(): boolean { return this._cancelled; }
  setCancelled() { this._cancelled = true; }
}
