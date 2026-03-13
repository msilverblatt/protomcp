import { describe, it, expect } from 'vitest';
import { ServerLogger } from '../src/log';

describe('ServerLogger', () => {
  it('sends log message', () => {
    const sent: any[] = [];
    const logger = new ServerLogger((msg) => sent.push(msg));
    logger.info('hello', { count: 5 });
    expect(sent).toHaveLength(1);
    expect(sent[0].log.level).toBe('info');
  });

  it('supports all RFC 5424 levels', () => {
    const sent: any[] = [];
    const logger = new ServerLogger((msg) => sent.push(msg));
    const levels = ['debug', 'info', 'notice', 'warning', 'error', 'critical', 'alert', 'emergency'] as const;
    for (const level of levels) {
      logger[level]('test');
    }
    expect(sent).toHaveLength(8);
  });
});
