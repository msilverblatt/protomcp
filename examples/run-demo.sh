#!/usr/bin/env bash
set -euo pipefail

# examples/run-demo.sh
# Runs each example through pmcp, demonstrating the MCP protocol interaction.
# Requires: pmcp installed and on PATH

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASSED=0
FAILED=0

if ! command -v pmcp &> /dev/null; then
    echo -e "${RED}Error: pmcp not found. Install it first: brew install protomcp/tap/protomcp${NC}"
    exit 1
fi

# Send a JSON-RPC request to a pmcp process and capture the response.
# Usage: run_example <label> <file> <tool_name> <args_json>
run_example() {
    local label="$1"
    local file="$2"
    local tool_name="$3"
    local args_json="$4"

    echo -e "\n${CYAN}━━━ ${label} ━━━${NC}"
    echo -e "  File: ${file}"
    echo -e "  Tool: ${tool_name}"
    echo -e "  Args: ${args_json}"

    # Build JSON-RPC messages
    local init_req='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"demo","version":"1.0"}}}'
    local init_notif='{"jsonrpc":"2.0","method":"notifications/initialized"}'
    local list_req='{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
    local call_req="{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"${tool_name}\",\"arguments\":${args_json}}}"

    # Send all messages and capture output
    local output
    output=$(printf '%s\n%s\n%s\n%s\n' "$init_req" "$init_notif" "$list_req" "$call_req" | \
        timeout 10 pmcp dev "$file" 2>/dev/null || true)

    if [ -z "$output" ]; then
        echo -e "  ${RED}✗ No response${NC}"
        FAILED=$((FAILED + 1))
        return
    fi

    # Extract the tools/call response (id: 3)
    local result
    result=$(echo "$output" | grep '"id":3' | head -1 || true)

    if [ -n "$result" ]; then
        echo -e "  ${GREEN}✓ Result:${NC}"
        echo "    $result" | python3 -m json.tool 2>/dev/null || echo "    $result"
        PASSED=$((PASSED + 1))
    else
        echo -e "  ${YELLOW}⚠ Could not extract result (raw output below)${NC}"
        echo "$output" | head -5
        FAILED=$((FAILED + 1))
    fi
}

echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
echo -e "${CYAN}║       protomcp Demo Runner           ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"

# Python examples
run_example "Python: Basic (add)" \
    "${SCRIPT_DIR}/python/basic.py" \
    "add" \
    '{"a": 5, "b": 3}'

run_example "Python: Real-World (file search)" \
    "${SCRIPT_DIR}/python/real_world.py" \
    "search_files" \
    "{\"directory\": \"${SCRIPT_DIR}\", \"pattern\": \"*.py\"}"

run_example "Python: Full Showcase (weather)" \
    "${SCRIPT_DIR}/python/full_showcase.py" \
    "get_weather" \
    '{"location": "San Francisco"}'

run_example "Python: Full Showcase (validate)" \
    "${SCRIPT_DIR}/python/full_showcase.py" \
    "validate_data" \
    '{"data_json": "{\"name\": \"test\", \"value\": 42}", "strict": true}'

# TypeScript examples
run_example "TypeScript: Basic (add)" \
    "${SCRIPT_DIR}/typescript/basic.ts" \
    "add" \
    '{"a": 5, "b": 3}'

run_example "TypeScript: Real-World (file search)" \
    "${SCRIPT_DIR}/typescript/real-world.ts" \
    "search_files" \
    "{\"directory\": \"${SCRIPT_DIR}\", \"pattern\": \"*.ts\"}"

run_example "TypeScript: Full Showcase (weather)" \
    "${SCRIPT_DIR}/typescript/full-showcase.ts" \
    "get_weather" \
    '{"location": "San Francisco"}'

run_example "TypeScript: Full Showcase (validate)" \
    "${SCRIPT_DIR}/typescript/full-showcase.ts" \
    "validate_data" \
    '{"data_json": "{\"name\": \"test\", \"value\": 42}", "strict": true}'

# Summary
echo -e "\n${CYAN}━━━ Summary ━━━${NC}"
echo -e "  ${GREEN}Passed: ${PASSED}${NC}"
if [ "$FAILED" -gt 0 ]; then
    echo -e "  ${RED}Failed: ${FAILED}${NC}"
else
    echo -e "  Failed: 0"
fi
