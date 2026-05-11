# Spec - Sensitive data discipline for committed artifacts

## Status

Active. Applies to every SOW, every skill (public or private), every
script, every spec, every committed doc, every commit message, and every
PR text in this repository.

## Rule

The following categories of literal values MUST NOT appear in any
committed file:

1. **IP addresses.** IPv4 or IPv6 literals to specific hosts.
   Loopback (`127.0.0.1`, `::1`) and link-local addresses used as
   loopback / link-local references are fine.
2. **Tenant-identifying hostnames.** Any hostname that uniquely
   identifies a customer, community member, internal ingestion
   host, or other non-public destination. **Public Netdata-org
   sites used in role-descriptive prose are allowed** (e.g.
   `learn.netdata.cloud` when describing the learn site,
   `netdata.cloud` when describing the marketing site). The Cloud
   REST API host is env-keyed because it's an operational target
   that scripts call.
3. **UUID-shaped identifiers.** Machine GUIDs, node UUIDs, claim
   IDs, space IDs, room IDs, agent IDs, ephemeral IDs,
   bearer-token UUIDs, tenant IDs, account IDs.
4. **Credentials.** API tokens, bearer tokens, session cookies,
   OAuth tokens, passwords, signing keys, SSH private keys,
   anything that grants access.
5. **Absolute filesystem paths to per-user or per-tenant state.**
   User home paths, the user's mirrored-repos tree, the larger
   monitoring mirror tree, or any workstation-specific opt/var
   path that identifies a particular install or user.
   **Documented Netdata default install paths
   (`/var/lib/netdata`, `/etc/netdata`, `/usr/lib/netdata`,
   `/usr/libexec/netdata`) are allowed** because they are
   open-source defaults documented across the codebase.
   Netdata can also be bundled (typically rooted at
   `/opt/netdata`), so scripts MUST autodetect the install
   prefix at runtime by probing the candidate locations and
   selecting the first one that exists. Common candidates:
   empty (system install), `/opt/netdata`, `/usr/local/netdata`.
   Scripts must NOT require a `NETDATA_PREFIX` env knob; the
   prefix is a discovered fact, not a user configuration.
6. **Real-name identifiers.** Usernames, email addresses, real
   names of community members, customers, employees,
   contributors.
7. **Tenant identifiers.** Netdata Cloud space names, room
   names, or any human-readable identifier that maps to a
   specific tenant or organization.
8. **Proprietary incident details.** Customer support
   narratives, non-public bug reports, private correspondence.

## Allowed alternatives

For every reference to a value covered above, use ONE of:

- An env-key placeholder: `${KEY_NAME}` -- the value lives in `.env`
  (gitignored). Examples:
  `ssh ${AGENT_EVENTS_HOSTNAME}` rather than the literal address;
  `${NETDATA_REPOS_DIR}/learn/ingest.js` and
  `${NETDATA_REPOS_DIR}/website/content/...` when committed skill
  content needs to point at sibling Netdata-org repositories the
  user has cloned locally. Sibling-repo file paths via
  `${NETDATA_REPOS_DIR}/...` are explicitly allowed in committed
  skill / SOW / spec content; literal workstation roots
  (`~/`, `/home/...`) are not.
- A repo-relative path: `<repo>/src/daemon/status-file.c`,
  `src/web/api/...` -- these describe locations inside this
  repository and are not leaks.
- Standard Linux/POSIX paths that carry no tenant or user identity:
  `/tmp`, `/run`, `/etc/passwd` (the file, not the contents), `/proc`,
  `/sys`. Use sparingly; prefer not to mention them at all unless the
  reference is essential.
- Generic role descriptions: "the production Netdata Cloud REST host",
  "the agent's varlib directory", "the user's repo mirror" -- without
  the actual value.
- Open-source code references: `file:line` citations into this repo
  (e.g. `src/daemon/status-file.c:988`) are fine; they describe code,
  not values.

## Required env keys

These keys MUST be defined in `<repo>/.env` (gitignored) for the
SOW family from SOW-0010 onward to function. If a SOW or script
references one and the key is unset, the script must error loudly
and exit non-zero. Values live ONLY in `.env`; this spec lists
names and roles only.

