# netdata-sos — the Netdata support bundle

`netdata-sos` collects a **sanitized diagnostic bundle** (tarball on POSIX
systems, zip on Windows) that users attach to support tickets, so support gets
everything it needs on first contact instead of asking for it over multiple
round trips.

- POSIX systems (Linux, Docker, static installs, macOS/BSD best-effort):
  [`netdata-sos.sh`](netdata-sos.sh)
- Windows: [`netdata-sos.ps1`](netdata-sos.ps1)

```sh
# installed with the agent (in PATH, like netdatacli):
sudo netdata-sos

# static installs:
sudo /opt/netdata/usr/sbin/netdata-sos

# For an older agent, download from an immutable release tag that contains the
# tool. Never pipe a URL into a root shell and never use the mutable master ref.
tag='<TRUSTED_NETDATA_RELEASE_TAG>'
t=$(mktemp "${TMPDIR:-/tmp}/netdata-sos.XXXXXX")
trap 'rm -f "$t"' EXIT HUP INT TERM
curl -fsSL -o "$t" "https://raw.githubusercontent.com/netdata/netdata/$tag/packaging/installer/netdata-sos.sh" \
  || wget -qO "$t" "https://raw.githubusercontent.com/netdata/netdata/$tag/packaging/installer/netdata-sos.sh"
```

Inspect the downloaded script. Only after that separate review, run:

```sh
sudo sh "$t"
```

```powershell
# Windows (elevated PowerShell):
powershell -ExecutionPolicy Bypass -File "C:\Program Files\Netdata\usr\libexec\netdata\netdata-sos.ps1"
```

Both scripts implement the same bundle contract: same directory layout, same
`MANIFEST.json` schema (`netdata-sos-bundle/v1`), same sanitization rules.
**If you change one script, mirror the change in the other and update this
document.**

## Design contract (do not regress these)

| guarantee | implementation |
|---|---|
| Zero system impact | self-demotion to idle CPU/IO priority (`nice -n 19` + usable `ionice -c 3` / `PriorityClass = Idle`); per-command timeout (10 s default) with process-tree termination; global deadline (240 s); size caps (5 MiB per log, 1 MiB per file, 2 MiB per command/API output); read-only collection that writes only private staging data and the requested final artifacts, never restarts anything, and never calls destructive `netdatacli` commands; final artifacts use no-overwrite publication so pre-existing files or symlinks in shared tmp dirs are never followed |
| Works when the agent is dead | no hard dependency on a running agent; the most valuable crash artifacts (status file, logs, buildinfo via the binary) are collected from disk; a `07-runtime/AGENT-API-UNREACHABLE.txt` marker is written instead of API captures without falsely claiming the process was down |
| Secrets always redacted | non-optional single-pass sanitizer; see "Sanitization" below |
| PII pseudonymized by default | IPs/MACs/emails/hostnames and ordinary customer FQDNs are replaced with **stable** pseudonyms (`ip-1`, `private-host-1`) so cross-file correlation still works; the mapping table is bounded and falls back to non-correlating placeholders after its limit; dynamic source paths get neutral archive names; the private map is saved **next to** the bundle, never inside it; `--no-obfuscate` / `-NoObfuscate` opts out of content pseudonymization |
| Caps cannot expose secrets | complete logical records are sanitized before any head/tail cap is applied; POSIX command output streams through a raw-input guard, while PowerShell jobs write a child-owned 16 MiB raw spool so command objects never accumulate in the parent queue; tail retention uses a fixed circular byte buffer; overflow, oversized records, and sanitizer failures are withheld fail-closed |
| Legible to humans AND AI agents | triage-ordered numbered directories; sanitized file copies have no injected provenance headers; provenance headers only on command captures; `MANIFEST.json` indexes every file with safe origin + sanitization state; `summary.txt` opens with a triage read-order |

## Platform support

