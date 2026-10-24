# List rooms via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

Rooms are Cloud-only organizational units. There is no agent-side
equivalent.

---

## Endpoint

`GET /api/v2/spaces/{spaceID}/rooms` -- list all rooms the user can
see in a space.

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# All rooms in a space.
agents_query_cloud GET "/api/v2/spaces/$SPACE/rooms"
```

## Per-room response fields

Verified live -- response is a JSON array, each entry:

| Field | Description |
|---|---|
| `id` | **Room UUID.** Use this anywhere the API expects a room id. |
| `slug` | URL-safe identifier (e.g. `agent-events-r0gtre6`) |
| `name` | Human-readable name (e.g. `agent-events`) |
| `description` | Free-form description (may be `null`) |
| `private` | `true` if invitation-only |
| `untouchable` | `true` for the auto-managed "All nodes" room (cannot be deleted) |
| `node_count` | Number of nodes assigned to this room |
| `member_count` | Number of users in this room |
| `isMember` | Whether the calling user is a member |
| `silencing_state` | Notification silencing state for the calling user |
| `permissions` | Array of permission strings the caller has on the room |
| `createdAt` | RFC3339 timestamp |

## Common patterns

```bash
# Find the room id by name.
agents_query_cloud GET "/api/v2/spaces/$SPACE/rooms" \
  | jq -r --arg NAME "agent-events" '.[] | select(.name==$NAME) | .id'

# Rooms with at least one reachable node, sorted by node count.
agents_query_cloud GET "/api/v2/spaces/$SPACE/rooms" \
  | jq -r 'sort_by(-.node_count) | .[] | select(.node_count > 0) | "\(.name)\t\(.node_count)"'

# Rooms the caller can administer.
agents_query_cloud GET "/api/v2/spaces/$SPACE/rooms" \
  | jq -r '.[] | select(.permissions | index("room:Delete")) | .name'
```

## Limits and gotchas

- **The "All nodes" room is special.** It auto-includes every
  node in the space and cannot be deleted. Filter on
  `untouchable=true` if you need it specifically.
- **`node_count` and `member_count` are server-side counts** --
  no need to fetch nodes/members just to get the totals.
- **Permissions vary per user.** The same room returns different
  `permissions[]` arrays depending on the caller's role; another
  user may see fewer permissions.

## See also

- [query-nodes.md](./query-nodes.md) -- enumerate nodes in a
  specific room.
- [query-members.md](./query-members.md) -- list members of a
  space (rooms inherit space membership scoped by room
  permissions).
- [query-alerts.md](./query-alerts.md) -- silencing rules
  reference rooms by id.
