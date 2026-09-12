# Direct-Agent Authentication And Helper Reference

Read for live setup, bearer diagnosis or helper maintenance. Source paths below are repository-relative; `./` links
are relative to this skill. The helper owns client behavior. Agent source establishes the local server contract;
Cloud-service observations are called out separately.

## Configuration

[The helper](./scripts/_lib.sh) locates its checkout through Git. `agents_load_env` sources trusted `<repo>/.env` and
requires the two Cloud settings below. The user fills values locally; the skill does not add configuration keys.

| Existing key | Role |
|---|---|
| `NETDATA_CLOUD_TOKEN` | Long-lived Cloud REST credential used for Cloud calls and bearer minting |
| `NETDATA_CLOUD_HOSTNAME` | Cloud REST target hostname |
| `AGENT_EVENTS_HOSTNAME` | Agent-events node SSH/direct HTTP host; not the hardcoded `agent-events` journal namespace |
| `AGENT_EVENTS_NODE_ID` | Target node UUID for the Agent-events workflow |
| `AGENT_EVENTS_MACHINE_GUID` | Its machine GUID, used as bearer cache key |

The `AGENT_EVENTS_*` keys are for that particular workflow. Other Agent queries use their own locally selected target
values; they do not require Agent-events configuration. Contributor setup lives in `.agents/ENV.md` and
`.env.template`; preserve unrelated entries in the private `.env`.

Minting uses a matching `node_id`, `machine_guid`, `claim_id` tuple from one Agent. Internal `_agents_get_claim_id`
captures `/api/v3/info` and extracts `.agents[0].cloud.claim_id` into a validated caller-local output variable.
For shell-access diagnosis the claim also lives under `<netdata-prefix>/var/lib/netdata/cloud.d/claimed_id`; capture it
privately rather than printing it. `agents_netdata_prefix` probes the local system, `/opt/netdata`, then
`/usr/local/netdata` for `var/lib/netdata` or `etc/netdata`, returning the first matching prefix or empty. It does not
inspect a remote host; do not substitute the workstation's result for an Agent's installation path.

## Protection And Headers

The Agent's `/api/v3/info` API has no bearer requirement, but service, network and proxy availability still matter.
An authorization denial without a signed identity uses 412 and may say
`You need to be authorized to access this resource`; an identity lacking required permissions uses 403.
An ACL denial has a separate path. Interpret the particular endpoint's permissions, rather than using 412 to conclude
that every path needs a bearer. Owners: `src/web/api/web_api_v3.c`, `src/web/api/web_api.c`,
`src/web/server/web_client.c` and `src/libnetdata/user-auth/http-access.h`.

For a requested auth probe, choose an available read-only Function, set `NODE_UUID`, `AGENT_HOST` and `AGENT_FUNCTION`,
using `AGENT_HOST` as the complete `host:port`, and suppress the response body:

```bash
agent_function_path="/host/${NODE_UUID:?set node}/api/v3/function?function=${AGENT_FUNCTION:?set Function}"
curl -sS --max-time 10 -o /dev/null -w '%{http_code}\n' -X POST \
  -H 'Content-Type: application/json' \
  "http://${AGENT_HOST:?set host:port}${agent_function_path}" \
  -d '{"info":true}'
```

