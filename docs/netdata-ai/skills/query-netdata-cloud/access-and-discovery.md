# Cloud Access And Discovery

Use this reference for live setup, missing identifiers or request failures. Explanation and saved-data review use
[Choose The Task](./SKILL.md#choose-the-task) without loading credentials.

## Local Token Setup

Create a token in [Netdata Cloud](https://app.netdata.cloud): open the user menu, **User Settings**, **API Tokens**,
then the add (**+**) control. Select a scope, enter a description and choose **Create**. Copy the token immediately;
it is shown once. UI placement can change; these are navigation hints, not an API contract.

Select access sufficient for the requested endpoint and role. Existing recipes mention `scope:all` (broad access)
and `scope:grafana-plugin` for data queries; do not assume the latter authorizes every endpoint or recommend full
access just to bypass an unexplained error. Exact scope-to-permission mappings require current Cloud-server evidence.
Configure the value locally as described in [Prerequisites](./SKILL.md#prerequisites), never in conversation.

The shared `agents_query_cloud` helper supplies `Authorization: Bearer <CLOUD_TOKEN>` and JSON Content-Type headers
internally. GET needs no JSON Content-Type but the helper sends it too. This is protocol notation, not an instruction
to print an authentication header. Source: [agents_query_cloud](../query-netdata-agents/scripts/_lib.sh).

## Discover Identifiers

Space IDs, room IDs and node UUIDs identify targets; they are not API tokens. Fill placeholders locally or discover
the relevant target. Do not require space/room IDs for node-only calls or for discovering spaces.

UI alternatives: in **Space Settings**, the **Info** tab exposes **Space Id**. Under **Rooms**, open a row's
**Room Settings** and copy **Room Id** from its **Room** tab. Current UI layout may differ.

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/v2/accounts/me` | GET | Confirm token access and return the user identity |
| `/api/v2/spaces` | GET | List spaces visible to the token |
| `/api/v2/spaces/{spaceID}/rooms` | GET | List rooms in a space |
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/nodes` | POST `{}` | List room nodes with metadata |

After [wrapper setup](./SKILL.md#prerequisites), capture discovery output privately instead of displaying identities.
These examples use shell variables; select only the target fields needed by the task:

```bash
SPACES_JSON="$(agents_query_cloud GET '/api/v2/spaces')"
```

Space records contain `id`, `slug`, `name`, `permissions[]` and metadata. Match the requested name or slug locally;
subsequent calls use the selected space ID.

```bash
SPACE="YOUR_SPACE_ID"
ROOMS_JSON="$(agents_query_cloud GET "/api/v2/spaces/$SPACE/rooms")"
```

```bash
ROOM="YOUR_ROOM_ID"
NODES_JSON="$(agents_query_cloud POST "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" '{}')"
```

[Node response fields](./query-nodes.md#per-node-response-fields) distinguish `nd` (node UUID used in `{nodeId}` paths)
from `mg`
(machine GUID), and document hostname, state, version, labels, hardware, OS, health and capability metadata. Do not
infer a node UUID from a machine GUID or treat the short entry-point examples as a complete response schema.

## Diagnose Failures

The helper uses curl's `--fail`; check its exit status and stderr before interpreting an absent response as no data.
Cloud deployments and endpoints can return different error envelopes. The following are checks, not exhaustive
status guarantees:

| Symptom | Check |
|---|---|
| HTTP 401 | Local configuration, selected host and whether the token is missing, malformed or revoked; replace only when needed |
| HTTP 403 | Endpoint permissions, token scope and the user's role/access to the target; do not widen access automatically |
| HTTP 404 or HTML | Method, API version, path, target resource/ID and proxy response; a missing resource can share this status |
| HTTP 400 with error JSON | Endpoint-specific parameter and payload validation; use the returned message as evidence |
| Empty result | Request success, response shape, scope/selectors, time window and target availability |

The metrics guide records `/api/docs/` as a Swagger location. Its current availability is not established here;
neither a missing docs page nor a 404 proves that all API paths are undiscoverable. Use the selected guide and current
endpoint evidence when diagnosing an unfamiliar route.
