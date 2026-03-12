export interface ToolResultOptions {
  result?: string;
  isError?: boolean;
  enableTools?: string[];
  disableTools?: string[];
  errorCode?: string;
  message?: string;
  suggestion?: string;
  retryable?: boolean;
}

export class ToolResult {
  result: string;
  isError: boolean;
  enableTools?: string[];
  disableTools?: string[];
  errorCode?: string;
  message?: string;
  suggestion?: string;
  retryable: boolean;

  constructor(options: ToolResultOptions = {}) {
    this.result = options.result ?? '';
    this.isError = options.isError ?? false;
    this.enableTools = options.enableTools;
    this.disableTools = options.disableTools;
    this.errorCode = options.errorCode;
    this.message = options.message;
    this.suggestion = options.suggestion;
    this.retryable = options.retryable ?? false;
  }
}
