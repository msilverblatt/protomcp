from dataclasses import dataclass
from typing import Optional

@dataclass
class ToolResult:
    result: str = ""
    is_error: bool = False
    enable_tools: Optional[list[str]] = None
    disable_tools: Optional[list[str]] = None
    error_code: Optional[str] = None
    message: Optional[str] = None
    suggestion: Optional[str] = None
    retryable: bool = False
