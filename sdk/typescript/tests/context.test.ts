import { describe, it, expect } from 'vitest';
import { ToolContext } from '../src/context';

describe('ToolContext', () => {
  it('sends progress notification', () => {
    const sent: any[] = [];
    const ctx = new ToolContext('pt-1', (msg) => sent.push(msg));
    ctx.reportProgress(5, 10, 'Working');
    expect(sent).toHaveLength(1);
    expect(sent[0].progress.progressToken).toBe('pt-1');
  });

  it('is noop without progress token', () => {
    const sent: any[] = [];
    const ctx = new ToolContext('', (msg) => sent.push(msg));
    ctx.reportProgress(1);
    expect(sent).toHaveLength(0);
  });

  it('tracks cancellation', () => {
    const ctx = new ToolContext('', () => {});
    expect(ctx.isCancelled()).toBe(false);
    ctx.setCancelled();
    expect(ctx.isCancelled()).toBe(true);
  });
});