| Key | Role |
|---|---|
| `NETDATA_CLOUD_TOKEN` | long-lived Cloud REST token |
| `NETDATA_CLOUD_HOSTNAME` | Cloud REST API host (the operational target scripts call) |
| `NETDATA_REPOS_DIR` | user's mirror of Netdata-org repos |
| `AGENT_EVENTS_HOSTNAME` | network address of the ingestion node -- dual-duty: ssh host (`ssh ${AGENT_EVENTS_HOSTNAME}`) AND direct-HTTP host (`http://${AGENT_EVENTS_HOSTNAME}:19999/...`). Can be a DNS name or an IP literal. NOTE: this is NOT the journalctl namespace (hardcoded to `agent-events`) and NOT the Cloud room name (also hardcoded to `agent-events`). |
| `AGENT_EVENTS_MACHINE_GUID` | events-ingestion agent machine GUID |
| `AGENT_EVENTS_NODE_ID` | events-ingestion agent node UUID |
| `CODACY_TOKEN` | Codacy Cloud Account API token; header form `api-token: <value>` |

Per-user setup is documented at `<repo>/.agents/ENV.md`. The
committed `<repo>/.env.template` is the starting point for a
new contributor's `.env`.

Things that are intentionally NOT env-keyed (and why):

- The Cloud Swagger URL is derived as
  `${NETDATA_CLOUD_HOSTNAME}/api/docs/`.
- The agent-events producer ingest URL is a `const char *` in
  `src/daemon/status-file.c`; reference by `file:line`.
- Public site hostnames (learn site, marketing site) are public
  and used in role-descriptive prose.
- Default Netdata install paths (`/var/lib/netdata`,
  `/etc/netdata`) are public OSS defaults; bundled installs
  (`/opt/netdata/...`, etc.) are handled by runtime
  autodetection in scripts, not a config knob.
- This repo's checkout root is found via
  `git rev-parse --show-toplevel`.

Adding new keys to `.env` is the user's prerogative. SOWs and
scripts can REQUEST keys; only the user adds them.

## Verification

Before any commit that touches a SOW, skill, spec, or doc, run:

```bash
# Helper: list of files staged for commit, excluding this spec.
files=$(git diff --cached --name-only --diff-filter=ACMR \
        | grep -v '^\.agents/sow/specs/sensitive-data-discipline\.md$')
[ -z "$files" ] && exit 0

# Run each pattern. The patterns themselves are not embedded in
# this code block as committable literals; they are constructed
# from concatenated character classes so a grep over THIS spec
# does not flag itself for the very examples it must define.
grep_args=(
  --line-number --extended-regexp --binary-files=without-match
)

# Domain pattern for the org.
domain='[A-Za-z0-9-]+\.netdata\.(cloud|io)'
git grep "${grep_args[@]}" -- $files -- "$domain"

# UUID-shaped identifiers.
uuid='[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'
git grep "${grep_args[@]}" -- $files -- "$uuid"

# IPv4 literals (review hits manually for false positives).
ipv4='([0-9]{1,3}\.){3}[0-9]{1,3}'
git grep "${grep_args[@]}" -- $files -- "$ipv4"

# Per-user / per-install absolute paths. Documented Netdata
# default paths (/var/lib/netdata, /etc/netdata) are intentionally
# excluded -- they are public OSS defaults; bundled installs use
# the NETDATA_PREFIX knob.
abs="(~/|/$(echo op)t/baddisk|/$(echo op)t/neda|/$(echo op)t/ai-agent|/$(echo ho)me/)"
git grep "${grep_args[@]}" -- $files -- "$abs"

# Long opaque tokens (40+ char base64-ish).
tok='[A-Za-z0-9_+/=-]{40,}'
git grep "${grep_args[@]}" -- $files -- "$tok"
```

Every match must either be removed or converted to an env-key
reference, OR explicitly justified inline (e.g. a citation of an
upstream open-source project's hostname when documenting how that
project reports its own data).

## Exceptions

- **This spec file itself** must list forbidden patterns and
  example regexes in order to define the rule. The verification
  grep excludes `<repo>/.agents/sow/specs/sensitive-data-
  discipline.md` from its scan. No other SOW, skill, or doc
  qualifies for this exemption.
- **Quoted user messages** preserved verbatim in a SOW's "User
  Request" section may include literals the user typed. Redact
  the literal value and replace with the `.env` key in `[env-
  keyed: ${KEY}]` form, with a footnote pointing here. The
  user's wording stays; only the literal value moves to `.env`.
- **Repo-relative paths** (`src/daemon/...`, `<repo>/src/...`)
  are not absolute paths and are fine.
- **Open-source upstream references** (e.g.
  `prometheus/prometheus@<sha>:cmd/...`) are fine; they
  describe external code, not tenant data.
- **Code citations** of the form `file:line` are fine
  (`src/daemon/status-file.c:988`).

## Failure mode

If a verification grep returns a hit on a committed file, the SOW
that introduced it has failed its Sensitive Data Gate and must be
treated as a regression. Re-open the SOW, redact, force-push only
with explicit user approval (otherwise create a follow-up commit
that scrubs).
