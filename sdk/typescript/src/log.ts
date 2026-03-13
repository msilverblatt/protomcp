const LEVELS = ['debug', 'info', 'notice', 'warning', 'error', 'critical', 'alert', 'emergency'] as const;
type LogLevel = typeof LEVELS[number];

export class ServerLogger {
  constructor(
    private sendFn: (msg: any) => void,
    private name?: string,
  ) {}

  private _log(level: LogLevel, message: string, data?: Record<string, unknown>) {
    this.sendFn({
      log: {
        level,
        logger: this.name ?? '',
        dataJson: JSON.stringify(data ?? { message }),
      },
    });
  }

  debug(msg: string, data?: Record<string, unknown>) { this._log('debug', msg, data); }
  info(msg: string, data?: Record<string, unknown>) { this._log('info', msg, data); }
  notice(msg: string, data?: Record<string, unknown>) { this._log('notice', msg, data); }
  warning(msg: string, data?: Record<string, unknown>) { this._log('warning', msg, data); }
  error(msg: string, data?: Record<string, unknown>) { this._log('error', msg, data); }
  critical(msg: string, data?: Record<string, unknown>) { this._log('critical', msg, data); }
  alert(msg: string, data?: Record<string, unknown>) { this._log('alert', msg, data); }
  emergency(msg: string, data?: Record<string, unknown>) { this._log('emergency', msg, data); }
}
