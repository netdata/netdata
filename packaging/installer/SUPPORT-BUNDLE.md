# netdata-support-bundle — the Netdata support bundle

`netdata-support-bundle` collects a **sanitized diagnostic bundle** (tarball on POSIX
systems, zip on Windows) that users attach to support tickets, so support gets
everything it needs on first contact instead of asking for it over multiple
round trips.

- POSIX systems (Linux, Docker, static installs, macOS/BSD best-effort):
  [`netdata-support-bundle`](netdata-support-bundle)
- Windows: [`netdata-support-bundle.ps1`](netdata-support-bundle.ps1)

The POSIX file is intentionally **extension-less** (`netdata-support-bundle`, not
`.sh`): the installed command and its repository path stay implementation-neutral,
so a future reimplementation (e.g. in Rust) can replace it at the same path and
command name without changing any build/packaging reference. It is a `/bin/sh`
script today (see the shebang). The Windows counterpart keeps `.ps1` because
PowerShell requires the extension to execute a script; on Windows a future
replacement is shipped through the MSI packaging instead.

```sh
# installed with the agent (in PATH, like netdatacli):
sudo netdata-support-bundle

# static installs:
sudo /opt/netdata/usr/sbin/netdata-support-bundle

# For an older agent, download from an immutable release tag that contains the
# tool. Never pipe a URL into a root shell and never use the mutable master ref.
tag='<TRUSTED_NETDATA_RELEASE_TAG>'
t=$(mktemp "${TMPDIR:-/tmp}/netdata-support-bundle.XXXXXX")
trap 'rm -f "$t"' EXIT HUP INT TERM
curl -fsSL -o "$t" "https://raw.githubusercontent.com/netdata/netdata/$tag/packaging/installer/netdata-support-bundle" \
  || wget -qO "$t" "https://raw.githubusercontent.com/netdata/netdata/$tag/packaging/installer/netdata-support-bundle"
```

Inspect the downloaded script. Only after that separate review, run:

```sh
sudo sh "$t"
```

```powershell
# Windows (elevated PowerShell):
powershell -ExecutionPolicy Bypass -File "C:\Program Files\Netdata\usr\libexec\netdata\netdata-support-bundle.ps1"
```

Both scripts implement the same bundle contract: same directory layout, same
`MANIFEST.json` schema (`netdata-support-bundle/v1`), same sanitization rules.
**If you change one script, mirror the change in the other and update this
document.**

## Design contract (do not regress these)

| guarantee | implementation |
|---|---|
| Zero system impact | self-demotion to idle CPU/IO priority (`nice -n 19` + `ionice -c 3` / `PriorityClass = Idle`); per-command timeout (10 s default, via `timeout` or a portable watchdog — the watchdog kills the direct child only, a documented limitation); global deadline checked before each collector, so the hard runtime bound is deadline + one command timeout; size caps (5 MiB per log, 1 MiB per file, 2 MiB per command/API output); read-only — writes only its private staging dir and the final artifacts, never restarts or reconfigures anything; artifacts are published with `O_EXCL` so pre-existing files or symlinks in shared tmp dirs are never followed |
| Works when the agent is dead | no hard dependency on a running agent; the most valuable crash artifacts (status file, logs, buildinfo via the binary) are collected from disk; a `07-runtime/AGENT-WAS-DOWN.txt` marker is written instead of API captures |
| Secrets always redacted | non-optional single-pass sanitizer; see "Sanitization" below |
| PII pseudonymized by default | IPs (v4+v6), MACs, emails, this host's names, the invoking user, child/mirrored node hostnames and stream destinations are replaced with **stable** pseudonyms (`ip-1`, `private-host-1`) so cross-file correlation still works; the private map is saved **next to** the bundle, never inside it; `--no-obfuscate` / `-NoObfuscate` opts out |
| Caps cannot expose secrets | all caps cut at LINE boundaries, so a secret can never straddle the cut and dodge the line-based sanitizer; a capped tail with no line break at all is withheld entirely; sanitizer failures withhold the file content (fail closed) |
| Legible to humans AND AI agents | triage-ordered numbered directories; sanitized file copies have no injected provenance headers; provenance headers only on command captures; `MANIFEST.json` indexes every file with safe origin + sanitization state; `summary.txt` opens with a triage read-order |

## Platform support

| platform | status | notes |
|---|---|---|
| Linux (glibc, systemd) | tested | full collection incl. journal namespace |
| Linux (musl/BusyBox, e.g. Alpine) | tested | BusyBox `timeout` has no `-k` (auto-detected); file logs instead of journal |
| Docker (official image) | tested | agent logs live in `docker logs` on the host — bundle includes a marker with the exact command to run and attach; `/proc/1/environ` needs `CAP_SYS_PTRACE` (fallback to exec env) |
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
| on-disk `netdata.conf`, `stream.conf`, cloud/claim conf, `go.d.conf`, go.d/health.d/python.d/charts.d/statsd.d user files, `exporting.conf` | the files users were asked to paste, ticket after ticket (child stream.conf + parent `[web]`/`[stream]` sections is a canned Freshdesk ask); **all pass the sanitizer**, and their bundle paths mirror their paths relative to the config directory |

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

Local API reads target `127.0.0.1:19999` directly and bypass any configured
proxy, so diagnostic data cannot leave the host through a forced proxy.

### `08-network/` — connectivity

