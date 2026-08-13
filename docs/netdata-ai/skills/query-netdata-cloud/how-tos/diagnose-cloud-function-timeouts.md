# Diagnose Cloud Function timeouts

## Question

How can an operator distinguish a slow Windows Function from an ACLK/Cloud transport timeout?

## Inputs

- A reachable agent address for the direct timing probe.
- The agent node UUID and Cloud credentials loaded by the token-safe wrapper environment.

## Steps

1. Check the local/direct Function path and record only status and elapsed time:

   ```bash
   curl --silent --show-error --max-time 45 -o /dev/null \
     -w 'status=%{http_code} total=%{time_total}s bytes=%{size_download}\n' \
     -X POST -H 'Content-Type: application/json' \
     --data '{"info":true}' \
     'http://AGENT_HOST:19999/api/v3/function?function=netdata-metrics-cardinality'
   ```

2. Load the token-safe Cloud wrapper and run the same Function request. The wrapper emits only the response body:

   ```bash
   source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
   agents_load_env
   agents_call_function --via cloud --node "$NODE_UUID" \
     --function netdata-metrics-cardinality --body '{"info":true}' \
     > .local/audits/query-netdata-agents/function-timeout-cloud.json
   jq '{status, type, errorMessage, errorMsgKey, errorCode}' \
     .local/audits/query-netdata-agents/function-timeout-cloud.json
   ```

## Output

- A fast direct HTTP 200 with a Cloud timeout indicates an ACLK/Cloud transport problem.
- A slow or failed direct request indicates the Function or its plugin is the bottleneck.
- Keep raw Cloud responses under `.local/audits/` and report only sanitized status and error fields.

## Notes / gotchas

- Cloud Function calls use the agent's ACLK HTTP tunnel and may use a different timeout than direct HTTP.
- Do not copy bearer tokens, Cloud tokens, node UUIDs, claim IDs, hostnames, or raw Function rows into durable artifacts.

## Source guides

- [Generic Cloud Function invocation](../query-functions.md)
- [Direct-agent query workflow](../../query-netdata-agents/SKILL.md)
