# examples/python/full_showcase.py
# Full-featured protomcp demo — multiple tools showcasing the complete API.
# Run: pmcp dev examples/python/full_showcase.py

import json
import time
from dataclasses import dataclass
from protomcp import tool, ToolResult, ToolContext, log
from protomcp import tool_manager

# --- Tool 1: Structured output with output schema ---

@dataclass
class WeatherData:
    location: str
    temperature_f: float
    conditions: str
    humidity: int

@tool(
    "Get current weather for a location",
    output_type=WeatherData,
    read_only=True,
    title="Weather Lookup",
)
def get_weather(location: str) -> ToolResult:
    log.info(f"Weather lookup for {location}")
    # Simulated weather data
    data = WeatherData(
        location=location,
        temperature_f=72.5,
        conditions="Partly cloudy",
        humidity=45,
    )
    return ToolResult(result=json.dumps({
        "location": data.location,
        "temperature_f": data.temperature_f,
        "conditions": data.conditions,
        "humidity": data.humidity,
    }))

# --- Tool 2: Long-running operation with progress ---

@tool(
    "Analyze a dataset (simulated long-running task)",
    title="Dataset Analyzer",
    idempotent=True,
    task_support=True,
)
def analyze_dataset(ctx: ToolContext, dataset_name: str, depth: str = "basic") -> ToolResult:
    log.info(f"Starting analysis of {dataset_name} at depth={depth}")
    steps = 10

    for i in range(steps):
        if ctx.is_cancelled():
            log.warning(f"Analysis cancelled at step {i}/{steps}")
            return ToolResult(
                result=f"Analysis cancelled at step {i}/{steps}",
                is_error=True,
                error_code="CANCELLED",
                retryable=True,
            )
        ctx.report_progress(i, steps, f"Analyzing step {i+1}/{steps}...")
        time.sleep(0.1)  # Simulate work

    ctx.report_progress(steps, steps, "Analysis complete")
    log.info("Analysis finished successfully")
    return ToolResult(result=json.dumps({
        "dataset": dataset_name,
        "depth": depth,
        "rows_analyzed": 15000,
        "anomalies_found": 3,
        "summary": "Dataset is healthy with 3 minor anomalies detected.",
    }))

# --- Tool 3: Dynamic tool list management ---

@tool(
    "Enable or disable tools at runtime",
    title="Tool Manager",
    destructive=True,
)
def manage_tools(action: str, tool_names: str) -> ToolResult:
    names = [n.strip() for n in tool_names.split(",")]
    log.info(f"manage_tools: action={action}, names={names}")

    if action == "enable":
        active = tool_manager.enable(names)
    elif action == "disable":
        active = tool_manager.disable(names)
    elif action == "list":
        active = tool_manager.get_active_tools()
    else:
        return ToolResult(
            result=f"Unknown action: {action}",
            is_error=True,
            error_code="INVALID_ACTION",
            suggestion="Use 'enable', 'disable', or 'list'",
        )

    return ToolResult(result=json.dumps({"active_tools": active}))

# --- Tool 4: Demonstrates error handling and logging levels ---

@tool(
    "Validate data against a schema (demonstrates error handling)",
    title="Data Validator",
    read_only=True,
    idempotent=True,
)
def validate_data(data_json: str, strict: bool = False) -> ToolResult:
    log.debug("Starting validation")

    try:
        data = json.loads(data_json)
    except json.JSONDecodeError as e:
        log.error(f"Invalid JSON: {e}")
        return ToolResult(
            result=f"Invalid JSON: {e}",
            is_error=True,
            error_code="PARSE_ERROR",
            message="The input is not valid JSON",
            suggestion="Check for syntax errors and try again",
            retryable=True,
        )

    issues = []
    if not isinstance(data, dict):
        issues.append("Root must be an object")
    elif "name" not in data:
        issues.append("Missing required field: name")

    if strict and isinstance(data, dict):
        allowed = {"name", "value", "tags"}
        extra = set(data.keys()) - allowed
        if extra:
            issues.append(f"Unknown fields: {', '.join(extra)}")

    if issues:
        log.warning(f"Validation failed: {issues}")
        return ToolResult(
            result=json.dumps({"valid": False, "issues": issues}),
            is_error=True,
            error_code="VALIDATION_FAILED",
        )

    log.info("Validation passed")
    return ToolResult(result=json.dumps({"valid": True, "issues": []}))


if __name__ == "__main__":
    from protomcp.runner import run
    run()
