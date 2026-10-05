# Diagnose Cloud Function timeouts

## Question

How can an operator distinguish a slow Windows Function from an ACLK/Cloud transport timeout?

## Inputs

- A reachable agent address (`AGENT_URL`, or `AGENT_HOST` with an optional port).
- The agent node UUID (`NODE_UUID`) and machine GUID (`MACHINE_GUID`) must be set in the shell before running this guide. The token-safe wrapper loads Cloud credentials, but does not populate these host identifiers.

## Steps

1. Derive the direct target without adding a second port when `AGENT_HOST` already
   contains one. Use the same node UUID for both probes, and record only status and
   elapsed time for the direct call:

   ```bash
   REPO_ROOT="$(git rev-parse --show-toplevel)"
   source "$REPO_ROOT/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   AUDIT_DIR="$REPO_ROOT/.local/audits/query-netdata-agents"
   mkdir -p "$AUDIT_DIR"
   if [[ -z "${AGENT_URL:-}" ]]; then
       AGENT_TARGET="${AGENT_HOST:-127.0.0.1}"
       case "$AGENT_TARGET" in
           \[*\]) AGENT_TARGET="${AGENT_TARGET}:${AGENT_PORT:-19999}" ;;
           *:*) : ;; # Preserve an already supplied port.
           *) AGENT_TARGET="${AGENT_TARGET}:${AGENT_PORT:-19999}" ;;
       esac
       AGENT_URL="http://${AGENT_TARGET}"
   fi
   AGENT_TARGET="${AGENT_URL#http://}"
   AGENT_TARGET="${AGENT_TARGET#https://}"
   AGENT_TARGET="${AGENT_TARGET%%/*}"

   # Resolve/cache Cloud credentials outside the measured interval.
   agents_call_function --via agent --node "$NODE_UUID" --host "$AGENT_TARGET" \
     --machine-guid "$MACHINE_GUID" --function 'netdata-metrics-cardinality%20info' \
     --body '{"info":true}' >/dev/null 2>&1 || true
   direct_start="$(date +%s)"
   direct_rc=0
   agents_call_function --via agent --node "$NODE_UUID" --host "$AGENT_TARGET" \
     --machine-guid "$MACHINE_GUID" --function 'netdata-metrics-cardinality%20info' \
     --body '{"info":true}' \
     > "$AUDIT_DIR/function-timeout-direct.json" || direct_rc=$?
   direct_elapsed="$(awk -v s="$direct_start" -v e="$(date +%s)" 'BEGIN { printf "%.0f", e-s }')"
   if [[ "$direct_rc" -eq 0 ]] && jq -e . "$AUDIT_DIR/function-timeout-direct.json" >/dev/null 2>&1; then
       jq --arg elapsed "${direct_elapsed}s" '{status: (.status // null), total: $elapsed}' \
         "$AUDIT_DIR/function-timeout-direct.json"
   elif [[ "$direct_rc" -ne 0 ]]; then
       jq -n --arg elapsed "${direct_elapsed}s" --arg rc "$direct_rc" \
         '{status: null, total: $elapsed, transport_error: $rc}'
   else
       jq -n --arg elapsed "${direct_elapsed}s" \
         '{status: null, total: $elapsed, response_parse_error: true}'
   fi
   ```

2. In the same shell session as Step 1, reuse the loaded token-safe wrapper, target, and credentials to run the same Function request. The wrapper emits only the response body. Keep `NODE_UUID` and `MACHINE_GUID` set in this shell:

   ```bash
   # Run this block in the same shell session as Step 1 to reuse the target and credentials.
   cloud_rc=0
   agents_call_function --via cloud --node "$NODE_UUID" \
     --function 'netdata-metrics-cardinality%20info' --body '{"info":true}' \
     > "$AUDIT_DIR/function-timeout-cloud.json" || cloud_rc=$?
   if [[ "$cloud_rc" -eq 0 ]] && jq -e . "$AUDIT_DIR/function-timeout-cloud.json" >/dev/null 2>&1; then
       jq '{status, type, errorMessage, errorMsgKey, errorCode}' \
         "$AUDIT_DIR/function-timeout-cloud.json"
   elif [[ "$cloud_rc" -ne 0 ]]; then
       jq -n --arg rc "$cloud_rc" '{status: null, transport_error: $rc}'
   else
       jq -n '{status: null, response_parse_error: true}'
   fi
   ```

## Output

- A fast successful direct response with a Cloud timeout indicates an ACLK/Cloud transport problem.
- A slow successful direct response indicates the Function or its plugin may be the bottleneck.
- A failed direct request does not establish a Function bottleneck: authentication, routing,
  and network failures must be diagnosed separately from the HTTP/curl error.
- Keep raw Cloud responses under `.local/audits/` and report only sanitized status and error fields.

## Notes / gotchas

- Cloud Function calls use the agent's ACLK HTTP tunnel and may use a different timeout than direct HTTP.
- Do not copy bearer tokens, Cloud tokens, node UUIDs, claim IDs, hostnames, or raw Function rows into durable artifacts.

## Source guides

- [Generic Cloud Function invocation](../query-functions.md)
- [Direct-agent query workflow](../../query-netdata-agents/SKILL.md)
