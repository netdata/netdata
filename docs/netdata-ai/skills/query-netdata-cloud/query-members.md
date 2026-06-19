# List members via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.

Members are Cloud-only. There is no agent-side equivalent.

---

## Endpoint

`GET /api/v2/spaces/{spaceID}/members` -- list members of a space.

## Use the wrapper

```bash
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env

# All members of a space.
agents_query_cloud GET "/api/v2/spaces/$SPACE/members"
```

## Per-member response fields

Verified live -- response is a JSON array, each entry:

| Field | Description |
|---|---|
| `memberID` | UUID identifying the user's membership in this space (NOT the user's account id) |
| `accountID` | UUID identifying the user's Cloud account |
| `name` | Display name |
| `email` | Email |
| `avatarURL` | Avatar image URL (may be empty) |
| `role` | One of: `admin`, `manager`, `troubleshooter`, `observer`, `member`, `billing`, ... |
| `joinMethod` | How they joined: `invite`, `auto`, `sso`, ... |
| `joinedAt` | RFC3339 timestamp |
| `deactivated` | `true` if the membership has been deactivated (cannot view space until reactivated) |

## Common patterns

```bash
# Members by role.
agents_query_cloud GET "/api/v2/spaces/$SPACE/members" \
  | jq -r 'group_by(.role) | map({(.[0].role): length}) | add'

# Active admins of a space.
agents_query_cloud GET "/api/v2/spaces/$SPACE/members" \
  | jq -r '.[] | select(.role=="admin" and (.deactivated|not)) | .name'

# Resolve a user's display name from an accountID found elsewhere
# (e.g. in an alert-config audit field).
TARGET_ACCOUNT="<account-uuid>"
agents_query_cloud GET "/api/v2/spaces/$SPACE/members" \
  | jq -r --arg id "$TARGET_ACCOUNT" '.[] | select(.accountID==$id) | "\(.name) <\(.email)>"'
```

## Limits and gotchas

- **Member visibility depends on the caller's role.** Observers
  may not see all members. The response is filtered server-side
  by what the caller is permitted to see.
- **`memberID` vs `accountID`**: the former is per-space (one
  user gets a different `memberID` in each space); the latter is
  the user's global Cloud account id and stays constant across
  spaces. Use `accountID` when correlating across spaces.
- **Deactivated members still appear in the list** with
  `deactivated:true`. Filter explicitly to exclude them.
- **Email and name are personal data.** Treat the response as
  semi-sensitive: do not paste raw response bodies into
  committed artifacts. Direct working output to
  `<repo>/.local/audits/...` (gitignored).

## See also

- [query-rooms.md](./query-rooms.md) -- room-level membership
  (`isMember`, `permissions[]`, `member_count`).
- [query-feed.md](./query-feed.md) -- audit-feed events include
  `user-create`, `space-user-added`, `space-user-removed`,
  `user-space-permissions-changed`, `room-user-added`, ... use
  the feed to track membership changes over time.
