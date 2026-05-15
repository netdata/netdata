# `.env` setup and reference

`.env` at the repo root holds per-user secrets and pointers
that AI-skill scripts consume. It is **gitignored** -- values
never reach the committed history.

This file is the single setup guide. Every skill that needs
`.env` keys lists them here with the role, where to find the
value, sample format, and which scripts consume it. If a
script tells you a key is missing, check this file.

## Quick start

```bash
cd <repo>
cp .env.template .env
# Open .env in your editor and fill in the keys you need.
chmod 0600 .env   # optional but recommended
```

You only need to fill in keys for the skills you actually
use. Each script checks its own required keys and exits with
a clear error if any are missing -- it will not corrupt
state if you forget a key.

## Key reference

### Netdata Cloud + agents

| Key | Role | Where to find it | Sample format |
|---|---|---|---|
| `NETDATA_CLOUD_TOKEN` | long-lived Cloud REST token | app.netdata.cloud -> user menu -> Settings -> API Tokens -> Create. `scope:all` (full) or `scope:grafana-plugin` (read-only data). | 36-char UUID-shaped token |
| `NETDATA_CLOUD_HOSTNAME` | Cloud REST API host | Almost always `app.netdata.cloud` | `app.netdata.cloud` |
| `NETDATA_REPOS_DIR` | local Netdata-org repos mirror dir | Pick or create. Will be populated by `mirror-netdata-repos` skill's sync script. | `$HOME/src/netdata` |

### agent-events ingestion node

The `agent-events` node is the Netdata-operated ingestion
host that receives status submissions from every Netdata
agent in the wild. The query-agent-events skill triages
crashes / panics / fatals from it.

| Key | Role | Where to find it | Sample format |
|---|---|---|---|
| `AGENT_EVENTS_HOSTNAME` | Network address of the ingestion node. Dual-duty -- ssh host (`ssh ${AGENT_EVENTS_HOSTNAME}`) AND direct-HTTP host (`http://${AGENT_EVENTS_HOSTNAME}:19999/`). Can be a DNS name or an IP. NOTE -- this is NOT the journalctl namespace (which is hardcoded to `agent-events`) and NOT the Cloud room name (also hardcoded to `agent-events`). | Operations / your records | `10.20.1.105` or `agent-events.example` |
| `AGENT_EVENTS_NODE_ID` | Cloud node UUID for that node | Visit the node in app.netdata.cloud and copy the UUID from the URL; or list nodes via the Cloud API and pick the matching one. | UUID |
| `AGENT_EVENTS_MACHINE_GUID` | Netdata machine GUID for that node | On the host: `sudo cat /var/lib/netdata/registry/netdata.public.unique.id` | UUID |

### Coverity Scan (coverity-audit skill)

| Key | Role | Where to find it | Sample format |
|---|---|---|---|
| `COVERITY_HOST` | Scan API host | Always `https://scan4.scan.coverity.com` for the new instance | URL |
| `COVERITY_PROJECT_ID` | integer project id | URL query param `?projectId=...` when you click the project in the dashboard | small integer |
| `COVERITY_COOKIE` | full browser Cookie header (with XSRF-TOKEN) | DevTools -> Network -> any request to scan4 -> Request Headers -> Cookie | long Cookie string |
| `COVERITY_VIEW_OUTSTANDING` | integer viewId for the "Outstanding" view | URL query param `?viewId=...` when you open that view | small integer |

The cookie expires; refresh by re-pasting from the browser
(or run `coverity-audit/scripts/keepalive.sh` to extend
it during a triage session).

### SonarCloud (sonarqube-audit skill)

| Key | Role | Where to find it | Sample format |
|---|---|---|---|
| `SONAR_HOST_URL` | SonarCloud host | Always `https://sonarcloud.io` | URL |
| `SONAR_ORG` | your organization key on SonarCloud | sonarcloud.io organization page | short string |
| `SONAR_PROJECT` | projectKey on SonarCloud | For Netdata: `netdata_netdata` | `org_repo` form |
| `SONAR_TOKEN` | personal access token | https://sonarcloud.io/account/security -> Generate | long opaque token |

### Codacy Cloud (codacy-audit skill)

