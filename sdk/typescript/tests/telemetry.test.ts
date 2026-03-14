import { describe, it, expect, beforeEach } from 'vitest';
import { telemetrySink, emitTelemetry, clearTelemetrySinks, type ToolCallEvent } from '../src/telemetry.js';

beforeEach(() => {
  clearTelemetrySinks();
});

describe('telemetry', () => {
  it('emits events to registered sinks', () => {
    const events: ToolCallEvent[] = [];
    telemetrySink((event) => events.push(event));

    emitTelemetry({ toolName: 'test', action: 'run', phase: 'start', args: { x: 1 } });

    expect(events).toHaveLength(1);
    expect(events[0].toolName).toBe('test');
    expect(events[0].phase).toBe('start');
    expect(events[0].args).toEqual({ x: 1 });
  });

  it('emits to multiple sinks', () => {
    const events1: ToolCallEvent[] = [];
    const events2: ToolCallEvent[] = [];
    telemetrySink((event) => events1.push(event));
    telemetrySink((event) => events2.push(event));

    emitTelemetry({ toolName: 'tool', action: '', phase: 'success', args: {} });

    expect(events1).toHaveLength(1);
    expect(events2).toHaveLength(1);
  });

  it('is fail-safe: does not propagate sink errors', () => {
    telemetrySink(() => { throw new Error('boom'); });

    const events: ToolCallEvent[] = [];
    telemetrySink((event) => events.push(event));

    // Should not throw
    emitTelemetry({ toolName: 'tool', action: '', phase: 'start', args: {} });

    // Second sink should still receive the event
    expect(events).toHaveLength(1);
  });

  it('includes optional fields', () => {
    const events: ToolCallEvent[] = [];
    telemetrySink((event) => events.push(event));

    const err = new Error('fail');
    emitTelemetry({
      toolName: 'tool',
      action: 'do',
      phase: 'error',
      args: {},
      error: err,
      durationMs: 150,
      result: 'partial',
    });

    expect(events[0].error).toBe(err);
    expect(events[0].durationMs).toBe(150);
    expect(events[0].result).toBe('partial');
  });

  it('clears sinks', () => {
    const events: ToolCallEvent[] = [];
    telemetrySink((event) => events.push(event));

    clearTelemetrySinks();
    emitTelemetry({ toolName: 'tool', action: '', phase: 'start', args: {} });

    expect(events).toHaveLength(0);
  });
});