A 200 means that specific unauthenticated request succeeded. `info:true` semantics depend on the Function; do not
assume an arbitrary Function supports it or is read-only. Normal bearer-protected execution SHOULD use the managed
Cloud-token flow in [Safe Execution](./SKILL.md#safe-execution), not manual credential handling.

Canonical protocol headers are `Authorization: Bearer <CLOUD_TOKEN>` to Cloud and
`X-Netdata-Auth: Bearer <AGENT_BEARER>` to the Agent. `src/web/api/http_header.c` also accepts `Authorization` as a
compatibility alias for an **Agent bearer**. That does not make a Cloud REST token an Agent bearer.
The direct helper uses HTTP: bearer authentication does not encrypt the connection. Transport confidentiality
depends on the network or tunnel protecting that connection.

## Mint And Cache Lifecycle

Internal `_agents_mint_bearer_json` issues Cloud `GET /api/v2/bearer_get_token` with `node_id`, `machine_guid` and
`claim_id`. Its stdout is credential-bearing JSON: the resolver captures it locally. It MUST NOT be called directly
with assistant-visible output. Use the public direct wrapper to resolve/mint/cache internally.

The Agent response owner is `src/web/api/v2/api_v2_bearer.c:bearer_get_token_json_response`:

| Field | Meaning |
|---|---|
| `token` | UUID bearer for the Agent auth header |
| `expiration` | Expiration timestamp; Agent source emits seconds, while the helper tolerates milliseconds |
| `bearer_protection` | Current protection state; a valid authorized token can also be used on an unprotected Agent |
| `mg` | Machine GUID |
| `status` | Response status |

`src/web/api/http_auth.c` defines a 24-hour lifetime for newly created tokens. Minting can reuse a matching token with
more than two hours left, so a mint response does not promise a new full lifetime. Use the returned expiry.

The resolver stores the response plus `_cached_at` under
`.local/audits/query-netdata-agents/bearers/<machine_guid>.json`, gitignored with file mode 0600; it attempts directory
mode 0700. The machine GUID must be a UUID, and existing cache-directory or cache-entry symlinks are rejected.
It returns the bearer through a validated caller-local output variable, not displayed stdout.

Current client policy, owned by `_agents_exp_to_seconds` and `_agents_resolve_bearer`:

- Normalize integer expirations greater than 10^12 from milliseconds to seconds; empty/invalid values become zero.
- Reuse a cached token with a real expiry only when more than 3600 seconds remain; equality also triggers refresh.
- If expiry normalizes to zero, reuse it only when `_cached_at` is positive and less than 7200 seconds old.
  This fallback is client policy, not a claim that the server issues three-hour tokens.
- A failed mint-response token check removes the cache entry and reports failure without displaying the response.
  Other failures, such as claim lookup or transport failure, need not clear the existing file.

The direct wrapper always resolves a bearer before the request. There is no automatic 412 detection, invalid-bearer
retry or fallback to Cloud. A direct call during a Cloud outage therefore needs a cache entry accepted by this policy;
open unauthenticated API access is a separate route. This client policy is not asserted to mirror frontend code.

Historical Cloud troubleshooting knowledge associates mint access with `PermissionSpaceRead`, node reachability and
a 400 response for stale nodes. These exact Cloud-side gates/statuses have not been revalidated against a current
Cloud-server owner here. Treat them as checks to investigate, not a guarantee established by local Agent source.
Likewise, milliseconds in Cloud responses are a compatibility assumption of the helper.

## Helper Interfaces

The source owns exact arguments and error behavior. Both request wrappers write the unfiltered response body to
stdout and masked command arguments to stderr. Load configuration first or supply the required environment locally.

| Public function | Contract |
|---|---|
| `agents_load_env` | Source repository `.env`; require Cloud token and hostname |
| `agents_repo_root` | Locate this checkout |
| `agents_audit_dir` | Create and return `.local/audits/query-netdata-agents/` |
| `agents_netdata_prefix` | Probe local install-prefix candidates |
| `agents_query_cloud METHOD PATH [BODY]` | HTTPS Cloud REST call with internal auth; optional JSON body |
| `agents_query_agent --node N --host H --machine-guid M METHOD PATH [BODY]` | Direct HTTP call under `/host/N`, resolving the bearer internally |
| `agents_call_function --via cloud\|agent --node N --function F [--body J]` | Function POST; default Cloud and `{"info":true}`; direct additionally needs `--host` and `--machine-guid` |
| `agents_run` / `agents_run_read` | Execute arguments with masked log; advanced callers MAY use them directly when appropriate |
| `agents_selftest_no_token_leak` | Legacy offline check of Cloud dry-run logging, masking and internal output-variable handling |

`agents_run` skips its command when `AGENTS_DRY_RUN=1`; `agents_run_read` still executes. Direct wrappers resolve
bearers before `agents_run`, so **dry-run is not a guarantee of no network or cache writes**. Never use it as a
substitute for read-only inspection of a script.

Internal `_agents_*` functions are implementation details, not assistant entry points. `_agents_get_claim_id` and
`_agents_resolve_bearer` use validated output names; `_agents_mint_bearer_json` is the explicit stdout exception and
MUST be captured by its caller. `_agents_set_outvar` uses `printf -v`, not evaluation of returned data.
`_agents_log_masked` masks known request-auth and identity argument forms; it is not a general-purpose content filter.

For safe local maintenance checks, use a fresh Bash process; no real `.env` or endpoint is used:

```bash
bash -c 'source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh; agents_selftest_no_token_leak'
python3 docs/netdata-ai/skills/query-netdata-agents/scripts/test_wrappers.py
```

The legacy self-test is limited to its listed cases. The wrapper tests use fake curl and a disposable repository to
exercise Cloud body forwarding, direct mint/cache, error confidentiality and no-fallback behavior. Neither test proves
arbitrary API responses contain no secrets or that a current live Cloud/Agent deployment accepts the request.