| item | why |
|---|---|
| listening sockets (netdata-related) | "dashboard unreachable" and port-conflict tickets |
| DNS config, proxy env/config (sanitized) | claiming-behind-proxy is a recurring theme; DNS misconfiguration breaks cloud connectivity |
| Netdata Cloud reachability (TCP plus certificate-validating HTTPS/TLS probe; no bundle data sent) | separates network problems from agent problems in one step |

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

## Redaction philosophy

This tool follows the same proportionate posture as established support-bundle
tools (sosreport, supportconfig, `kubectl cluster-info dump`, Elastic's
diagnostics): **redact the well-defined, high-value cases robustly, and treat
redaction as best-effort defense-in-depth — not a guarantee.**

Two facts do the heavy lifting and are why we do not chase completeness:

1. the tool runs on the **user's own host**, under their own account; and
2. the output is plain-text, organized, and the user is told to **review the
   bundle before sending it** (`summary.txt` and this document say so).

Concretely, we redact credential-bearing config keys, URL/DSN credentials,
JWT/Bearer/Basic tokens, PEM key blocks, `stream.conf` API-key sections, and
PII (IPs, MACs, emails, hostnames, usernames); and we never collect files that
are *pure* secrets at all (the never-collect list). We deliberately do **not**
try to parse arbitrary nested structure to prove no secret can ever slip
through — a line-based tool cannot balance nested JSON brackets or detect
indentation-based YAML block-scalar boundaries reliably, and every attempt adds
fragile regex for encodings that do not occur in the data this bundle collects.
A brittle sanitizer that tries to do everything is worse than a stable one that
does the common cases well; the durable place for structure-aware,
schema-driven redaction is inside the agent, not a portable shell/PowerShell
script. When extending the tool, prefer this restraint.

## Sanitization

Two passes, one sweep, applied to **every** collected file:

1. **Secrets — always on, not configurable:**
   - values of any key whose punctuation-normalized name contains a
     secret word or phrase:
     `api key, apikey, token, password, passwd, pwd, secret, community, bearer,
     webhook, license key, auth, credential, cookie, passphrase, proxy user,
     proxy pass, username, dsn, private key, access key, session, recipient,
     account sid, priv key` — in ini (`k = v`), yaml (`k: v`), env (`K=V`) and
     JSON (`"k": "v"`) forms, covering escaped JSON strings and numeric/scalar
     JSON values. (Aliases are matched as substrings, so only unambiguous
     secret tokens are on the list — e.g. `pat` is deliberately NOT, because it
     matches `path`.) Keys must look like real config keys (≤64 chars,
     no sentence punctuation) so prose containing "token" is not mangled.
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
   - JWTs; `Bearer <value>` where the value contains a digit (real tokens do;
     this avoids mangling config prose like `bearer token protection = no`),
     `Basic <value>`, and `Authorization:` header values;
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

The private map is written next to the bundle (`*.pseudonym-map.tsv`) so the
**user** can decode references if support asks "what is private-host-2?" — it
is never included in the bundle itself. Pseudonym mappings are capped at 4096
entries; past the cap, values get a non-correlating placeholder so hostile
high-cardinality input cannot grow memory or the private map without bound.

Redaction here is defense in depth, not a substitute for exclusion: files that
are pure secrets (see exclusion list) are never read at all. Files containing
NUL bytes (binary or BOM-less UTF-16 input) are withheld rather than run
through byte-unsafe line redaction. A source file whose leaf is itself a
symlink is withheld (a swapped link must not redirect collection to another
target); symlinked parent directories resolve normally. Do not extend
collection to a
directory writable by any other identity. Each run uses a newly created,
unpredictable staging directory and never reuses a pre-existing path. POSIX
staging is created under `umask 077` on POSIX and under the per-user `%TEMP%`
tree on Windows, with an unpredictable random name. Final artifacts (tarball,
zip, pseudonym map) are published with a no-overwrite move so a pre-existing
file or symlink at the target is never followed or clobbered.

**The agent itself provides no redaction anywhere** — `GET /netdata.conf` and
`netdatacli dumpconfig` print secrets verbatim. Everything must be sanitized
by these scripts.

## How to extend it (checklist for future contributors)

1. Map the new item to a real support ask (link the ticket/issue class) and
   add it to the right section table above **with its why**.
2. Use the existing helpers — `collect_cmd` / `collect_file` / `collect_api`
   (`Save-Cmd` / `Save-File` / `Save-Api` / `Save-CmdRaw` on Windows). They enforce
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
   `netdata-support-bundle --selftest` (`netdata-support-bundle.ps1 -SelfTest` on Windows) — it must
   pass on GNU awk, mawk, BusyBox awk, and PowerShell. CI executes both suites.
   For new collection sources also
   plant a sentinel secret in the source, run a collection, and `grep -r` the
   extracted bundle. Zero hits or it does not ship.
7. Never add anything from the "What is NEVER collected" list, and never make
   the tool write, restart, reconfigure, or otherwise mutate the system.

## Bundle format contract

- Schema id: `netdata-support-bundle/v1` (in `MANIFEST.json`). Bump the suffix on
  breaking layout changes; downstream ticket tooling may parse it.
- Command captures are `.txt` files starting with a
  `# netdata-support-bundle v<version> | command: ... | captured: <utc>` header; on POSIX
  they also end with an `# exit: N | duration: Ns` trailer. PowerShell command
  captures carry the provenance header only (background jobs do not surface a
  meaningful process exit code).
- Copied files and API responses are sanitized without provenance headers
  (and remain parseable when they fit their cap); their
  provenance lives in `MANIFEST.json`, not in the files.