| platform | status | notes |
|---|---|---|
| Linux (glibc, systemd) | tested | full collection incl. journal namespace |
| Linux (musl/BusyBox, e.g. Alpine) | tested | portable process-tree watchdog; file logs instead of journal |
| Docker (official image) | tested | agent logs live in `docker logs` on the host — bundle includes a marker explaining the private local-review workflow because raw logs are unsafe to attach; `/proc/1/environ` needs `CAP_SYS_PTRACE` (fallback to exec env) |
| Static builds (`/opt/netdata`) | tested paths | all paths resolved under the prefix |
| FreeBSD | best effort | `/usr/local/etc/netdata` + `/var/db/netdata` paths, `sockstat` fallback, `ps -H` threads; no `/proc` items |
| macOS (Homebrew) | best effort | `/usr/local` and `/opt/homebrew` prefixes, `sysctl`/`vm_stat` fallbacks, `ps -M` threads |
| Windows | tested in CI | Windows PowerShell 5.1 runs the adversarial suite and builds/opens a complete fixture zip; PowerShell 7 runs the same sanitizer suite |

Portability rules the POSIX script obeys (keep them when editing):

- POSIX `sh` only — no bashisms (users run it with `sh`, `dash`, BusyBox `ash`).
- **No `{n,m}` regex intervals inside the awk sanitizer** — older BSD awks
  treat them as literal braces, which would silently disable redaction.
  Character classes are written out explicitly instead.
- Feature-detect, never assume: usable `ionice`, `journalctl`, `coredumpctl`,
  `curl`/`wget`, `ss`/`sockstat`/`netstat`,
  `free`/`sysctl`, `/proc` availability are all probed before use.
- Every external command is optional: a missing tool degrades that one file,
  never the run.

## Why this exists (evidence)

Analysis of the full Freshdesk ticket history (443 tickets, 2026-07) and 287
maintainer comments across 79 GitHub bug threads showed:

- 24% of support tickets required at least one "please provide X" round trip
  (average 1.7 per ticket; some needed 3+), each adding a day or more of
  latency and eroding customer confidence.
- The asks are highly repetitive. Everything ranked below the top-20 asks fits
  in one automated collection pass.

Every item collected maps to a recurring support ask. That mapping is the
"why" column in the tables below. When adding a new item, add its why.

## What is collected, and why

### `summary.txt`, `MANIFEST.json`, `README.md` (bundle root)

| item | why |
|---|---|
| `summary.txt` | one-page human overview; opens with agent state and a "read order for triage" per issue class, so support (or an AI agent) starts at the right file |
| `MANIFEST.json` | machine-readable index: every file with its origin (command / source path / API endpoint), size, and sanitization state; lets AI tooling navigate the bundle without guessing |
| `README.md` | self-documentation for whoever receives the bundle |

### `01-system/` — platform context

| item | why |
|---|---|
| kernel/OS/architecture, distro | first question in the bug template; kernel regressions have been root causes (two GitHub issues traced to kernel changes) |
| memory, disks, CPU count, uptime | capacity questions asked in most performance tickets |
| virtualization / container detection, cgroup version | OpenVZ/LXC/CageFS visibility problems are a recurring collector-failure class |
| **clock/time sync** | clock drift on children silently breaks streaming and cloud auth — maintainers explicitly ask ("check if the clock on child nodes is drifting") |
| `/proc/self/mountinfo` (POSIX) | namespace visibility issues ("cannot open /proc/diskstats") are diagnosed from the mount table |
| kernel OOM/segfault messages | evidence of the kernel killing netdata — distinguishes crashes from kills |
| SELinux/AppArmor state | MAC denials cause silent collector failures |

### `02-install/` — how netdata got here

| item | why |
|---|---|
| `.environment` file | install method, flags, release channel, custom CFLAGS (`-ffast-math` alone broke dbengine once); contains no secrets |
| `.install-type` marker | `kickstart-build` / `kickstart-static` / `oci` / `binpkg-*` — determines which update/troubleshoot paths apply |
| package manager info | version skew between repo package and expectation is a recurring theme |
| container context (env, cgroup, pid 1) | missing `init: true`, missing `pid: host`, and wrong images are recurring Docker-ticket root causes; `NETDATA_*` env values pass through the sanitizer |

