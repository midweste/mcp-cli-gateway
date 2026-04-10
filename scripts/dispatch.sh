#!/usr/bin/env bash
# dispatch.sh — CLI wrapper for the MCP Gemini Gateway.
# Talks directly to the gateway binary over stdio using the MCP JSON-RPC protocol.
#
# Usage:
#   dispatch.sh dispatch <model> <prompt> [label] [cwd]    — Run a job
#   dispatch.sh batch    <jobs-json>                        — Run multiple jobs
#   dispatch.sh status                                     — Queue/health status
#   dispatch.sh jobs                                       — Active jobs
#   dispatch.sh pacing                                     — Pacing state
#   dispatch.sh stats    [last]                             — Performance stats
#   dispatch.sh errors   [last]                             — Recent failures
#   dispatch.sh cancel   [--id N] [--model M] [--batch B]  — Cancel jobs
#   dispatch.sh retry    <id>                               — Retry a failed job
#   dispatch.sh result   <id>                               — Get job details
#
# Environment:
#   GATEWAY_BIN  — Path to the gateway binary (default: auto-detect)
#   CWD          — Working directory for dispatch (default: $PWD)

set -euo pipefail

# ── Resolve gateway binary ──
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATEWAY_BIN="${GATEWAY_BIN:-$SCRIPT_DIR/../mcp-gemini-gateway}"

if [[ ! -x "$GATEWAY_BIN" ]]; then
  echo "ERROR: gateway binary not found at $GATEWAY_BIN" >&2
  echo "  Run 'make build' first, or set GATEWAY_BIN." >&2
  exit 1
fi

# ── JSON escaping ──
json_escape() {
  python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()), end="")' <<< "$1"
}

# ── MCP protocol: send request, capture response ──
# Sends the full MCP handshake + tool call, extracts the tool result.
mcp_call() {
  local tool_name="$1"
  local arguments="$2"
  local work_dir="${CWD:-$(pwd)}"

  local init_req='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{"roots":{"listChanged":false}},"clientInfo":{"name":"dispatch-script","version":"1.0.0"}}}'
  local init_notif='{"jsonrpc":"2.0","method":"notifications/initialized"}'
  local tool_req="{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"${tool_name}\",\"arguments\":${arguments}}}"

  # The gateway reads from stdin and writes to stdout.
  # Send all three messages with newline separators, then close stdin.
  # Collect all stdout lines — the tool response is the JSON object with "id":2.
  local response
  response=$(printf '%s\n%s\n%s\n' "$init_req" "$init_notif" "$tool_req" \
    | "$GATEWAY_BIN" 2>/dev/null)

  # Extract the response for id:2 (the tool call result).
  # MCP responses are newline-delimited JSON objects.
  local exit_code=0
  echo "$response" | while IFS= read -r line; do
    echo "$line" | python3 -c '
import json, sys
try:
    obj = json.loads(sys.stdin.read())
    if obj.get("id") == 2:
        result = obj.get("result", {})
        content = result.get("content", [])
        is_error = result.get("isError", False)
        for item in content:
            if item.get("type") == "text":
                text = item["text"]
                try:
                    parsed = json.loads(text)
                    if is_error:
                        print(json.dumps({"error": parsed}, indent=2))
                    else:
                        print(json.dumps(parsed, indent=2))
                except json.JSONDecodeError:
                    if is_error:
                        print("ERROR: " + text)
                    else:
                        print(text)
                sys.exit(0)
        print(json.dumps(result, indent=2))
        sys.exit(0)
except (json.JSONDecodeError, KeyError):
    pass
sys.exit(1)
' && break
  done
}

# ── Command routing ──
CMD="${1:-help}"
shift || true

case "$CMD" in
  dispatch)
    MODEL="${1:-fast}"
    PROMPT="${2:?Usage: dispatch.sh dispatch <model> <prompt> [label] [cwd]}"
    LABEL="${3:-cli-dispatch}"
    CWD="${4:-$(pwd)}"
    ESCAPED_PROMPT=$(json_escape "$PROMPT")
    mcp_call "gateway_dispatch" \
      "{\"model\":\"$MODEL\",\"prompt\":$ESCAPED_PROMPT,\"label\":\"$LABEL\",\"cwd\":\"$CWD\"}"
    ;;

  batch)
    JOBS_JSON="${1:?Usage: dispatch.sh batch '<jobs-json>'}"
    mcp_call "gateway_batch_dispatch" "{\"jobs\":$JOBS_JSON}"
    ;;

  status)
    mcp_call "gateway_status" "{}"
    ;;

  jobs)
    mcp_call "gateway_jobs" "{}"
    ;;

  pacing)
    mcp_call "gateway_pacing" "{}"
    ;;

  stats)
    LAST="${1:-}"
    if [[ -n "$LAST" ]]; then
      mcp_call "gateway_stats" "{\"last\":\"$LAST\"}"
    else
      mcp_call "gateway_stats" "{}"
    fi
    ;;

  errors)
    LAST="${1:-}"
    if [[ -n "$LAST" ]]; then
      mcp_call "gateway_errors" "{\"last\":\"$LAST\"}"
    else
      mcp_call "gateway_errors" "{}"
    fi
    ;;

  cancel)
    ARGS="{}"
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --id)     ARGS="{\"id\":\"$2\"}"; shift 2 ;;
        --model)  ARGS="{\"model\":\"$2\"}"; shift 2 ;;
        --batch)  ARGS="{\"batch_id\":\"$2\"}"; shift 2 ;;
        *)        echo "Unknown cancel flag: $1" >&2; exit 1 ;;
      esac
    done
    mcp_call "gateway_cancel" "$ARGS"
    ;;

  retry)
    ID="${1:?Usage: dispatch.sh retry <id>}"
    mcp_call "gateway_retry" "{\"id\":$ID}"
    ;;

  result)
    ID="${1:?Usage: dispatch.sh result <id>}"
    mcp_call "gateway_result" "{\"id\":$ID}"
    ;;

  help|--help|-h)
    head -18 "$0" | tail -16 | sed 's/^# \?//'
    ;;

  *)
    echo "Unknown command: $CMD" >&2
    echo "Run 'dispatch.sh help' for usage." >&2
    exit 1
    ;;
esac