| Key | Role | Where to find it | Sample format |
|---|---|---|---|
| `CODACY_TOKEN` | Account API token (header `api-token: <value>`) | https://app.codacy.com -> top-right avatar -> Account -> API tokens -> "Create API Token" | 20-char opaque string |
| `CODACY_HOST` | API host. Defaults to `https://api.codacy.com`; set only if Codacy moves the API host. | n/a | URL |
| `CODACY_PROVIDER` | git provider. Defaults to `gh` (GitHub). | n/a | `gh` |
| `CODACY_ORG` | Codacy organization (matches the GitHub org). Defaults to `netdata`. | n/a | short string |
| `CODACY_REPO` | Codacy repository name. Defaults to `netdata`. | n/a | short string |

`CODACY_TOKEN` is required by `pr-issues.sh` and any wrapper that
calls the v3 API. `analyze-local.sh` does NOT need it (the local
CLI runs anonymously).

## Per-skill checklist

Set the keys for whichever skills you plan to use. Skills
not listed here either need no `.env` keys or rely on `gh
auth` instead.

### query-netdata-cloud / query-netdata-agents

- `NETDATA_CLOUD_TOKEN`
- `NETDATA_CLOUD_HOSTNAME`
- (For agent-events examples in those skills' docs:
  `AGENT_EVENTS_HOSTNAME`, `AGENT_EVENTS_NODE_ID`,
  `AGENT_EVENTS_MACHINE_GUID`.)

### query-agent-events

- `NETDATA_CLOUD_TOKEN`
- `NETDATA_CLOUD_HOSTNAME`
- `AGENT_EVENTS_HOSTNAME`
- `AGENT_EVENTS_NODE_ID`
- `AGENT_EVENTS_MACHINE_GUID`

### mirror-netdata-repos

- `NETDATA_REPOS_DIR`

### integrations-lifecycle / learn-site-structure

- `NETDATA_REPOS_DIR` (for cross-repo path references in
  examples / recipes)

### coverity-audit

- `COVERITY_HOST`
- `COVERITY_PROJECT_ID`
- `COVERITY_COOKIE`
- `COVERITY_VIEW_OUTSTANDING`

### sonarqube-audit

- `SONAR_HOST_URL`
- `SONAR_ORG`
- `SONAR_PROJECT`
- `SONAR_TOKEN`

### codacy-audit

- `CODACY_TOKEN` (required by `pr-issues.sh`; not by `analyze-local.sh`)
- `CODACY_HOST` (optional; defaults to `https://api.codacy.com`)
- `CODACY_PROVIDER` / `CODACY_ORG` / `CODACY_REPO` (optional; default to `gh` / `netdata` / `netdata`)

### pr-reviews / graphql-audit

- No `.env` keys required. Both rely on `gh auth login`
  having been run.

## Common mistakes

- **Trailing whitespace** in a value: bash variable
  expansion preserves the whitespace; the value comes
  through with the trailing space and breaks API calls
  silently. Strip whitespace inside the quotes.
- **Wrong quoting**: quotes around bash-expansion characters
  (`$`, backticks, `\`) are interpreted. For tokens
  containing those characters, use single quotes:
  `SONAR_TOKEN='abc$def'`.
- **Expired Coverity cookie**: re-paste from the browser.
  The script's error message will tell you when this
  happens.
- **Wrong `gh` org**: `pr-reviews` and `graphql-audit` use
  `gh` against the current repo's remote. Make sure your
  remote points to the right repo (`git remote -v`).
- **Cloud token scope too narrow**: some endpoints require
  `scope:all`. If you get a 403 with what looks like a valid
  token, regenerate with broader scope.
- **`NETDATA_REPOS_DIR` and tilde**: bash does NOT expand
  the home-directory shortcut character inside
  double-quoted strings. If you write `"<TILDE>/src/netdata"`,
  the literal tilde is kept in the value, and scripts will
  fail with "directory does not exist" because that path
  is not real. Use `$HOME` instead, or the full absolute
  path:
  ```
  NETDATA_REPOS_DIR="$HOME/src/netdata"
  ```

## Why these are env-keyed

Every value above is either:
- a **secret** (token / cookie) that must never leak into
  committed artifacts, or
- a **per-user / per-deployment** path or identifier (mirror
  dir, ingestion node) that varies between contributors.

The committed skills, scripts, and docs reference these
values exclusively via `${KEY}` placeholders, never literal
values. The discipline is enforced by the spec at
`<repo>/.agents/sow/specs/sensitive-data-discipline.md`,
which includes a pre-commit grep recipe to catch
literal-value leaks.

## When a skill says "X is empty in .env"

That skill's `_lib.sh` ran the bash safety net
`: "${X:?...}"` because `X` was unset or empty. Open this
file, find the row for `X`, follow the "where to find it"
pointer, paste the value into `.env`, and re-run.
