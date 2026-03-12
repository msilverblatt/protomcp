import { describe, it, expect } from 'vitest';
import { ToolResult } from '../src/result.js';

describe('ToolResult', () => {
  it('creates a plain result', () => {
    const r = new ToolResult({ result: 'hello' });
    expect(r.result).toBe('hello');
    expect(r.isError).toBe(false);
    expect(r.enableTools).toBeUndefined();
    expect(r.disableTools).toBeUndefined();
    expect(r.errorCode).toBeUndefined();
    expect(r.message).toBeUndefined();
    expect(r.suggestion).toBeUndefined();
    expect(r.retryable).toBe(false);
  });

  it('creates an error result', () => {
    const r = new ToolResult({
      result: 'failed',
      isError: true,
      errorCode: 'NOT_FOUND',
      message: 'Resource not found',
      suggestion: 'Check the ID',
      retryable: false,
    });
    expect(r.isError).toBe(true);
    expect(r.errorCode).toBe('NOT_FOUND');
    expect(r.message).toBe('Resource not found');
    expect(r.suggestion).toBe('Check the ID');
    expect(r.retryable).toBe(false);
  });

  it('supports enable/disable tools', () => {
    const r = new ToolResult({
      result: 'ok',
      enableTools: ['tool_a'],
      disableTools: ['tool_b'],
    });
    expect(r.enableTools).toEqual(['tool_a']);
    expect(r.disableTools).toEqual(['tool_b']);
  });
});