### `03-process/` — the running agent

| item | why |
|---|---|
| netdata process tree with CPU/memory | "netdata is eating my CPU/RAM" tickets need this first |
| **per-thread CPU** (POSIX) | maintainers ask users to find the hot thread in htop; this captures it non-interactively |
| `/proc/PID/status`, `limits`, fd count | leak and limit diagnosis |
| agent process environment (sanitized) | proxy/claiming issues: the env the service sees differs from the user's shell — asked explicitly in GitHub threads |
| zombie process check | plugin-reaping failures in containers (`init: true` guidance) |

### `04-config/` — configuration

| item | why |
|---|---|
| **effective running config** (`GET /netdata.conf`) | the #1 GitHub maintainer ask; shows the merged config the agent actually uses and annotates unrecognized options — resolves "my config is ignored" outright; authoritative over on-disk files |
| on-disk `netdata.conf`, `stream.conf`, cloud/claim conf, `go.d.conf`, go.d/health.d/python.d/charts.d/statsd.d user files, `exporting.conf` | the files users were asked to paste, ticket after ticket (child stream.conf + parent `[web]`/`[stream]` sections is a canned Freshdesk ask); **all pass the sanitizer**; arbitrary config paths are replaced by neutral names and recorded only in the private sidecar map |

### `05-logs/` — history

| item | why |
|---|---|
| systemd journal, **including `--namespace=netdata`** | the agent logs to its own journal namespace on systemd installs — plain `journalctl -u netdata` misses almost everything; support asks for "a complete log from start until the problem" |
| `/var/log/netdata/*.log` tails (size-capped) | non-systemd installs, static builds, macOS/BSD |
| Windows Event Log (`NetdataWEL` + Application, Netdata providers) | Windows agents log to the Event Log; Windows is a top-3 support theme |
| updater service journal | update failures; the updater keeps no persistent log file |
| **coredump metadata** (`coredumpctl list`, never the dumps) | tells support a dump exists and matches the crash time — the dump itself is fetched later only if needed |
| docker marker file | in containers the log "files" are symlinks to stdout — history only exists in `docker logs` on the host; the bundle says raw logs must not be attached and gives a private capture/review/redaction workflow using the requested time window |

### `06-state/` — persistent state

| item | why |
|---|---|
| **`status-netdata.json`** (trusted fallback locations, newest safe file wins; shared `/tmp` is excluded) | the single most valuable crash artifact: last exit reason, fatal line/file/function, signal, **stack trace** — same data that feeds agent-events crash telemetry; support gets crash forensics with zero extra round trips |
| state dir aggregate inventory | unexpected file counts/sizes without exposing filenames that may themselves be live tokens, hostnames, or job identifiers; **contents of secret files are never read** (see exclusions) |
| claim state (`claimed_id` only) | claim id is the identifier support needs to find the node in Cloud; a non-persisted `cloud.d` across restarts is a known Freshdesk root cause |
| db disk usage per tier + sqlite sizes | retention questions ("why do I only have N days") are answered by tier sizes vs configured limits |
| dyncfg files (sanitized) | jobs created via UI live here, not in `/etc/netdata` — invisible in classic config collection |
| go.d job statuses, health silencers | which collector jobs exist/fail; why alerts are silent |

### `07-runtime/` — live agent state (only when API responds)

| item | why |
|---|---|
| `/api/v3/info` | best single call: structured buildinfo, features, cloud status, per-tier retention — and it works even under bearer protection |
| `/api/v1/info`, `/api/v2/node_instances` | children, streaming state, `db_size`, metric counts — the exact endpoint maintainers ask for in retention/memory tickets |
| `/api/v3/stream_info`, `/api/v1/aclk` | streaming and cloud-connection diagnostics |
| active alerts + alert instances | alert tickets are the single biggest Freshdesk theme |
| `/api/v1/functions`, `/api/v1/ml_info` | which plugins expose what; ML state |
| `netdata -W buildinfo` + `buildinfojson` | required by the bug template; the paths section proves which config dirs the binary uses; works with the daemon **down** |
| `netdatacli aclk-state json` | canned Freshdesk ask for cloud issues |
| netdata self CPU/memory/clients CSVs (10 min, bounded) | replaces the "please send a screenshot of the Netdata memory charts" round trip |

