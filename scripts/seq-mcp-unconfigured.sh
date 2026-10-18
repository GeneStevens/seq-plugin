#!/bin/sh
# A stand-in MCP server over stdio for a SEQ that is not set up. $1 is the message.
# It lists one tool and answers every tool call with the message as an error, so
# whichever client started it shows the player what to fix on the first call.
# Used by seq-mcp-run.sh; not run by hand.
msg=$1
json=$(printf %s "$msg" | sed 's/\\/\\\\/g; s/"/\\"/g')
while IFS= read -r line; do
  id=$(printf %s "$line" | sed -n 's/^{.*"id" *: *\([0-9][0-9]*\).*/\1/p; s/^{.*"id" *: *\("[^"]*"\).*/\1/p' | head -1)
  [ -n "$id" ] || continue # a notification wants no answer
  case "$line" in
    *'"method":"initialize"'* | *'"method": "initialize"'*)
      ver=$(printf %s "$line" | sed -n 's/.*"protocolVersion" *: *"\([^"]*\)".*/\1/p')
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"%s","capabilities":{"tools":{}},"serverInfo":{"name":"seq-not-set-up","version":"0"}}}\n' "$id" "${ver:-2025-06-18}" ;;
    *'"method":"tools/list"'* | *'"method": "tools/list"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"look","description":"%s","inputSchema":{"type":"object","properties":{}}}]}}\n' "$id" "$json" ;;
    *'"method":"tools/call"'* | *'"method": "tools/call"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"isError":true,"content":[{"type":"text","text":"%s"}]}}\n' "$id" "$json" ;;
    *'"method":"ping"'* | *'"method": "ping"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$id" ;;
    *) printf '{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"not set up"}}\n' "$id" ;;
  esac
done