### `08-network/` — connectivity

| item | why |
|---|---|
| listening sockets (netdata-related) | "dashboard unreachable" and port-conflict tickets |
| DNS config, proxy env/config (sanitized) | claiming-behind-proxy is a recurring theme; DNS misconfiguration breaks cloud connectivity |
| Netdata Cloud reachability (TCP/TLS handshake only, no data sent) | separates network problems from agent problems in one step |

## What is NEVER collected

These are excluded by design. **Do not add them.**

- `cloud.d/private.pem` (ACLK private key), `cloud.d/token` (claim token)
- `bearer_tokens/` (the **filenames** are live API tokens), `netdata.api.key`,
  `mcp_dev_preview_api_key`, `netdata_random_session_id`
- `/etc/netdata/ssl/` and any `*.pem` / `*.key`
- dbengine data files (metric data, GBs), `ml.db`, `registry.db` (person GUIDs
  and dashboard URLs)
- metric values other than netdata's own bounded self-monitoring charts
- anything outside netdata's own scope (no full system journals, no other
  services' logs, no packet captures)

## Sanitization

Two passes, one sweep, applied to **every** collected file:

1. **Secrets — always on, not configurable:**
   - values of any key whose punctuation-normalized name contains a complete
     secret word or phrase:
     `api key, apikey, token, access token, auth token, claim token, refresh
     token, session token, password, passwd, pass, pwd, pat, key, secret,
     client secret, client password, community, bearer,
     webhook, license key, auth, credential, cookie, passphrase, proxy user,
     proxy pass, username, dsn, private key, access key, session, recipient,
     account sid, priv key` (including common compact/camelCase spellings) — in
     ini (`k = v`), yaml (`k: v`), env (`K=V`) and
     JSON (`"k": "v"`) forms, including escaped strings, numeric/scalar
     values, and nested values. Keys must look like real config keys (≤64 chars,
     no sentence punctuation) so prose containing "token" is not mangled.
     Multi-line JSON objects/arrays and YAML block-scalar secret values are
     withheld through their closing boundary (or to EOF if malformed).
     Exemptions are decided by the KEY, never the value: keys ending in
     `file path dir directory protection support mode level port timeout
     cookies secure log size options format type` describe secrets rather than being
     secrets, so `bearer token protection = no` and `api key file = /path`
     stay readable while `TOKEN=false` and `PASSWORD=/x` are redacted;
   - argv/env-style secrets mid-line (`-token=X`, `--password "X"`,
     `CLAIM_TOKEN=X`, `api key = X` inside captured process command lines),
     including single- and double-quoted values;
   - URL-embedded credentials (`scheme://user:pass@`) and Go DSN credentials
     (`user:pass@tcp(...)`);
   - JWTs; `Bearer <value>` (including alphabetic-only credentials, with only
     an explicit diagnostic-key shape exempted), `Basic <value>`, and complete
     `Authorization` / `Authentication` header values;
   - secrets in URL query parameters (`?token=`, `&api_key=`, ... — request
     lines in access logs);
   - private-key PEM blocks — the WHOLE multi-line block is withheld from the
     BEGIN marker through the END marker (fail closed if END never arrives);
   - `stream.conf` parent-side `[<UUID>]` section headers (they ARE the API
     keys);
   - `bearer_tokens/` directory listings show a file COUNT only — the
     filenames are the tokens.
2. **PII — on by default, `--no-obfuscate` / `-NoObfuscate` to disable:**
   - non-loopback IPv4 addresses → `ip-N` and IPv6 → `ip6-N` (stable per
     bundle; compressed, lettered, and numeric-only uncompressed forms;
     validated so timestamps, `file.c:123` refs and `::1` are left alone);
   - MAC addresses → `[MAC]`; email addresses → `[EMAIL]`;
   - this host's hostname/FQDN → `redacted-host`; the invoking user's name →
     `redacted-user`;
   - ordinary FQDNs → `private-host-N`; only public Netdata service domains and
     a small exact allowlist of known Netdata filenames are preserved (a broad
     suffix exemption would leak names such as `customer.key`);
   - child/mirrored node hostnames (pre-seeded from the local API before
     collection, so they pseudonymize consistently in every file) and
     `stream.conf` `destination` hosts regardless of TLD → `private-host-N`;
   - resolv.conf `search`/`domain` values → `[SEARCH-DOMAINS-WITHHELD]`
     (corporate search domains are rarely under private TLDs).

The private map is written next to the bundle (`*.pseudonym-map.*`) so the
**user** can decode references if support asks "what is private-host-2?" — it
is never included in the bundle itself. It also maps neutral archive filenames
back to arbitrary config/log source paths, including when content PII
obfuscation is disabled. Correlating content mappings are capped at 4096
entries; additional values use generic placeholders so hostile high-cardinality
input cannot grow memory or the private map without bound.

Redaction here is defense in depth, not a substitute for exclusion: files that
are pure secrets (see exclusion list) are never read at all.

Source collection skips files reached through a symbolic link or reparse point,
and withholds text containing embedded NUL characters (for example, BOM-less
UTF-16 or binary input) instead of applying byte-unsafe redaction. The link
check protects against links planted before collection; it is deliberately not
claimed as an atomic no-follow open. The root/Administrator and Netdata service
identities are trusted not to adversarially replace source entries or mutate
source-directory contents while the tool runs. Do not extend collection to a
directory writable by any other identity. Each run uses a newly created,
unpredictable staging directory and never reuses a pre-existing path. POSIX
staging is created under `umask 077`; Windows staging disables ACL inheritance
as part of the atomic directory-creation call, before the path is observable,
and grants access only to the current identity, SYSTEM, and Administrators.

**The agent itself provides no redaction anywhere** — `GET /netdata.conf` and
`netdatacli dumpconfig` print secrets verbatim. Everything must be sanitized
by these scripts.

## How to extend it (checklist for future contributors)

1. Map the new item to a real support ask (link the ticket/issue class) and
   add it to the right section table above **with its why**.
2. Use the existing helpers — `collect_cmd` / `collect_file` / `collect_api`
   (`Collect-Cmd` / `Collect-File` / `Collect-Api` on Windows). They enforce
   timeouts, size caps, sanitization, and manifest registration. Never write
   into the bundle directly.
3. Respect the cost budget: nothing unbounded, nothing that queries metric
   data without a tight window, nothing that can block longer than the
   per-command timeout.
4. If the item can contain credentials of a NEW shape, extend the sanitizer
   in **both** scripts and add the pattern to the Sanitization section above.
5. Mirror the change in the other script (`.sh` ↔ `.ps1`) or record explicitly
   in your PR why it is platform-specific.
6. Test the redaction: add a vector to the built-in regression suite and run
   `netdata-sos --selftest` (`netdata-sos.ps1 -SelfTest` on Windows) — it must
   pass on GNU awk, mawk, BusyBox awk, and PowerShell. CI executes both suites.
   For new collection sources also
   plant a sentinel secret in the source, run a collection, and `grep -r` the
   extracted bundle. Zero hits or it does not ship.
7. Never add anything from the "What is NEVER collected" list, and never make
   the tool write, restart, reconfigure, or otherwise mutate the system.

## Bundle format contract

- Schema id: `netdata-sos-bundle/v1` (in `MANIFEST.json`). Bump the suffix on
  breaking layout changes; downstream ticket tooling may parse it.
- Command captures are `.txt` files starting with a
  `# netdata-sos v<version> | command: ... | captured: <utc>` header; on POSIX
  they also end with an `# exit: N | duration: Ns` trailer; PowerShell jobs
  record their terminal job state instead of an exit code.
- Copied files and API responses are sanitized without provenance headers
  (and remain parseable when they fit their cap); their
  provenance lives in `MANIFEST.json`, not in the files.
