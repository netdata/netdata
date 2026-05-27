# SOW-0038 - Step 6: cgroup-name Go binary; remove cgroup-name.sh

## Status

Status: completed

Sub-state: revised after round-4 reviewer feedback (2026-05-26). Round-4 NOT READY findings applied: (a) documented existing `-m 3` GCP curl timeout (lines 267-269) as a script behavior that MUST be preserved verbatim and reconciled with master plan D6 ("D6 forbids NEW timeouts; existing script timeouts are part of the verbatim port"); (b) specified `@sbindir_POST@` PATH resolution via `-ldflags -X` build-time injection by CMake; (c) corrected false "filesystem rename atomicity" claim — script uses non-atomic `echo > file`, Go binary MUST preserve this exact non-atomic behavior; (d) called out the `ystem.slice` sed typo at line 682 as a required-to-preserve bug; (e) rewrote parallelism analysis — `docker_like_get_name_api` returns 0 on curl/jq failures, so `||` fallback chains at lines 598/623 are gated by jq-missing or empty-host preconditions, NOT runtime failures; primary speedup is in-process replacement of subprocess spawning, not parallelism; (f) resolved SYSLOG_IDENTIFIER/ND_REQUEST contradiction (Option A: binary uses `cgroup-name`, validation gate excludes this byte difference); (g) corrected SOW invocation list (removed `systemctl` and `awk` — not in script); (h) enumerated all 3 tmpfile paths explicitly; (i) added fixtures for kubepods root, kind clusters, Proxmox multi-line name, docker-with-jq-missing, podman-with-jq-missing; (j) documented `get_lbl_val` "null" return, `k8s_get_kubepod_name` implicit return, `cmd_line` trailing-space and single-quote escaping verbatim requirements, `eval` at line 113 as required behavior, KUBE_CONFIG unset-vs-empty bash semantics; (k) specified build system integration via `add_go_target` macro and `go.mod` placement; (l) specified replay-harness capture wrapper log format.

Sub-state revised after round-5 reviewer feedback (2026-05-26). Round-5 MAJOR + MINOR findings applied: (a) F5 MAJOR — corrected truncation-scope wording: the 100-char truncation at script line 729 is INSIDE the `if [ -z "${NAME}" ]; then ... fi` block (lines 668-730), NOT unconditional (this round-5 fix was itself incomplete and was corrected again in round 6 — see next paragraph); (b) Contradiction 1 MAJOR — rewrote "Docker fallback to podman CLI" fixture to match the gating semantics established in round 4: the line 598 `||` CLI branch fires ONLY when DOCKER_HOST is empty OR jq is missing (NOT on runtime failure); (c) Contradiction 4 MAJOR — rewrote the Implications and Decisions section (now at L348-350): "Parallelism is the ONLY new behaviour" replaced with "Two new behaviours: PRIMARY in-process subprocess replacement, SECONDARY bounded parallelism within GCP metadata fetch chain", matching the Purpose framing; (d) MINOR — expanded EXIT_RETRY enumeration to include L602 (docker_get_name empty-NAME post-call) and L627 (podman_get_name empty-NAME post-call); expanded EXIT_DISABLE enumeration to include L585 (k8s_get_name `*)` default case); (e) MINOR — flagged `add_go_target` macro caveat: the macro hardcodes `GO_LDFLAGS` and silently drops extra ldflags args via `${ARGN}`, so the naive snippet WILL NOT inject `sbindirPost` — macro must be extended; (f) MINOR — documented K8s pod_uid tmpfile cache overwrite-every-miss contract; (g) MINOR fixture additions: fatal-no-args (exit 1), K8s QoS-suffix top-level cgroup, K8s pod_uid-only variants (L344 vs L347), Nomad partial-vars fall-through to CONT_NAME, multi-label netdata.cloud/* coexistence including label-with-`=`-in-value; (h) MINOR — added replay wrapper temporal-ordering note.

Sub-state revised after round-6 reviewer feedback (2026-05-26). Round-6 MAJOR findings applied: (F1) the round-5 truncation-scope correction was HALF-WRONG — verified by re-reading `cgroup-name.sh.in:580-739` in full. K8s-resolved names DO skip the L729 truncation (K8s assignment at L662-666 is BEFORE the L668 gate), but docker/podman-resolved names DO get truncated. The dispatcher elif's at L669/L674/L679 (docker / ECS / containerd) and L684 (libpod) all live INSIDE the L668 block; they call `docker_validate_id`/`podman_validate_id` which call `docker_get_name`/`podman_get_name`, and those helpers assign NAME at L603/L605/L628/L630 — all of those NAME assignments happen with control INSIDE the L668 block, so L729 truncation applies on the way out. Rewrote the Implementation Understanding bullet, fixed the corresponding fixtures (Docker-resolved/Podman-resolved >100 chars are TRUNCATED, not preserved), and updated the Acceptance Criteria flow-doc description. (F2) Unified L15 Purpose section to match L348-350: SECONDARY = "bounded parallelism within the GCP metadata fetch chain at script lines 267-269 only" (ONE chain, not "two specific fallback chains"). (F3) Rewrote the Inferences section bullet at L97 — removed the "try docker socket; if that fails, try docker CLI ... can run concurrently" sentence (which contradicted L88-89 and L350) and replaced it with the gating-preconditions model already established elsewhere in the SOW. Awaiting round-7 reviewer pass.

Sub-state revised after round-10 subagent reviewer feedback (2026-05-26). Round-10 MAJOR + MINOR findings applied: (R10-MAJOR) Internal contradiction at Acceptance Criteria § Error/timeout edge cases (Docker/Podman non-2xx wording) — earlier wording said "script's `curl --fail` propagates non-2xx as exit ≥22" and "treat any non-2xx as equivalent to curl-failure", which CONTRADICTED the round-9 BINDING table at L200-206 establishing that Docker/Podman API calls (L171/L174) OMIT `--fail`. The contradiction is observably different at the implementation level: an implementer following the old L236 wording would short-circuit on `resp.StatusCode != 200` and never call the JSON parser, while the binding table requires reading the body and feeding it to the parser. Rewrote the bullet to match L200-206: ALWAYS read the response body, feed it to the JSON parser, return 0 with empty NAME on parse/extract failure (per script line 181); added corner-case fixtures (404 body that parses but lacks Name field; 200 with malformed JSON). (R10-MINOR cosmetic) Sub-state header and PATH env-var enumeration both cited "script line 7" for the `export PATH=...` extension; actual line is 8 (lines 1-7 are shebang / SPDX / comment / blank). Corrected both citations.

Sub-state revised after round-11 follow-up sweep (2026-05-26). Trivial cosmetic fix: the Process gate § FLOW.md contents bullet at L270 still cited `(line 7)` for the `export PATH=...` statement; corrected to `(line 8; lines 1-7 are shebang / SPDX / comment / blank)` to match the wording already applied at L13 and L140. Whole-file sweep for residual `line 7` references confirms no other stale citations remain (only the two L13/L140 occurrences inside the "lines 1-7 are..." clarifier, which are correct). Awaiting round-11 reviewer pass.

Sub-state revised after round-9 reviewer feedback (2026-05-26). Round-9 CRITICAL + MAJOR findings applied: (Q9-CRITICAL) removed the false claim that the script reads `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` — verified via grep against `cgroup-name.sh.in`: zero matches. The script's K8s API calls at lines 407 and 421 use `curl --fail -sSk` (`-k` = insecure skip TLS verify); no CA cert is consulted anywhere. Updated the "Files read" enumeration (removed `ca.crt`) and rewrote the TLS compatibility test at the Acceptance Criteria section: the Go binary MUST use `tls.Config{InsecureSkipVerify: true}` for K8s API and kubelet calls, matching the script's `-k`; do NOT load the projected service-account CA cert (would make the binary MORE strict than the script and break clusters whose API server presents a cert the script today accepts via `-k`). (Q9-MAJOR-1) added an explicit binding `curl --fail` vs no-`--fail` semantics table to the "In-process replacement IS PERMITTED" curl bullet: GCP metadata (L267-269) and K8s API/kubelet (L407/L421) use `--fail` — non-2xx MUST be treated as hard failure in Go; Docker/Podman API calls (L171/L174) use `-sS` (no `--fail`) — non-2xx body MUST be passed to JSON parsing, returning 0 with empty NAME on any parse/extract failure (matches script line 181's unconditional `return 0`). Listed all 5 curl call sites in the script and enumerated which use which mode. (Q9-MAJOR-2) added `HOME` and `PATH` to the explicit env-var enumeration: `HOME` is not read directly by the script (verified) but is inherited and consumed by kubectl when `KUBE_CONFIG=""` (kubectl falls back to `$HOME/.kube/config`); `PATH` is extended at script line 8 (lines 1-7 are shebang / SPDX / comment / blank) and the Go binary must replicate the extension before any `exec.LookPath`/`exec.Command`. Awaiting round-10 reviewer pass.

Sub-state revised after round-8 reviewer feedback (2026-05-26). Round-8 MAJOR + MINOR findings applied: (Q-MAJOR) `systemd-cat-native` invocation mechanism unspecified — the SOW said "Go binary MUST emit the same log fields" but did not say HOW. Resolution: adopt Option A per reviewer's recommendation — the Go binary invokes `systemd-cat-native --log-as-netdata` as a subprocess with the log payload piped on stdin, EXACTLY as the script does at `cgroup-name.sh.in:75-83`. This aligns with the verbatim-port mandate, eliminates implementation risk (no need to reimplement the systemd journal protocol, namespace support, or TLS in Go), and `systemd-cat-native` is always present where Netdata is installed (built at `CMakeLists.txt:3315`). The log call is not a performance bottleneck (one stderr line per resolution failure). Added to Implementation Understanding + Acceptance Criteria In-process replacement scope (`systemd-cat-native` is in the FORBIDDEN list — stays as subprocess) + Flow Document scope. (Q-M1) Added `NETDATA_LOG_LEVEL=debug` fixture to validate the script's `debug()` LOG_LEVEL=7 path at `cgroup-name.sh.in:105-107` produces identical log emissions in the Go binary. (Q-M2) Signal handling specified: the Go binary inherits the script's behaviour — no explicit signal handlers; the Go default `os/signal` (SIGTERM/SIGINT cause default Go runtime termination, including killing any in-flight `exec.Command` child via the standard `cmd.Process` behaviour on parent exit; explicitly DO NOT install `signal.Notify` handlers, DO NOT call `signal.Reset` — match the bash default of "die when killed, propagate to children via the process group"). (Q-M3) reviewer proposed binary-neutral wording for the cgroup-name resolver config; implementation later corrected this to preserve the legacy user-facing INI key for backward compatibility. (Q-M4) Dual-run wrapper performance impact addressed: the wrapper doubles discovery latency during the validation window; this is acceptable on `representative validation host` (test/dev workstation) but the wrapper MUST NOT ship on production hosts — installation is staged ONLY on the validation host, never via packaging. The validation window is bounded (several hours, max 24h) and explicitly removed by the close commit. (Q-M5) `MESSAGE` newline-escaping explicitly called out: the script at `cgroup-name.sh.in:82` uses `MESSAGE=${*//$'\n'/\\n}` which replaces every literal newline byte (0x0A) with the two-byte ASCII sequence `\n` (backslash followed by lowercase n, 0x5C 0x6E). The Go binary MUST replicate this exact transformation via `strings.ReplaceAll(msg, "\n", "\\n")` BEFORE emitting the MESSAGE field to systemd-cat-native — do NOT use `%q`, `fmt.Sprintf`, or any Go default formatting (they escape differently, e.g., `%q` would also escape backslashes and produce `\\n` from a literal `\n` input). Awaiting round-9 reviewer pass.

Sub-state revised after round-7 final reviewer feedback (2026-05-26). Round-7 BLOCKING concerns applied (7 fixes; final reviewer was used per user's "elite reviewer used once" protocol, future iterations use the standard 5 reviewers only): (C1) CRITICAL — verbatim-port vs curl→net/http contradiction resolved by adding a new explicit "In-process replacement scope" sub-section to Acceptance Criteria that enumerates exactly which operations may be in-process replacements (text/JSON processing primitives: sed, grep, jq, head, basename, curl-to-net/http), which MUST stay as subprocesses (docker, podman, kubectl, snap, ps, command-v/hash presence probes), and adds binding "External-environment compatibility tests" for TLS, proxy, socket, error/timeout, kubectl-auth, and Docker/Podman CLI edge cases — every test enumerated must pass before replacement. (C2) CRITICAL — replay harness rewritten to DUAL-RUN mode: the capture wrapper now invokes BOTH `cgroup-name.sh.real` AND the Go binary `cgroup-name` in the SAME invocation, on the SAME live system, capturing the SAME instantaneous system state for both, then compares outputs immediately and records `diff_status` per record. Justification for dual-run vs record-and-replay added: instantaneous system state (docker map, kubectl response, /etc/pve files, tmpfile cache, ephemeral env like XDG_RUNTIME_DIR) is not mockable for live parity proof. (C3) MAJOR — Podman fixtures rewritten to match the script's actual `libpod-<HEX>` / `libpod-conmon-<HEX>` detection at L684; added an explicit `podman-<HEX>` fall-through fixture proving the binary does NOT add detection branches the script lacks. (C4) MAJOR — corrected the K8s pod-regex elif fixture descriptions: line 341 = CRI-prefixed CONTAINER ID, line 344 = non-CRI CONTAINER ID, line 347 = POD_UID-ONLY (only one of the three resolves pod_uid alone, contrary to the earlier "three pod_uid branches" wording). (C5) MAJOR — rewrote the external-command-and-builtin inventory: added `basename` (L16) and `head` (L711, L719) as external commands explicitly invoked by the script; added a full classification table separating external commands from shell builtins; corrected the earlier wrong claim that "`basename` is NOT invoked". (C6) MAJOR — removal list expanded to include `src/collectors/cgroups.plugin/cgroup-internals.h:163` (comment update from `cgroup-name.sh` → `cgroup-name`) and `src/libnetdata/inicfg/MIGRATE_TO_YAML_WIP.md:1312` with an evidence-backed KEEP decision (historical migration ledger preserves legacy wording with a marker footnote added). (C7) MAJOR — Pre-Implementation Gate filled with concrete content for every required sub-section (problem/root-cause model, evidence reviewed with file:line citations, affected contracts and surfaces, existing patterns to reuse, risk and blast radius, sensitive data handling plan, implementation plan, validation plan, artifact impact plan, open decisions resolved); stale "Go binary location" local decision removed (resolved by SOW-0032 D9). Awaiting multi-reviewer pass (final reviewer is not consulted again per protocol).

## Requirements

### Purpose

Replace the shell script `src/collectors/cgroups.plugin/cgroup-name.sh.in` with a Go binary that is a **100% faithful, line-by-line, option-by-option port** of the shell script's algorithm. The Go binary's PRIMARY speedup mechanism is **in-process execution replacing subprocess spawning** (sed, grep, jq replaced with Go code; docker/podman/kubectl API calls replaced with Go HTTP clients where possible per the script's existing branches). A SECONDARY mechanism is **bounded parallelism within the GCP metadata fetch chain at script lines 267-269 only** — the 3 `curl --fail -s -m 3 --noproxy "*"` calls there are truly independent and can run as 3 parallel goroutines (each preserving its own `-m 3` timeout, first-error cancellation matching the script's `&&` short-circuit). No other parallelism is introduced; see Parallelism Scope below. Where the script's sequencing matters for correctness (one check gates another), the Go binary preserves the dependency.

**Bug-for-bug compatibility is required.** Known bugs in the script that the Go binary MUST reproduce verbatim:
- The `sed` pattern at `cgroup-name.sh.in:682` matches `^.*ystem.slice_containerd.service_cpuset_` (missing leading `s`). The regex at line 679 correctly matches `system.slice_containerd...`, but the sed extraction uses `ystem.slice`. Since `.` matches any char, the typo still extracts correctly when the cgroup begins with `system.slice`, but the pattern is technically wrong. The Go binary MUST use the same (buggy) pattern; do NOT "fix" it.
- The error message at `cgroup-name.sh.in:639` says "a podman id cannot be extracted from docker cgroup" — should say "podman cgroup". Go binary MUST preserve the typo verbatim in stderr log payload.
- The `cmd_line` construction at line 11 uses `printf "'%s' "` which (i) leaves a trailing space after the last argument and (ii) does NOT escape single quotes inside arguments — both behaviors MUST be reproduced byte-for-byte in the Go binary's ND_REQUEST log field.

This is a SPEED OPTIMISATION, not a redesign. The shell script is 10 years of battle-tested behaviour resolving names for millions of containers across every plausible scenario. The Go binary inherits 100% of that behaviour — including its bugs — and adds nothing.

**D6 reconciliation (existing vs new timeouts):** master plan D6 forbids NEW artificial timeouts. The script's existing `curl --fail -s -m 3` per-request timeout on the 3 GCP metadata API calls (`cgroup-name.sh.in:267-269`) is an EXISTING script behavior and MUST be preserved verbatim in the Go binary — it is the ONLY explicit timeout anywhere in the 741-line script. D6 governs new timeouts the binary would add; it does not strip existing ones.

After this SOW completes, `cgroup-name.sh` is removed entirely from the source tree, the build system, and packaging manifests.

### User Request

Verbatim from user (2026-05-26):

> It is important to do in the cgroup-name binary EXACTLY THE SAME, cgroup-name.sh does. Line by line, option by option, IT HAS TO DO 100% THE SAME, NO MORE, NO LESS.
>
> cgroup-name.sh is extremely battle tested. It works for 10 years, it has discovered the names of millions of containers, in all possible scenarios. We need the exact FLOW, the exact CONDITIONS, the same PRIORITIES, the same calls, no matter what.
>
> cgroup-name.sh has been VERY CAREFULLY designed to avoid unnecessary calls, etc.
>
> DO NOT REINVENT IT. WE NEED A FASTER SCRIPT, NOT A DIFFERENT LOGIC.
>
> You need to study the script VERY CAREFULLY, write down all parameters, all options, all calls, the exact flow. We are looking for better speed. Not a different thing. Not a similar thing, but a 100% ported to Go script.

This is the binding directive for this SOW.

### Implementation Understanding

Facts:

- The script is at `src/collectors/cgroups.plugin/cgroup-name.sh.in` (741 lines). It is the `.in` template; `configure_file` (CMakeLists.txt:3671) substitutes `@sbindir_POST@` into PATH and produces the installed script at `${libexecdir}/netdata/plugins.d/cgroup-name.sh`.
- Single caller: `cgroups.plugin` via `spawn_popen_run_variadic` at `cgroup-discovery.c:235`. Arguments: `cg->id` (cgroup full path) and `cg->intermediate_id` (some prepared form).
- Return contract: stdout one line with the resolved name (and optionally labels). Exit codes (full enumeration verified from script):
  - `0 = EXIT_SUCCESS` — name resolved, use it. Default value at line 652.
  - `1 = exit from fatal()` (line 100-103) — used only when called without a cgroup name (line 659). C caller treats as transient failure.
  - `2 = EXIT_RETRY` — transient detection failure. Full source enumeration (verified 2026-05-26): (i) `k8s_get_name` case `2)` at line 580 (k8s_get_kubepod_name returned 2: kubectl/kubelet not ready, OR the implicit-return path returned 1 when `[ -n "$name" ]` was false); (ii) `docker_get_name` post-call empty-NAME branch at line 602 (NAME empty after dispatch → set EXIT_RETRY + NAME=`${id:0:12}`); (iii) `podman_get_name` post-call empty-NAME branch at line 627 (same semantic for podman). C caller retries on next discovery iteration.
  - `3 = EXIT_DISABLE` — skip this cgroup permanently. Full source enumeration (verified 2026-05-26): (i) K8s pod_uid/cntr_id extraction failure (line 360); (ii) K8s pause container (line 367); (iii) kubevirt helper-container skip (lines 489-496); (iv) `k8s_get_name` `*)` default case at line 585 (k8s_get_kubepod_name returned any unexpected code, e.g., 3 from the pause-container path bubbling up — NAME is set to `k8s_${id}` and a "disabling it" warning is emitted, observationally distinct from the in-function return-3 path which exits the function without assigning NAME). C caller sets `processed = 1`.
- **Input transform**: the second argument is processed via bash `CGROUP="${2//\//_}"` at line 648 (all `/` replaced with `_`). The Go binary MUST apply the same transform to its second argument before processing.
- **Output transforms near the end of the script — CRITICAL SCOPE DISTINCTION** (verified by reading `cgroup-name.sh.in:580-739` in full, corrected 2026-05-26 — earlier "K8s/docker/podman SKIP truncation" wording was HALF-WRONG: K8s skips, docker/podman do NOT):
  - **100-char truncation at line 729 is CONDITIONAL** — it lives INSIDE the `if [ -z "${NAME}" ]; then ... fi` block that spans lines 668-730. The truncation runs whenever NAME is still empty when control reaches line 668 (i.e., the K8s top-block at lines 662-666 did NOT set NAME). Inside the L668 block, the assignment at line 728 (`[ -z "${NAME}" ] && NAME="${CGROUP}"`) is the fall-through; the truncation at line 729 then applies to whatever NAME holds at that point — INCLUDING any NAME assigned by branches NESTED INSIDE the L668 block.
  - **K8s-resolved names SKIP the truncation.** `k8s_get_name` is invoked from the L662-666 block (BEFORE the L668 gate), so K8s assignments at script lines 564/568/573/578/583 leave NAME populated when control reaches L668; the `[ -z $NAME ]` test is false, the entire L668-L730 block (truncation included) is skipped.
  - **Docker/podman-resolved names ARE subject to truncation.** The docker/podman dispatcher elif's at lines 669/674/679 (docker / ECS / containerd → `docker_validate_id`) and 684 (libpod → `podman_validate_id`) live INSIDE the L668 block. Those validators call `docker_get_name` / `podman_get_name`, which assign NAME inside their helper bodies (script lines 603/605 inside `docker_get_name`; 628/630 inside `podman_get_name`). Control returns to the dispatcher INSIDE the L668 block, falls through past L728 (NAME is non-empty, so the `&& NAME="${CGROUP}"` is a no-op), and HITS L729: if the resolved NAME is >100 chars, it IS truncated.
  - **Same applies to systemd-nspawn / libvirt-lxc / libvirt-qemu / Proxmox VM/LXC / lxc.payload branches** (script lines 689-725): all inside the L668 block, all subject to L729 truncation.
  - **Space→underscore at line 732 is UNCONDITIONAL** — it lives OUTSIDE the `[ -z "${NAME}" ]` block and applies to ALL final NAME values, including K8s-resolved names that bypassed truncation.
  - The Go binary MUST preserve this exact split: do NOT truncate K8s-resolved names even if they exceed 100 chars; DO truncate docker/podman/nspawn/libvirt/Proxmox/lxc.payload/fall-through names at 100 chars; DO space→underscore every final NAME unconditionally. See the dedicated fixtures in the Validation gate ("K8s name >100 chars NOT truncated" vs "Docker-resolved NAME >100 chars IS truncated") that prove this contract.
- **External-command and shell-builtin inventory** (round-7 correction 2026-05-26 — earlier draft mis-classified `basename` and omitted `head`). Verified via grep against `cgroup-name.sh.in`. Explicit classification:

  **External commands invoked (each MUST be reproduced in the Go binary per the In-process replacement scope above):**
  - `docker` — CLI invocations via `docker_like_get_name_command`. Environment-observing; stays as `exec.Command`.
  - `podman` — CLI invocations (including the docker-CLI fallback at line 598 which literally invokes `podman`). Environment-observing; stays as `exec.Command`.
  - `kubectl` — script lines 427, 433 with `--kubeconfig=...`. Environment-observing; stays as `exec.Command`.
  - `snap` / `snap list docker` — script line 593. Environment-observing; stays as `exec.Command`.
  - `ps` (`ps -C kubelet`) — script line 425. Environment-observing; stays as `exec.Command`.
  - `curl` — script lines 142, 169-173 (Docker/Podman HTTP API), 263-269 (GCP metadata API), K8s API server and kubelet API paths. Replaced with Go `net/http` BUT subject to the compatibility tests above.
  - `jq` — script lines 158-161 (presence probe), 177 (JSON parsing). Presence probe stays as `exec.LookPath`; JSON parsing replaced with `encoding/json`.
  - `sed` — many invocations. Replaced with Go `regexp`.
  - `grep` / `grep -E` — many invocations. Replaced with Go `regexp` / line iteration.
  - `head` — script lines 711 (`grep -e '^name: ' "${FILENAME}" | head -1 | sed …`, Proxmox VM config) and 719 (`grep -e '^hostname: ' "${FILENAME}" | head -1 | sed …`, Proxmox LXC config). Replaced with Go line-limit iteration. The Go port MUST pick only the FIRST matching line in each Proxmox config file (matching `head -1` semantics) — this is also called out in the multi-line Proxmox fixtures below.
  - `basename` — script line 16 (`PROGRAM_NAME="$(basename "${0}")"`). The Go binary computes `PROGRAM_NAME` via `filepath.Base(os.Args[0])` (or hardcodes `"cgroup-name"` per the SYSLOG_IDENTIFIER Option A decision). This is an in-process replacement (no subprocess), but `basename` IS listed in the inventory as an external command the script invokes.
  - `echo` — many invocations. POSIX `echo` is sometimes an external command, sometimes a bash builtin depending on PATH and shell — under bash it is ALWAYS a builtin (no fork). The Go binary's equivalent is just `fmt.Println` or building a string. Listed here for inventory completeness; treat as a builtin for the in-process replacement scope.
  - `hash` — bash builtin used at lines 593, 622 for command presence probing. Replaced with `exec.LookPath`.
  - `command -v` — bash builtin used at lines 593, 622 for snap+kubectl presence probing. Replaced with `exec.LookPath`.

  **Shell builtins / bash constructs (naturally in-process in Go):**
  - `[ ]` / `[[ ]]` tests, `case`, `if`, `while`, `for`, `local`, `return`, `exit`, `fatal()` (script-defined wrapper around `exit 1`).
  - Parameter expansions: `${var//pat/repl}`, `${var/pat/repl}`, `${var:0:N}`, `${#var}`, `${var:-default}`, `${var:=default}`, `${var+x}` (used at L432 for KUBE_CONFIG unset-vs-empty test).
  - BASH_REMATCH array (populated by `[[ ... =~ ... ]]`).
  - `eval` at line 113 — see Resolved policy bullet for the exact required-behavior port.
  - `printf` at line 11 for `cmd_line` construction — see Resolved policy bullet for the single-quote / trailing-space port.
  - `IFS="="` read at line 124 for label parsing — see fixture below.
  - Process substitution `<<<` and `<(...)`.

  **NOT invoked anywhere in the script** (verified via grep, listed to prevent regressions from earlier wrong inventory): `systemctl`, `awk`, `xargs`, `find`, `nc`, `socat`, `wget`, `cat` (file reads use bash redirection or `[ -r ]` tests).

  **Files read** (directly or via tested-by builtins; not via `cat`): `/sys/fs/cgroup/*`, `/proc/<pid>/comm` (K8s pause detection at lines 282-291), `/etc/kubernetes/admin.conf` (KUBE_CONFIG default), `/etc/pve/qemu-server/<id>.conf` (script line 709), `/etc/pve/lxc/<id>.conf` (script line 717), `/var/run/secrets/kubernetes.io/serviceaccount/token` (script line 400, K8s in-cluster), `/run/` (Docker/Podman socket paths under default `DOCKER_HOST`/`PODMAN_HOST`), `${TMPDIR:-/tmp}/netdata-cgroups-*` (3 tmpfile cache paths). The script does NOT read `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` — the K8s API curl calls at lines 407/421 use `curl --fail -sSk` (`-k` = insecure skip TLS verify), so no CA cert is consulted. The Go binary MUST match: `tls.Config{InsecureSkipVerify: true}` for those calls; do NOT load the projected service-account CA cert (doing so would make the binary MORE strict than the script and break clusters with custom/expired CAs that the script today accepts).

  **Files written**: only the 3 tmpfile cache paths under `${TMPDIR:-/tmp}` (non-atomic, per the Tmpfile write semantics bullet below).

  **The FLOW.md flow document MUST include this classification table** (external commands vs builtins) verbatim, so the Go port author cannot accidentally treat a builtin as an external invocation (and vice versa).
- **Writes 3 tmpfiles for K8s cluster-name caching** at `cgroup-name.sh.in:375-377` (verified 2026-05-26 — earlier `/tmp/netdata-cgroups-k8s-*` glob was wrong; only 1 of the 3 paths matches it):
  - `${TMPDIR:-/tmp}/netdata-cgroups-k8s-cluster-name` (line 375)
  - `${TMPDIR:-/tmp}/netdata-cgroups-kubesystem-uid` (line 376)
  - `${TMPDIR:-/tmp}/netdata-cgroups-containers` (line 377)
  These caches persist across invocations. The Go binary MUST preserve the same tmpfile contract: identical paths, identical contents, identical TTL semantics (none — script never expires them; cache lives until `/tmp` is cleaned), identical permissions (umask-default, no explicit chmod).

  **Cache-overwrite-every-miss contract** (verified at `cgroup-name.sh.in:383-470`, documented 2026-05-26): the cache HIT fast-path at lines 383-389 requires (i) all 3 tmpfiles to exist AND (ii) the current cntr_id to be present in the containers file. On ANY cache MISS — including the case where cluster-name and kubesystem-uid are already cached but the current cntr_id is new — the MISS branch at lines 390-470 refetches the pods list AND OVERWRITES all 3 tmpfiles unconditionally at lines 467-469 (no conditional check on whether cluster-name or kubesystem-uid changed). The Go binary MUST preserve this overwrite-every-miss behavior — do NOT optimize by skipping the cluster-name/kubesystem-uid writes when the underlying value is unchanged. This matters for byte-for-byte replay equivalence: a sequence of [resolve container A → resolve container B (new)] rewrites all 3 cache files twice, with the second write potentially producing identical bytes to the first.
- **Tmpfile write semantics are NON-atomic** (corrected 2026-05-26 — earlier "filesystem rename atomicity" claim was factually wrong). The script writes via plain `echo "$value" > "$path" 2> /dev/null` (lines 467-469), which is `open(O_WRONLY|O_CREAT|O_TRUNC) + write + close` — NOT a write-to-tmp-then-rename. A concurrent reader CAN see a zero-length or partially-written file; a concurrent writer CAN interleave writes (last writer wins per fd, but the file is overwritten in place). The Go binary MUST preserve this exact non-atomic behavior — do NOT use `os.Rename` from a tmp file, do NOT take advisory locks, do NOT use `O_EXCL`. Use `os.OpenFile(path, O_WRONLY|O_CREAT|O_TRUNC, 0666)` then `Write` then `Close`, ignoring all errors (matching `2> /dev/null`).
- The script runs as the `netdata` user (master plan confirms).
- Logging uses `systemd-cat-native --log-as-netdata` with structured key/value fields. The Go binary MUST emit the same log fields (`INVOCATION_ID`, `SYSLOG_IDENTIFIER`, `PRIORITY`, `THREAD_TAG=cgroup-name`, `ND_LOG_SOURCE=collector`, `ND_REQUEST`, `MESSAGE`).
- **`systemd-cat-native` invocation mechanism (Option A decided 2026-05-26):** the Go binary invokes `systemd-cat-native --log-as-netdata` as a SUBPROCESS, piping the structured key/value payload on the child's stdin via a heredoc-equivalent, EXACTLY as the script does at `cgroup-name.sh.in:75-83`. The Go binary does NOT implement the systemd journal protocol directly (no in-process journal-protocol library; no TLS / namespace / authentication reimplementation). Rationale: (a) verbatim-port mandate — the script invokes a subprocess, so does the binary; (b) zero implementation risk vs reimplementing the journal protocol; (c) `systemd-cat-native` is a Netdata-supplied C binary always present where Netdata is installed (built at `CMakeLists.txt:3315`, source at `src/libnetdata/log/systemd-cat-native.c`); (d) log emission is not a performance bottleneck (one stderr write per resolution; not in the hot path for the 100-cgroup-discovery latency target). Invocation pattern in Go pseudocode:
  ```go
  cmd := exec.Command("systemd-cat-native", "--log-as-netdata")
  cmd.Stdin = strings.NewReader(payloadKVLines)  // identical bytes to the script's heredoc
  cmd.Stderr = os.Stderr                          // surface systemd-cat-native errors if any
  _ = cmd.Run()                                    // errors swallowed; script doesn't check either
  ```
  The `payloadKVLines` MUST be byte-identical to the script's heredoc body (modulo the explicit exclusions: SYSLOG_IDENTIFIER, first ND_REQUEST token, timestamps, INVOCATION_ID — see Validation gate). `systemd-cat-native` is on the FORBIDDEN list in the In-process replacement scope below.
- **`MESSAGE` newline-escaping (explicit 2026-05-26 per round-8 feedback):** the script at `cgroup-name.sh.in:82` constructs `MESSAGE` via `MESSAGE=${*//$'\n'/\\n}`, which replaces every literal newline byte (0x0A) inside the joined argument string with the two-byte ASCII sequence `\n` (backslash + lowercase n, 0x5C 0x6E). The Go binary MUST replicate this exact transformation via `strings.ReplaceAll(msg, "\n", "\\n")` before adding the `MESSAGE=` line to the systemd-cat-native stdin payload. Do NOT use Go's `%q` formatting, `strconv.Quote`, `fmt.Sprintf` default verbs, or any other escaping helper — they escape additional characters (backslashes, quotes, non-printables) and would diverge from the script's single-substitution behaviour.
- **Signal handling (explicit 2026-05-26 per round-8 feedback):** the script has NO explicit signal handlers (no `trap` statements). On SIGTERM/SIGINT the script and its in-flight subprocess (e.g., `docker inspect`, `kubectl`) are killed by the kernel via the process group. The Go binary MUST match: do NOT call `signal.Notify`, do NOT install custom SIGTERM/SIGINT handlers, do NOT call `signal.Reset`. Rely on Go's default runtime behaviour, which terminates the process on these signals and propagates termination to the process group containing any `exec.Command` children. Rationale: the C caller (`cgroups.plugin` via `spawn_popen_run_variadic` at `cgroup-discovery.c:235`) owns subprocess-lifetime management (its own timeout, its own kill semantics); the binary stays out of that contract.
- **The script reads ALL of these environment variables — the Go binary MUST honour every one** (verified by grep against `cgroup-name.sh.in`):
  - `NETDATA_LOG_LEVEL` (line 32)
  - `NETDATA_INVOCATION_ID` (line 76)
  - `NETDATA_HOST_PREFIX` (lines 242, 245, 707, 709, 715, 717 — used in 5 detection branches; especially critical for containerised Netdata reading host cgroups via prefix)
  - `KUBE_CONFIG` (lines 427, 432) — bash semantics: `[[ -z ${KUBE_CONFIG+x} ]]` at line 432 tests whether the variable is **UNSET** (distinct from set-to-empty-string). If `KUBE_CONFIG=""` (explicitly empty), the `+x` expansion produces `x` (not empty), the `-z` test is false, and the default `/etc/kubernetes/admin.conf` is NOT applied — kubectl is invoked with `--kubeconfig=""` which causes kubectl to use its own defaults (typically `$HOME/.kube/config`). The Go binary MUST distinguish "unset" from "empty string" — `os.LookupEnv("KUBE_CONFIG")` returns `(value, true)` for empty-string-set, `(value, false)` for unset. Only set the default when `LookupEnv` returns `false`.
  - `KUBERNETES_SERVICE_HOST`, `KUBERNETES_PORT_443_TCP_PORT` (line 398 — in-cluster K8s API server)
  - `USE_KUBELET_FOR_PODS_METADATA` (line 413)
  - `KUBELET_URL` (line 414)
  - `MY_NODE_NAME` (line 417)
  - `DOCKER_HOST` (line 645) — DEFAULT VALUE: `unix:///var/run/docker.sock` applied via bash `:=` when unset OR empty. Go binary MUST apply identical default.
  - `PODMAN_HOST` (line 646) — DEFAULT VALUE: `unix:///run/podman/podman.sock` applied via bash `:=` when unset OR empty. Go binary MUST apply identical default.
  - `TMPDIR` (lines 375-377) — used for K8s metadata tmpfile cache directory.
  - `HOME` (round-9 explicit 2026-05-26) — NOT read directly by the script (verified via grep against `cgroup-name.sh.in`: zero matches), but inherited by the script process and propagated to every subprocess via Go's `os.Environ()` equivalent (bash auto-exports it). Critical for `kubectl` invocations at script lines 427 and 433: when `KUBE_CONFIG=""` is set explicitly (empty string), the `[[ -z ${KUBE_CONFIG+x} ]]` test at line 432 is FALSE (variable is set, just to empty), so the `/etc/kubernetes/admin.conf` default is NOT applied; kubectl is invoked with `--kubeconfig=""` and falls back to its OWN built-in default of `$HOME/.kube/config`. The Go binary MUST propagate `HOME` to every `exec.Command` invocation (especially kubectl). If the Go binary ever constructs an explicit `cmd.Env` slice (instead of inheriting `os.Environ()`), `HOME` MUST be included along with `PATH`, `LC_ALL`, and every other variable the parent inherited. Fixture below covers this: a `KUBE_CONFIG=""` + `HOME=/some/test/home` invocation must produce the same kubeconfig-resolution behaviour in script and binary.
  - `PATH` (round-9 explicit 2026-05-26) — read implicitly by `exec.LookPath` and `exec.Command`; the script extends it at line 8 (lines 1-7 are shebang / SPDX / comment / blank; the `export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin:@sbindir_POST@"` statement is on line 8). The Go binary MUST apply the SAME extension via `os.Setenv("PATH", ...)` BEFORE any `exec.LookPath` or `exec.Command` call, with `sbindirPost` injected at build time via `-ldflags -X` (see Resolved policy). The extended PATH is then naturally inherited by every spawned subprocess via Go's default `cmd.Env=nil` (which copies the parent env).
- **The script's main dispatch (lines 656-728) is a SWITCH, not a fallback chain.** A single cgroup matches exactly one detection branch via regex. Parallelism is NOT applicable across detection branches.
- **Within `docker_get_name` (lines 590-607)**: deterministic dispatch by snap/`hash docker` checks, NOT a parallel fallback. The `||` fallback at line 598 (`docker_like_get_name_api DOCKER_HOST "${id}" || docker_like_get_name_command podman "${id}"`) is **GATED**, not "API tried then CLI tried on failure". See next bullet.
- **Within `podman_get_name` (lines 618-632)**: the `||` fallback at line 623 (`docker_like_get_name_api PODMAN_HOST "${id}" || docker_like_get_name_command podman "${id}"`) is **GATED**, not sequential.
- **CRITICAL — `docker_like_get_name_api` return-code semantics** (verified at `cgroup-name.sh.in:147-182`, 2026-05-26): the function returns `1` ONLY in two cases — (a) the host variable is empty (line 153-156), or (b) `jq` is not installed (line 158-161). On EVERY other path (curl failure, jq parse failure, empty OUTPUT, malformed JSON) the function returns `0` (line 181 — unconditional `return 0`). Therefore the `||` operator at lines 598 and 623 invokes `docker_like_get_name_command` **only when jq is missing OR the host var is empty** — not on transient runtime failure of the API call. In the common case (jq installed, host configured) the CLI branch is dead code.
- **Parallelism Scope (corrected 2026-05-26):** The earlier SOW characterisation of lines 598/623 as parallelizable was technically true but **mislead about where speedup comes from**. Because the `||` branches are gated by preconditions that the Go binary can check synchronously (and cheaply — `jq` presence is just a binary lookup, host var is just env), running both branches in parallel saves nothing in the common case. The Go binary MUST:
  - Check the gating preconditions first (host non-empty AND jq available for the API attempt). If either fails, do NOT attempt the API; go straight to the CLI per the script's `||` semantics.
  - If both preconditions hold, attempt the API only. Do NOT fall back to CLI on API failure (the script does not either — `return 0` from `docker_like_get_name_api` does NOT trigger the `||` branch).
  - **The PRIMARY speedup vs the script is in-process execution** of the dozens of `sed`/`grep`/`jq` subprocesses the script spawns per invocation; the API call itself can also become a Go `net/http` call instead of a `curl` subprocess. Parallelism within a single resolution is a tertiary win and only applies to truly-independent calls (e.g., the 3 GCP metadata fetches at lines 267-269 could run in parallel — each has its own `-m 3` timeout, none depends on the others' values, and the script's `&&` short-circuit just means it stops the chain on first failure, equivalent to "cancel siblings on first error").
- **Within `k8s_get_kubepod_name` (lines 398-440)**: the K8s in-cluster API path (lines 398-424, conditioned on `KUBERNETES_SERVICE_HOST`/`KUBERNETES_PORT_443_TCP_PORT`) and the kubectl path (lines 425-436, conditioned on `ps -C kubelet` AND `command -v kubectl`) are MUTUALLY EXCLUSIVE based on environment state — NOT a fallback chain. They MUST NOT be parallelized. Each invocation deterministically uses ONE path based on env.
- The Go binary MUST treat the main dispatch as a switch with explicit precedence per regex, treat the K8s API vs kubectl paths as mutually exclusive, and treat lines 598/623 as gated-by-precondition single-branch dispatches (NOT parallel races, NOT API-with-CLI-fallback-on-failure). The ONLY genuinely parallelizable independent work is the GCP metadata fetch chain at lines 267-269 — and even that must preserve `-m 3` per call and "stop on first failure" semantics.

Inferences:

- The "exact flow" requirement means the first artifact is a structured flow document derived from a line-by-line read of the script. The flow document is the implementation spec; the Go code is a port of the flow document. Reviewers verify the flow document against the script BEFORE any Go code is reviewed.
- Parallelism is bounded by the script's own dependency graph: methods that the script chooses based on prior state (e.g., kubectl path is gated by `ps -C kubelet` having found kubelet) must preserve that ordering. The `||` chains at script lines 598 (`docker_get_name`) and 623 (`podman_get_name`) LOOK like fallback chains but are NOT runnable in parallel: `docker_like_get_name_api` returns 1 ONLY when its preconditions fail (host var empty OR jq missing — script lines 153-161), so the CLI branch is gated by those preconditions, not by API runtime failure. The Go binary checks the gating preconditions synchronously and dispatches to exactly ONE branch — no race, no concurrent try, no first-result-wins selection. The only genuine parallelism is the GCP metadata fetch chain (script lines 267-269) covered above.
- Output (stdout) is the resolved name with optional labels. The exact format is preserved verbatim. The cgroups.plugin C side parses the output (`cgroup_parse_resolved_name_and_labels`) and the parsing assumes a specific format.
- Exit codes are preserved exactly.

Resolved policy (no more Unknowns):

- **No subprocess invocation may be skipped, conditionalised, or reordered** for any reason — including "it looks wasteful". If the script makes a call, the binary makes the same call under the same conditions. Zero algorithmic deviation. This is an absolute prohibition; the "Default answer: NO" framing of earlier drafts is replaced by this rule.
- **The Go binary spawns fresh on each invocation** (no process pool). The binary is invoked once per cgroup-name resolution; parallelism is internal goroutines.
- **`@sbindir_POST@` PATH resolution mechanism (decided 2026-05-26):** the Go binary receives the sbindir component via `-ldflags "-X main.sbindirPost=${sbindir}"` injected by CMake at build time, mirroring how `configure_file` substitutes the same token in the `.in` template. The Go binary constructs `PATH` at startup as `${PATH}:/sbin:/usr/sbin:/usr/local/sbin:${sbindirPost}` using `os.Setenv("PATH", ...)`. This MUST happen BEFORE any `exec.Command` invocation. If `sbindirPost` is empty at build time (test builds), the trailing component is omitted (matching CMake's behavior when the variable is unset).
- **`LC_ALL=C` is mandatory before subprocess invocation.** The Go binary MUST call `os.Setenv("LC_ALL", "C")` at startup BEFORE any `exec.Command` runs, AND MUST also ensure `LC_ALL=C` is in the env passed to each spawned subprocess (Go's `cmd.Env = os.Environ()` propagates it; an explicit `cmd.Env` override MUST include `LC_ALL=C`). Without this, subprocess regex matching (grep), sort order, and sed locale-sensitive substitutions can differ from the script.
- **`get_lbl_val` "null" sentinel (lines 187-205):** the function returns the literal string `"null"` (not empty, not error) when the requested label is missing or has an empty value. This sentinel is consumed by:
  - Name construction at lines 503-505 and 521-522 (producing names like `cntr_null_null_null` when labels are missing).
  - Null-name detection at lines 532-539 (regex `_null(_|$)` triggers warning + return-2-or-1 based on `USE_KUBELET_FOR_PODS_METADATA`).
  The Go binary's port of `get_lbl_val` MUST return the literal Go string `"null"` (4 bytes) on the missing-label path. Do NOT return empty string, do NOT return an error.
- **`k8s_get_kubepod_name` implicit return (lines 541-543):**
  ```bash
  echo "$name"
  [ -n "$name" ]
  return
  ```
  Bash's bare `return` propagates the exit status of the last command, which is `[ -n "$name" ]` (0 if non-empty, 1 if empty). The Go port MUST replicate: emit the name on stdout, then return 0 if name is non-empty, 1 if empty. The downstream `case "$?"` at line 554 dispatches on this code — getting it wrong changes `EXIT_CODE` assignment from EXIT_SUCCESS to EXIT_RETRY to EXIT_DISABLE.
- **`eval` at line 113 is REQUIRED behavior, not a security smell:**
  ```bash
  eval "$(grep -E "^(NOMAD_NAMESPACE|NOMAD_JOB_NAME|NOMAD_TASK_NAME|NOMAD_SHORT_ALLOC_ID|CONT_NAME|IMAGE_NAME)=" <<< "$output")"
  ```
  This grep-filters output to exactly 6 variable names (the regex anchors at `^` and uses `=` as the assignment marker), then `eval`s the result as bash variable assignments. The Go binary MUST extract these 6 variables (and ONLY these 6) from the docker/podman inspect output by parsing lines that match `^(NOMAD_NAMESPACE|NOMAD_JOB_NAME|NOMAD_TASK_NAME|NOMAD_SHORT_ALLOC_ID|CONT_NAME|IMAGE_NAME)=` and storing each in a local map. Do NOT extract other env vars; do NOT use Go's `os.Setenv` (the script does not pollute the parent env, eval just sets local-scope shell variables which are scoped to the function via `local` declarations — verify by inspecting `parse_docker_like_inspect_output`'s call site).
- **SYSLOG_IDENTIFIER and ND_REQUEST decision (Option A, 2026-05-26)**: the script logs `PROGRAM_NAME=$(basename $0)` which evaluates to `cgroup-name.sh` (line 16); the binary at `cgroup-name` naturally produces `cgroup-name`. Decision: the Go binary uses `cgroup-name` (its own name) for `SYSLOG_IDENTIFIER` and inside `ND_REQUEST`. This is an INTENTIONAL byte-level difference vs the script in the stderr log payload. The Validation gate's "byte-for-byte equivalence" requirement excludes these two fields (see Validation gate below — payload comparison ignores the `cgroup-name.sh` → `cgroup-name` token in `SYSLOG_IDENTIFIER` and the first token of `ND_REQUEST`; all other log content must match byte-for-byte). The `cmd_line` construction MUST still use bash's `printf "'%s' "` semantics verbatim:
  - Each argument is wrapped in single quotes.
  - A trailing space follows the LAST argument (printf format includes the trailing space).
  - Single quotes WITHIN an argument are NOT escaped (bash's `printf "'%s' "` produces broken output for arguments containing `'`); the Go binary MUST reproduce this brokenness verbatim — do NOT add escaping.
  Example: argv `[cgroup-name, /docker/abc, docker_abc]` → `ND_REQUEST='cgroup-name' '/docker/abc' 'docker_abc' ` (note trailing space).

### Acceptance Criteria

**In-process replacement scope (binding, resolves verbatim-port vs HTTP contradiction — round-7 fix 2026-05-26):**

The "no subprocess invocation may be skipped, conditionalised, or reordered" rule (Resolved policy bullet at L108) is ABSOLUTE for operations that observe the host environment. The "in-process replacement" rule (Purpose at L17) is BOUNDED to text/JSON processing primitives that are pure functions of their input. This list is exhaustive — anything not listed here is OUT OF SCOPE for replacement and MUST stay as the script invokes it.

**In-process replacement IS PERMITTED for (text/JSON processing primitives only):**
- `sed` — every invocation. Replace with Go `regexp` + string substitution.
- `grep` / `grep -E` — every invocation. Replace with Go `regexp` matching / line iteration.
- `jq` — every invocation EXCEPT the `hash jq` / `command -v jq` presence probes (which are environment-observing — see below). JSON parsing replaced with `encoding/json`. Note: `jq` MUST still appear-to-be-installed when the gating preconditions check it (see below).
- `head -1`, `head -n 1`, plain `head` with a numeric limit — replace with line-count limited Go reading.
- `basename` — replace with `filepath.Base`. The script's only `basename` call is at line 16 on `${0}` for `PROGRAM_NAME`; the Go binary computes this via `filepath.Base(os.Args[0])` (or just hardcodes `"cgroup-name"` per the SYSLOG_IDENTIFIER decision).
- Bash builtins: `echo`, `printf`, `[ ]`/`[[ ]]` tests, parameter expansions, `case`/`if`/`while` — naturally replaced because the Go binary is not bash.
- `curl` invocations that hit HTTP/HTTPS endpoints (Docker socket via HTTP, Podman socket via HTTP, K8s API server, kubelet API, GCP metadata API) — replace with Go `net/http`. **HOWEVER** this replacement carries explicit compatibility tests (see "External-environment compatibility tests" below). The Go HTTP client MUST mimic curl's exact observable behavior on TLS/proxy/socket/error edge cases.

  **BINDING — `curl --fail` vs no-`--fail` semantics (round-9 explicit 2026-05-26).** The script uses TWO distinct curl invocation styles whose observable HTTP-error behaviour differs and MUST be replicated by the Go `net/http` replacement on a per-call-site basis. The mapping is exact:
  - **`curl --fail -s -m 3 --noproxy "*"`** — GCP metadata calls at script lines 267-269. `--fail` makes curl exit ≥22 on any HTTP non-2xx, so the bash `&&` short-circuit at L267-269 STOPS the chain on the first non-2xx. The Go binary MUST treat non-2xx (any 4xx or 5xx) as a hard failure for these 3 calls, abort the chain, leave `kube_cluster_name=""`, and fall through to the `"unknown"` default per script lines 270-275.
  - **`curl --fail -sSk -H "$header" "$url"`** — K8s in-cluster API and kubelet API calls at script lines 407 and 421. `--fail` again — non-2xx becomes a curl-exit-≥22, the bash `if ! kube_system_ns=$(curl ...)` test sees a non-zero exit and runs the warning + early-return path (lines 408-410 for the namespace lookup, lines 422-424 for the pods lookup). The Go binary MUST treat non-2xx on these calls as failure: emit the same warning log line, return the same exit code as the script's branch.
  - **`curl -sS --unix-socket "${address}" "http://localhost${path}"`** — Docker socket API call at script line 171.
  - **`curl -sS "${address}${path}"`** — Docker/Podman TCP API call at script line 174.
    Both Docker/Podman API calls OMIT `--fail`. Without `--fail`, curl exits 0 on HTTP non-2xx and writes the response body (which on errors is typically a small JSON error doc or empty) to stdout. The caller `docker_like_get_name_api` at lines 169-181 then pipes `JSON` into `jq`; if the body isn't a valid Docker inspect response, `jq` fails OR yields an unexpected shape, the OUTPUT is empty, and the function returns 0 anyway (line 181). The Go binary MUST replicate this: do NOT treat non-2xx as a hard error for Docker/Podman API calls; ALWAYS read the response body and pass it to the JSON parser; on parse failure / unexpected shape / empty extract, return 0 with empty NAME (matching script line 181). Specifically: a Docker socket returning HTTP 404 with body `{"message":"No such container"}` MUST be handled the same way the script handles it — body goes to JSON parsing, parsing yields no NAME, function returns 0.
  - **No other curl invocations exist in the script** (verified via grep). Any future change to the script that adds a curl call MUST be re-checked against this `--fail`-vs-no-`--fail` table before the Go binary is updated; do NOT generalise either branch's semantics across call sites.

**In-process replacement IS FORBIDDEN for (environment-observing operations):**
- `docker` CLI invocations (`docker_like_get_name_command` at line 184-185) — MUST use `exec.Command("docker", ...)` with identical argv. The script literally invokes `docker inspect` (or `podman` per the gated fallback). Replacing with a docker SDK call would change which docker context is read, which credentials store is consulted, and which CLI plugins might intercept the call.
- `podman` CLI invocations — same as docker. MUST use `exec.Command("podman", ...)`.
- `kubectl` invocations (script lines 427, 433) — MUST use `exec.Command("kubectl", "--kubeconfig=...", ...)`. The Go binary MUST NOT use `client-go` or any in-process K8s client. kubectl reads kubeconfig merge order, exec auth plugins, OIDC token files, and CSR auth in ways that are environment-specific; replacing kubectl with client-go changes auth behavior on every cluster that uses exec-auth or CSR.
- `snap` / `snap list docker` (script line 593) — MUST use `exec.Command("snap", "list", "docker")`. Snap presence and snap-managed Docker detection is environment-state.
- `ps -C kubelet` (script line 425) — MUST use `exec.Command("ps", "-C", "kubelet")`. The Go binary MUST NOT walk `/proc` directly here; `ps -C` reads `/proc` with specific filtering that depends on the host's `procps` version. Reproducing the exact match semantics in Go is harder than just invoking `ps`.
- `command -v <tool>` / `hash <tool>` presence probes — MUST use `exec.LookPath(<tool>)`. This is an in-process replacement but is listed here because it is environment-observing and the failure semantics must match: `command -v` and `hash` both return non-zero exit when the tool is not on PATH; `exec.LookPath` returns an error. The Go binary MUST check the error and branch identically to the script's `if command -v X` test.
- File reads under `/sys`, `/proc`, `/etc/pve`, `/etc/kubernetes`, `/var/run`, `/run/`, `${TMPDIR}` — MUST use `os.Open`/`os.ReadFile` directly (not via `cat`). This is naturally in-process and matches the script's `[ -f ]`/`[ -r ]`/`[ -d ]` tests.
- **`systemd-cat-native --log-as-netdata`** (script line 75-83) — MUST use `exec.Command("systemd-cat-native", "--log-as-netdata")` with the payload piped on stdin. The Go binary MUST NOT reimplement the systemd journal protocol in-process (no in-process journal library, no namespace/TLS reimplementation). See the Implementation Understanding bullet for the invocation pattern. Rationale: verbatim-port mandate + zero-risk (systemd-cat-native is a Netdata-supplied binary always present at runtime).

**External-environment compatibility tests (binding for curl → net/http replacement):**

For every code path where the script invokes `curl` and the Go binary uses `net/http`, the Validation gate's fixture matrix MUST include compatibility tests proving identical observable behavior on these edge cases:

1. **TLS edge cases** (K8s API server, kubelet API):
   - **The script does NOT verify TLS for K8s API / kubelet calls.** Script lines 407/421 use `curl --fail -sSk` — the `-k` flag disables certificate verification entirely. The script does NOT read or load `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` (verified via grep: zero matches in `cgroup-name.sh.in`). The Go binary MUST replicate this exact behaviour: set `tls.Config{InsecureSkipVerify: true}` on the K8s API and kubelet HTTP clients. Do NOT load the projected service-account CA cert; do NOT use the system trust store. Loading the CA would make the binary MORE strict than the script and would break clusters whose API server presents a cert the script today accepts via `-k` (custom CA, expired, hostname mismatch, etc.). Fixture: in-cluster API call against a kubelet with self-signed/expired/mismatched cert; verify the script (via `curl -k`) and the Go binary BOTH succeed (no TLS error from either) and produce byte-identical name resolution.
   - Same `-k` semantics apply to the GCP metadata calls at lines 267-269 (HTTP, not HTTPS — TLS is not in play; listed here only to forestall confusion).
   - Negative TLS fixture: a malicious / misconfigured K8s API endpoint returns garbage over TLS — both implementations should reach the response body (TLS not verified) and then fail in JSON parsing identically. The point is to prove that ONLY JSON-parse failure (not TLS failure) gates the failure path, matching the script's `curl -k` semantics.
2. **Proxy edge cases**:
   - `HTTPS_PROXY`/`HTTP_PROXY` set in env: the script's curl calls at script lines 267-269 use `--noproxy "*"` (GCP metadata MUST NOT go through a proxy). The Go binary MUST replicate by setting `Transport.Proxy = nil` on the GCP metadata HTTP client (NOT inheriting `http.ProxyFromEnvironment`). Fixture: invoke with `HTTPS_PROXY=http://nonexistent:1` set; verify GCP metadata path still works (binary bypasses proxy) and other HTTP paths still honor the proxy.
   - `NO_PROXY` matching the docker/podman/kubelet socket host: verify Go binary's HTTP client respects `NO_PROXY` for those calls (script's curl reads `NO_PROXY` automatically).
3. **Socket edge cases** (`DOCKER_HOST=unix:///path`, `PODMAN_HOST=unix:///path`):
   - Socket missing (path doesn't exist): both must produce the same stderr+exit. Curl returns "Couldn't connect" (exit 7); `net/http` over a `net.Dial("unix", path)` returns `connect: no such file or directory`. The Go binary's `docker_like_get_name_api`-equivalent MUST return 0 (not 1) on this case, matching `cgroup-name.sh.in:181`.
   - Socket exists but EPERM (netdata user can't open): both must produce the same outcome — return 0, NAME empty, no fallback.
   - Socket exists but server returns HTTP 500 / malformed JSON: same — return 0, NAME empty.
   Fixtures: one per socket-error class (missing, EPERM, 500, malformed JSON), each run against both script and binary; outputs compared.
4. **Error/timeout edge cases**:
   - GCP metadata `-m 3` timeout: fixture already enumerated below. Verify Go binary honors per-call 3s timeout, NOT a single context covering all 3 calls.
   - Docker/Podman HTTP API server returns non-2xx (404 / 500 / 503): the script's Docker/Podman API curl calls (lines 171/174) OMIT `--fail`, so curl exits 0 on non-2xx and writes the response body to stdout (per the binding table at L200-206 above). The Go binary MUST replicate this exactly: do NOT short-circuit on `resp.StatusCode != 200`; ALWAYS read the response body and feed it to the JSON parser; on parse failure, missing Name field, or empty extract, the function returns 0 with empty NAME (matches script line 181's unconditional `return 0`). Fixture per status code class — including the corner case where a 404 body happens to parse as JSON but has no Name field (must yield empty NAME, return 0), and a 200 with malformed JSON (must yield empty NAME, return 0). Do NOT use a `StatusCode != 200 → curl-failure` shortcut: that would skip the JSON parser and diverge from the script on bodies that happen to be parseable.
   - Connection RST mid-response: same — return 0.
   - Slow Loris (server accepts but never responds, no per-call timeout set): the script's curl on docker/podman/kubelet calls has NO explicit timeout in the script. The Go binary MUST also have no timeout on those calls (a hang propagates upstream; `cgroups.plugin` handles it via its own subprocess timeout). Verify Go binary's net/http call honors no client timeout for these paths.
5. **kubectl compatibility tests** (kubectl is NOT replaced; it stays a subprocess):
   - Exec-auth plugin in kubeconfig: invoke against a cluster whose kubeconfig has `exec:` auth (e.g., aws-iam-authenticator); verify the script and the Go binary both successfully shell out to kubectl which then shells out to the auth plugin.
   - OIDC token with refresh: same shape.
   - Empty `KUBE_CONFIG=""` vs unset: fixture already enumerated below.
   - `KUBE_CONFIG` pointing to non-existent file: kubectl errors; verify both script and binary handle identically.
6. **Docker/Podman CLI compatibility tests** (CLI not replaced; stays a subprocess):
   - Docker context configured to remote host: `docker inspect` reads the active context; verify both script and binary invoke `docker inspect` with the same argv and inherit the same context.
   - Podman rootless socket auto-discovery (`XDG_RUNTIME_DIR`): verify the Go binary inherits and propagates `XDG_RUNTIME_DIR` to the `podman` subprocess.

**These compatibility tests are NOT a future enhancement.** Every test enumerated above is part of the Validation gate's fixture matrix and MUST pass before the Go binary replaces the script.

**Process gate (binding before any Go code is written):**

- A **flow document** is produced at `src/collectors/cgroups.plugin/cgroup-name/FLOW.md` that captures the script's algorithm line-by-line:
  - Every function with its inputs and outputs.
  - Every external command invocation, with its arguments and how the output is parsed.
  - Every conditional branch and the conditions for taking it.
  - Every environment variable read and where it influences flow (full enumeration per Implementation Understanding).
  - Every regex pattern (verbatim).
  - Every BASH_REMATCH capture group and its semantic meaning.
  - Every sed substitution pipeline (verbatim) and its bash-equivalent.
  - Every bash parameter expansion: `${var//pat/repl}`, `${var/pat/repl}`, `${var:0:N}`, `${var//\//_}`, `${#var}`.
  - Every `eval "$(grep -E ...)"` dynamic variable assignment (line 113 — extracting env vars from docker inspect output).
  - Every `cmd_line` construction with single-quote escaping (line 11) — needed for ND_REQUEST log field.
  - Every fallback chain WITHIN A FUNCTION with the exact priority order. The main dispatch (lines 656-728) is a SWITCH, not a fallback chain — document it as such.
  - Every exit code and the conditions for emitting it.
  - Every log line (format, fields, when emitted). Document explicitly that `systemd-cat-native --log-as-netdata` is invoked as a SUBPROCESS (script line 75-83) with the structured key/value payload piped on stdin via heredoc; the Go binary preserves this subprocess invocation (Option A). Document the `MESSAGE` newline-escaping verbatim — `${*//$'\n'/\\n}` at script line 82 maps each literal newline (0x0A) to the two-byte sequence `\n` (0x5C 0x6E); the Go port uses `strings.ReplaceAll(msg, "\n", "\\n")`, NOT `%q` or `strconv.Quote`.
  - Every tmpfile path written and its TTL/refresh semantics (`/tmp/netdata-cgroups-k8s-*` per lines 375-377).
  - Every output transform with its exact scope: (i) 100-char truncation at line 729 is INSIDE the `[ -z "${NAME}" ]` block (lines 668-730). ONLY K8s-resolved names (assigned at script lines 564/568/573/578/583 from the L662-666 K8s top-block BEFORE the L668 gate) skip the truncation. Docker/podman-resolved names DO get truncated — the dispatcher elif's at L669/L674/L679/L684 enter the L668 block before invoking `docker_validate_id`/`podman_validate_id` → `docker_get_name`/`podman_get_name`, so NAME assignments at L603/L605/L628/L630 land inside the L668 block and L729 applies on the way out. Same for systemd-nspawn / lxc / qemu / Proxmox / lxc.payload / fall-through. (ii) space→underscore at line 732 is UNCONDITIONAL and applies to every final NAME. Document the scope explicitly so the Go port preserves the K8s-skip vs everything-else-truncated split.
  - Initial environment setup: `export LC_ALL=C` (line 9) and `export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin:@sbindir_POST@"` (line 8; lines 1-7 are shebang / SPDX / comment / blank). The Go binary MUST apply these BEFORE any subprocess invocation, AND inject `@sbindir_POST@` via `-ldflags -X` (decision recorded above).
  - Input transform `CGROUP="${2//\//_}"` at line 648 — second argument has all `/` replaced with `_` before use.
  - `docker_validate_id` behaviour on failure (lines 609-616): emits `error()` log line but does NOT set `EXIT_CODE`; script falls through to line 732 with `EXIT_CODE=$EXIT_SUCCESS` (the default from line 652) and `NAME` empty/unchanged. The Go binary MUST preserve this exact semantic — docker cgroup with invalid ID exits 0 with NAME equal to the (possibly transformed) cgroup id.
  - Same nuance for `podman_validate_id` (lines 634-641), including the typo "a podman id cannot be extracted from docker cgroup" (line 639) which MUST be preserved verbatim in the error log payload.
  - `cmd_line` construction at line 11 — single-quote wrapping, trailing space, NO single-quote escaping inside arguments. All three behaviors verbatim.
  - `get_lbl_val` (lines 187-205) — returns literal string "null" on missing label.
  - `k8s_get_kubepod_name` implicit return (lines 541-543) — `return` propagates `[ -n "$name" ]` exit status.
  - `eval` at line 113 — extracts ONLY the 6 named env vars from inspect output; required behavior.
  - `docker_get_name` dispatch (lines 593-599): `command -v snap && snap list docker` → API; `hash docker` → CLI; else `||` chain GATED by docker_like_get_name_api's return-1 preconditions (jq missing OR DOCKER_HOST empty). Document the gating, NOT a sequential try-then-fallback-on-failure.
  - `podman_get_name` dispatch (line 623): same gating semantics.
  - `docker_like_get_name_api` return-code table (line 147-182): return 1 ONLY when host var is empty OR jq is missing; return 0 in ALL other cases (including curl failure, jq parse failure, empty OUTPUT).
  - GCP metadata calls at lines 267-269 — `curl --fail -s -m 3 --noproxy "*"` per call; bash `&&` short-circuit on first failure; verbatim required including the `-m 3` timeout.
  - Tmpfile writes at lines 467-469 — plain `echo > file 2> /dev/null`; non-atomic; errors swallowed.
- Reviewers verify the flow document against the script. Discrepancies are fixed in the flow document, not in the script.
- The flow document is the implementation spec for the Go code. **No Go code is written before the flow document is signed off** by the multi-reviewer protocol.

**Implementation gate:**

- The Go binary at `src/collectors/cgroups.plugin/cgroup-name/` (per SOW-0032 D9) implements the flow document verbatim.
- **Go module placement decision (2026-05-26):** the binary is its OWN Go module — `src/collectors/cgroups.plugin/cgroup-name/go.mod` and `src/collectors/cgroups.plugin/cgroup-name/go.sum`. It is NOT folded into the existing `src/go/` module tree. Rationale: cgroup-name has zero shared deps with go.d collectors; isolating it keeps the binary lean (Go's static linking still pulls only what `main` imports) and keeps the dependency graph readable for maintainers.
- **Build system: CMakeLists.txt uses the existing `add_go_target` macro** from `packaging/cmake/Modules/NetdataGoTools.cmake` (mirroring how `go-plugin`, `nd-mcp-target`, `scripts-plugin`, and `topology-ip-intel-downloader-target` are built).

  **CRITICAL macro-extension caveat (flagged round-5, 2026-05-26):** the current `add_go_target` macro takes EXACTLY 4 positional arguments (`target output build_src build_dir`) and hardcodes `-ldflags "${GO_LDFLAGS}"` internally; any 5th-and-beyond argument passed at the call site is collected into `${ARGN}` and **silently dropped** (the macro never references `${ARGN}` when building the `go build` command line). The naive snippet below WILL NOT inject the `sbindirPost` value as written — the `-ldflags ...` portion is unused.

  The implementation work for this SOW therefore includes extending `NetdataGoTools.cmake`'s `add_go_target` to accept extra ldflags as an optional argument (e.g., a 5th positional `EXTRA_LDFLAGS` parameter, or a keyword-style `LDFLAGS "..."` pair parsed via `cmake_parse_arguments`) and append them to the per-target ldflags string alongside `GO_LDFLAGS`. Extension must be backward-compatible with all existing `add_go_target` call sites in the repo (grep before changing).

  The intended call site, AFTER the macro is extended:
  ```cmake
  # PSEUDOCODE — actual signature depends on the extension chosen
  add_go_target(cgroup-name-target cgroup-name
                src/collectors/cgroups.plugin/cgroup-name .
                EXTRA_LDFLAGS "-X main.sbindirPost=${sbindir_POST}")
  install(PROGRAMS ${CMAKE_BINARY_DIR}/cgroup-name
          COMPONENT netdata
          DESTINATION usr/libexec/netdata/plugins.d)
  ```
  Before committing: (i) read `NetdataGoTools.cmake` at HEAD to confirm the exact macro signature; (ii) implement the macro extension; (iii) verify with `cmake --trace-expand` (or a temporary `message(STATUS ...)`) that the ldflags string actually contains `-X main.sbindirPost=...` in the final `go build` invocation; (iv) verify the resulting binary's `main.sbindirPost` is populated at runtime via a small smoke test.
- Install rule places the binary at `${libexecdir}/netdata/plugins.d/cgroup-name` (no `.sh` suffix).
- `cgroups.plugin` is changed to set the default resolver path at `sys_fs_cgroup.c:406-407` from `cgroup-name.sh` to `cgroup-name`. The user-facing INI key remains the legacy `"script to get cgroup names"` string for backward compatibility; changing it would make existing `netdata.conf` overrides invisible.
- `cgroup-name.sh.in` is REMOVED from the source tree. The following references MUST also be removed or updated in the same SOW (verified via repo-wide grep, 2026-05-26):
  - `CMakeLists.txt:3671-3676` — `configure_file` + `install(PROGRAMS ...)` rules.
  - `netdata.spec.in:631` — rpm packaging manifest entry `%{_libexecdir}/%{name}/plugins.d/cgroup-name.sh`.
  - `.gitignore:93` — `src/collectors/cgroups.plugin/cgroup-name.sh` (gitignore entry for the generated file).
  - `src/collectors/cgroups.plugin/sys_fs_cgroup.c:406-407` — default path constant pointing to `cgroup-name.sh`; update to `cgroup-name`.
  - `src/collectors/cgroups.plugin/README.md` — search and update all references (verified at least lines 104, 111).
  - `src/collectors/cgroups.plugin/cgroup-discovery.c:826` — comment containing GitHub permalink to the shell script (`https://github.com/netdata/netdata/blob/0fc101679dcd12f1cb8acdd07bb4c85d8e553e53/collectors/cgroups.plugin/cgroup-name.sh#L121-L147`). **Resolution decision (2026-05-26):** replace the permalink with a reference to the new FLOW.md document — `src/collectors/cgroups.plugin/cgroup-name/FLOW.md#parse_docker_like_inspect_output` (or equivalent anchor). The existing permalink stays valid (it's a frozen-commit URL) but readers should be directed to the live spec.
  - `CHANGELOG.md:2441` — `Feat(cgroup-name.sh): Add support for ...`. **Resolution decision:** leave unchanged. CHANGELOG entries describe historical state; changing them rewrites history. The Go binary replacement gets its own CHANGELOG entry when this SOW ships.
  - `src/libnetdata/sanitizers/chart_id_and_name.c:78` — comment mentioning `cgroup-name.sh`; update to `cgroup-name`.
  - `src/collectors/guides/proxmox/integrations/proxmox_ve_monitoring.md:137` — Proxmox guide references cgroup-name.sh; update binary name.
  - `src/collectors/guides/proxmox/metadata.yaml:133` — Proxmox metadata references cgroup-name.sh; update binary name.
  - `src/collectors/cgroups.plugin/cgroup-internals.h:163` — **round-7 addition 2026-05-26**. Comment reads `// by default this is the *id (path), later changed to the resolved name (cgroup-name.sh) or systemd service name.` Update `cgroup-name.sh` → `cgroup-name` to reflect the binary replacement.
  - `src/libnetdata/inicfg/MIGRATE_TO_YAML_WIP.md:1312` — **round-7 evidence-backed decision 2026-05-26**. This file is a historical migration WIP ledger (filename explicitly contains `MIGRATE_TO_YAML_WIP`); its purpose is to record the legacy `[plugin:cgroups]` config-knob wording (`script to get cgroup names`) for migration planning to the future YAML config format. Decision: KEEP the historical wording unchanged. Rewriting historical migration-source wording would distort the migration record (the legacy knob was named for a script, even if the implementation now sits behind a binary; the migration mapping must record the legacy state as it was). Add a single brief footnote/marker line near 1312 noting the legacy knob is now satisfied by `cgroup-name` (a binary, not a script) — without altering the original wording. This preserves the migration ledger's integrity while still making the discrepancy discoverable.
- The following references were CLMED in earlier drafts but DO NOT exist (verified via grep) — no work needed:
  - `packaging/cmake/pkg-files/deb/plugin-cgroups/postinst` — no such file; no deb plugin-cgroups package exists.
  - `packaging/makeself/install-or-update.sh` — does NOT reference `cgroup-name.sh`.
  - No snap or container packaging variant references the script.
- The shell script invocation pattern via `spawn_popen_run_variadic` is unchanged; only the target executable changes. No netdata daemon code changes outside the path constant and the description string.

**Validation gate:**

- A **fixture matrix** of `(stdin cgroup-id, system state, env vars)` → expected `(stdout, exit code, stderr log lines)` is captured by running the shell script on a controlled test environment covering every detection branch. The matrix MUST cover at minimum (verified against `cgroup-name.sh.in` lines 656-728, 2026-05-26):
  - K8s on systemd cgroup driver (path includes `kubepods.slice`).
  - K8s on cgroupfs driver (path includes `kubepods/`).
  - K8s with CRI = docker (`docker-<HEX>.scope` under kubepods).
  - K8s with CRI = cri-o (`crio-<HEX>.scope` under kubepods).
  - K8s with CRI = cri-containerd (`cri-containerd-<HEX>.scope` under kubepods).
  - K8s pause container (script lines 366-368, must skip permanently with **EXIT_DISABLE = exit code 3**, NOT EXIT_RETRY).
  - K8s with jq missing (script lines 370-372, returns 1 internally → `k8s_get_name` handler at lines 572-575 sets `NAME="k8s_${id}"` and `EXIT_CODE=$EXIT_SUCCESS` — NOT a skip; enable with fallback name).
  - K8s with USE_KUBELET_FOR_PODS_METADATA=1 (line 413, kubelet direct API path).
  - K8s with GKE-style cluster-name lookup via metadata API (lines 263-275).
  - Docker root daemon (cgroup `/docker/<HEX>`).
  - Docker rootless (cgroup under `user.slice/docker-*.scope`).
  - Docker via snap (script lines 645, snap-installed docker variant).
  - Docker via HTTP API on TCP (DOCKER_HOST=tcp://...).
  - Docker inspect returning Nomad labels (`NOMAD_NAMESPACE`, `NOMAD_JOB_NAME`, `NOMAD_TASK_NAME`, `NOMAD_SHORT_ALLOC_ID` per lines 113-115).
  - Docker inspect returning netdata.cloud/* labels (lines 124-133).
  - **Amazon ECS containers** (path matches `^.*ecs[-_/\.][a-fA-F0-9]+[-_\.]?.*$` per lines 674-678; calls docker_validate_id).
  - **containerd under systemd** (path matches regex `system.slice_containerd.service_cpuset_[a-fA-F0-9]+[-_\.]?.*$` at line 679, ID extracted via sed pattern `^.*ystem.slice_containerd.service_cpuset_\([a-fA-F0-9]\+\)[-_\.]\?.*$` at line 682 — note the **`ystem.slice` typo in the sed**, NOT `system.slice`; fixture must verify the Go binary uses the exact (buggy) sed pattern and still extracts correctly).
  - **Podman libpod root** (cgroup `/machine.slice/libpod-<HEX>.scope`) — matches the L684 `libpod-[a-fA-F0-9]+` regex; extracts the HEX via sed pattern with optional `conmon-` prefix.
  - **Podman libpod-conmon** (cgroup containing `libpod-conmon-<HEX>`) — same regex matches; sed extraction strips the `conmon-` prefix via the `\(conmon-\)\?` capture, yielding the bare HEX. Verify the Go binary preserves the optional-capture semantics.
  - **Podman libpod rootless** (cgroup under `user.slice/user-1000.slice/.../libpod-<HEX>.scope`) — same L684 regex matches; verify extraction works for the user-slice path shape.
  - **Podman `podman-<HEX>` fall-through** (cgroup `/system.slice/podman-<HEX>.scope` or `user.slice/user-1000.slice/.../podman-<HEX>.scope`) — round-7 fix 2026-05-26: the script at `cgroup-name.sh.in:684-687` ONLY matches `libpod-<HEX>` / `libpod-conmon-<HEX>`. The `podman-` prefix (without `libpod-`) does NOT match any podman branch and falls through to the default at line 728 (`NAME="${CGROUP}"`) with truncation at line 729 if >100 chars and space→underscore at line 732. Expected NAME = the cgroup path with `/` already replaced by `_` (per the L648 input transform), possibly truncated to 100 chars. Exit 0. This fixture proves the Go binary does NOT add a `podman-<HEX>` detection branch that the script lacks.
  - **Podman via HTTP API** (PODMAN_HOST set) — cgroup must still match the L684 `libpod-` regex to reach `podman_validate_id` → `podman_get_name` → API path. Otherwise the PODMAN_HOST is irrelevant (the cgroup falls through).
  - systemd-nspawn (`/machine.slice/systemd-nspawn@<name>.service` per lines 689-691).
  - LXC via libvirt (`machine.slice_machine.*-lxc` per lines 693-697).
  - LXC 4.0 (`lxc.payload.*` per lines 723-725).
  - **Proxmox VMs** (`qemu.slice_<id>.scope` + `/etc/pve/qemu-server/<id>.conf` per lines 707-714).
  - **Proxmox LXC** (`lxc_<id>` + `/etc/pve/lxc/<id>.conf` per lines 715-722).
  - QEMU via libvirt-qemu (`machine_.*.libvirt-qemu` per lines 703-705).
  - QEMU/KVM via libvirt machine.slice (`machine.slice_machine.*-qemu` per lines 698-701).
  - **NOTE on plain systemd services**: the shell script does NOT contain a detection branch for plain `/system.slice/*.service` cgroups — they fall through to `NAME="${CGROUP}"` at line 728. The systemd-service friendly-name conversion happens in C code at `cgroup-discovery.c:932-934`, NOT in the script. Therefore "plain systemd service" is NOT a fixture for the Go binary — the binary returns the cgroup ID unchanged for these paths.
  - Cgroup paths that match no detection (script falls through to default name per line 728).
  - **Fall-through cgroup ID >100 chars triggers truncation (line 729, INSIDE the `[ -z $NAME ]` block)**: cgroup that matches no detection branch; fall-through NAME assignment at line 728 yields a >100-char string; truncation applied at line 729; verify final NAME is exactly 100 chars.
  - **K8s-resolved NAME >100 chars NOT truncated (CRITICAL — proves the truncation-scope contract)**: K8s pod whose constructed `pod_<name>_<container>` exceeds 100 chars (e.g., very long namespace + pod + container names). Expected: NAME assigned by `k8s_get_name` BEFORE entering the `[ -z $NAME ]` block at line 668; the line 729 truncation is therefore SKIPPED entirely; only the line 732 space→underscore is applied. Final NAME may exceed 100 chars. Run the same fixture against the shell script to confirm; the Go binary must reproduce identical (untruncated) output.
  - **Docker-resolved NAME >100 chars IS truncated (CRITICAL — proves docker/podman path enters the L668 block)**: docker container whose inspect output yields a >100-char composite NAME (e.g., long CONT_NAME with Nomad-prefix labels). Expected: NAME assigned inside `docker_get_name` (script L603/L605) which is invoked from the dispatcher elif at L669/L674/L679 INSIDE the L668 block; control falls through to L729; final NAME is truncated to exactly 100 chars. Run the same fixture against the shell script to confirm; the Go binary must reproduce identical (truncated) output.
  - **Podman-resolved NAME >100 chars IS truncated**: same shape for a podman container whose `podman_get_name` (L628/L630) produces a long composite NAME; dispatcher elif at L684 lives inside the L668 block; final NAME truncated to 100 chars.
  - **systemd-nspawn / libvirt / Proxmox / lxc.payload NAME >100 chars IS truncated**: at least one fixture in this group (e.g., Proxmox VM with `name:` field >100 chars in `/etc/pve/qemu-server/<id>.conf`) to verify the entire L668-block family is subject to L729.
  - Cgroup IDs that contain spaces (must convert to underscores per line 732). This applies to ALL final NAMEs regardless of resolution path (K8s, docker, podman, fall-through). Verify at least one fixture per resolution path with a space in the resolved NAME.
  - **Docker ID validation failures**: docker-prefixed cgroup whose extracted ID is NOT 12 chars AND NOT 64 chars (script `docker_validate_id` at lines 609-616 emits error and leaves NAME empty).
  - **Podman ID validation failures**: podman-prefixed cgroup whose extracted ID is NOT 64 chars (script `podman_validate_id` at lines 634-641 emits error and leaves NAME empty).
  - **Docker `||` CLI fallback via `podman` command (line 598)** — gating-semantics fixture (corrected 2026-05-26 — earlier "tries Docker API, then falls back to `podman` CLI on failure" wording was WRONG; the `||` is gated by preconditions, NOT runtime failure). Preconditions: neither `snap docker` nor `hash docker` succeeds, so the `else` branch at line 597-599 is taken. The `||` fires ONLY when `docker_like_get_name_api` returns 1 — which happens ONLY when (a) `DOCKER_HOST` is empty OR (b) `jq` is missing (per script lines 153-161). On every other path (curl failure, jq parse failure, empty OUTPUT) the API function returns 0 and the `||` does NOT fire. Verify the Go binary: (i) checks the gating preconditions synchronously and skips the CLI when API succeeds; (ii) when the CLI branch fires, invokes `podman inspect` (NOT `docker inspect` — the script literally uses `podman` as the command in this fallback, even for a Docker cgroup). This fixture overlaps with the docker-jq-missing and DOCKER_HOST="" fixtures below; treat those as the canonical drivers and use this one to cross-check the snap-absent + docker-absent precondition.
  - **K8s API vs kubectl path selection**: separate fixtures for (a) `KUBERNETES_SERVICE_HOST` set → in-cluster API path; (b) kubelet+kubectl present → kubectl path; (c) neither → warning emitted.
  - **K8s helper-container skip (kubevirt)**: virt-launcher pods with `volumerootdisk` or `guest-console-log` containers skip with exit code 3 (lines 489-496).
  - **K8s null-name detection**: namespace/pod_name/container_name resolved to "null" string (lines 532-539); behaviour depends on `USE_KUBELET_FOR_PODS_METADATA` (return 2 vs return 1).
  - **K8s missing kubectl with kubelet present**: `ps -C kubelet` succeeds but `command -v kubectl` fails — script falls to warning at line 438.
  - **K8s jq parse failure of cluster name**: jq returns error parsing namespace UID; warning emitted, `kube_system_uid` stays empty; downstream `cluster_id` label not set (line 442).
  - **KUBE_CONFIG empty-string semantics**: `KUBE_CONFIG=""` (explicitly set to empty) is treated differently from unset per `${KUBE_CONFIG+x}` check at line 432. Fixture must cover both cases.
  - **NETDATA_HOST_PREFIX**: fixtures covering both unset (default empty-string expansion) and set to a non-empty prefix path (e.g., `/host`).
  - **Label values with commas/quotes/backslashes**: at least one fixture with a label value containing a comma to verify the C parser (`cgroup-discovery.c:184` splits on commas) does not diverge between shell and Go output.
  - **K8s kubepods root cgroup** (script line 334-336): cgroup path resolves to exactly `"kubepods"` after `.slice`/`.scope` stripping. Expected: `NAME="kubepods"`, no labels, exit 0.
  - **K8s kind cluster** (script comment lines 309-314): cgroup path under `kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod<UID>.slice/cri-containerd-<HEX>.scope`. The `kubepods` substring in the path matches the gate at line 324; verify `clean_id` processing (lines 329-351) handles the `kubelet-kubepods` prefix correctly.
  - **Proxmox VM config with multiple `name:` lines** (script line 711, `grep -e '^name: ' ... | head -1`): config file containing two or more `name: <value>` lines; verify Go binary picks only the FIRST (matching `head -1` semantics).
  - **Proxmox LXC config with multiple `hostname:` lines** (script line 719): same shape, `hostname:` lines.
  - **Docker with jq missing** (`docker_like_get_name_api` line 158-161 returns 1): Docker cgroup + `jq` not on PATH → triggers the `||` CLI fallback at line 598 / 623. Verify the Go binary's gating logic.
  - **Podman with jq missing**: same as above but for podman cgroup at line 623.
  - **Docker with DOCKER_HOST set to empty string** (line 153-156 returns 1): explicitly `DOCKER_HOST=""` → "No DOCKER_HOST is set" warning + return 1 → CLI fallback.
  - **Docker with curl/jq runtime failure** (line 181 returns 0): jq present, host configured, but curl fails OR jq parse fails. Expected: `docker_like_get_name_api` returns 0, NAME stays empty, the `||` CLI branch does NOT fire (gating is on preconditions, not runtime failure), `docker_get_name` falls through to its post-call check at line 600 → `EXIT_CODE=$EXIT_RETRY`, `NAME="${id:0:12}"`. This is the critical fixture proving the gating semantics.
  - **GCP metadata timeout** (lines 267-269): one or more of the 3 metadata API calls times out at 3 seconds; `kube_cluster_name` falls back to `"unknown"`; downstream `cluster_name` label is NOT set (per line 501 condition `!= "unknown"`).
  - **`cmd_line` with arg containing single-quote**: argv `[binary, "/odd/path", "weird'arg"]` → ND_REQUEST log field contains the broken bash-printf output `'binary' '/odd/path' 'weird'arg' ` (note: single-quote not escaped — verifies the Go binary preserves the script's brokenness).
  - **`cmd_line` trailing space**: any fixture — verify ND_REQUEST log field ends with a trailing space.
  - **Tmpfile cache hit on container_id** (lines 383-389): pre-populated `netdata-cgroups-containers` file containing the container_id → cache hit path bypasses kubectl/API calls. Verify Go binary reads cache identically.
  - **Tmpfile cache miss with kubesystem-uid file present but cluster-name empty**: partial cache state; verify Go binary's read-with-fallback to `k8s_gcp_get_cluster_name`.
  - **TMPDIR set to non-default path** (lines 375-377): `TMPDIR=/var/cache/netdata` → tmpfiles written under that path, not `/tmp`.
  - **K8s name containing `null` token via `get_lbl_val`** (line 532-539): pod with missing namespace label → `get_lbl_val` returns "null" → `name="cntr_null_..."` → matches `_null(_|$)` regex → warning + EXIT_RETRY (if USE_KUBELET_FOR_PODS_METADATA set) or return 1 (otherwise).
  - **Binary called without cgroup argument (script line 658-660 → `fatal()` → exit 1)**: invoke with no positional args (or empty `$2`). Expected: stderr `fatal` log line "called without a cgroup name. Nothing to do.", exit code 1, no stdout. Verifies the only exit-1 path in the script.
  - **K8s QoS-suffix top-level cgroup (script lines 336-340)**: cgroup path resolves to exactly `kubepods.slice/kubepods-besteffort.slice` (or `-burstable.slice`, `-guaranteed.slice`) with NO pod beneath. After `.slice`/`.scope` stripping, `clean_id` ends in `besteffort`/`burstable`/`guaranteed`; lines 339-340 apply `name=${clean_id//-/_}` then `name=${name/#kubepods_kubepods/kubepods}`. Verify the Go binary emits the same simplified name (e.g., `kubepods_besteffort`). One fixture per QoS class.
  - **K8s pod regex elif's — separate fixtures per branch (script lines 341, 344, 347)** — round-7 correction 2026-05-26: the script has THREE distinct elif's at lines 341/344/347 but only ONE of them resolves pod_uid alone; the other two resolve container ID. Verified by re-reading `cgroup-name.sh.in:341-351`:
    - **Line 341** = CRI-prefixed CONTAINER ID branch. Regex `.+pod[a-f0-9_-]+_(docker|crio|cri-containerd)-([a-f0-9]+)$`. Captures `cntr_id` from `BASH_REMATCH[2]` (the CRI-suffixed container ID hex). Comment: `...pod<POD_UID>_(docker|crio|cri-containerd)-<CONTAINER_ID> (POD_UID w/ "_")`. Build a fixture per CRI (docker, crio, cri-containerd) where the path embeds an underscore-form POD_UID followed by `_docker-<hex>` / `_crio-<hex>` / `_cri-containerd-<hex>`; verify `cntr_id` extraction.
    - **Line 344** = NON-CRI CONTAINER ID branch. Regex `.+pod[a-f0-9-]+_([a-f0-9]+)$`. Captures `cntr_id` from `BASH_REMATCH[1]` (the trailing container ID hex with no CRI prefix). Comment: `...pod<POD_UID>_<CONTAINER_ID>`. Build a fixture where the path is `…/pod<UID-DASHED>_<CONTAINER_ID>` (no `_docker-` / `_crio-` / `_cri-containerd-` between pod and container); verify `cntr_id` extraction.
    - **Line 347** = POD_UID-ONLY branch. Regex `.+pod([a-f0-9_-]+)$`. Captures `pod_uid` from `BASH_REMATCH[1]` (POD_UID with `_` and `-` chars). Comment: `...pod<POD_UID> (POD_UID w/ and w/o "_")`. Line 349 then normalises `_` → `-` via `pod_uid=${pod_uid//_/-}`. Build one fixture per POD_UID shape (underscore-form vs dashed-form) where the path ends in `…/pod<UID>` with NO trailing container ID; verify `pod_uid` extraction and the `_`→`-` normalisation.
    Total: THREE fixtures minimum, ONE per elif. The Go port MUST keep the three branches distinct (different `BASH_REMATCH` index, different capture target, different downstream lookup path) — do NOT merge them into a single regex.
  - **Docker inspect with Nomad PARTIAL env vars (script lines 113-118 fall-through to CONT_NAME)**: docker inspect output contains, e.g., `NOMAD_NAMESPACE=default` and `NOMAD_JOB_NAME=myjob` but NO `NOMAD_TASK_NAME` and NO `NOMAD_SHORT_ALLOC_ID`. Script's `[ -n "$NOMAD_NAMESPACE" -a -n "$NOMAD_JOB_NAME" -a -n "$NOMAD_TASK_NAME" -a -n "$NOMAD_SHORT_ALLOC_ID" ]` is false → NAME falls back to `${CONT_NAME}` with leading `/` stripped. Verify the Go binary's all-4-required precondition and the fall-through to CONT_NAME.
  - **Docker inspect with MULTIPLE `netdata.cloud/*` labels coexisting (script lines 122-133)**: inspect output contains, e.g., `LABEL_netdata.cloud/foo=bar`, `LABEL_netdata.cloud/baz=qux`, and an `image=...` line. The script iterates over the loop appending each to LABELS (comma-joined). Verify the Go port (i) preserves the iteration order from the inspect output, (ii) comma-joins multiple labels without trailing comma, (iii) interleaves correctly with the image= prefix that appears first.
  - **Docker inspect label with `=` in value (script line 124: `IFS="=" read -r lname lval`)**: label like `LABEL_netdata.cloud/url=https://example.com/path?k=v`. Bash's `read -r lname lval` with `IFS="="` puts the FIRST `=`-split as `lname` and the ENTIRE REMNDER (including subsequent `=` chars) as `lval`. Verify the Go port splits on the FIRST `=` only — not on every `=`. Label value in LABELS must be `https://example.com/path?k=v` (with the `?k=v` preserved verbatim).
  - **`NETDATA_LOG_LEVEL=debug` debug() emission (script lines 105-107, LOG_LEVEL=7)** — round-8 addition 2026-05-26. Run a Docker-resolution fixture with `NETDATA_LOG_LEVEL=debug` set; the script's `debug()` helper emits a DEBUG-priority log line via `systemd-cat-native` whenever it is called along the resolution path. Verify the Go binary: (i) parses `NETDATA_LOG_LEVEL=debug` identically (same string match — the script's `case` at line 32 maps the literal token `debug` to LOG_LEVEL=7); (ii) emits the same DEBUG-priority log lines from the same call sites; (iii) when `NETDATA_LOG_LEVEL` is unset or set to a non-debug value, the same Docker fixture produces NO debug lines (proves the level filter works). Captures the full stderr log payload for byte-for-byte comparison vs the script (modulo the explicit exclusions in the equivalence rule below).
  - **`MESSAGE` newline-escaping fixture (script line 82 `${*//$'\n'/\\n}`)** — round-8 addition 2026-05-26. Trigger an error log path whose payload includes a literal newline byte (e.g., a Docker inspect output with embedded `\n` that flows into an error message via `error()` wrapping). Verify the Go binary's `MESSAGE=` field contains the two-byte sequence `\n` (0x5C 0x6E) — NOT a literal newline, NOT `\\n` (a 3-byte `\n` from Go's `%q` over-escaping), NOT `\012` (Go's `strconv.QuoteAscii` octal). Byte-compare the captured stderr from script vs binary.
- Each fixture must capture: (i) full env (including `NETDATA_HOST_PREFIX`, all kubelet/kube env, DOCKER_HOST, PODMAN_HOST, KUBE_CONFIG, TMPDIR), (ii) filesystem state (`/etc/pve/*` files for Proxmox, `/var/run/secrets/kubernetes.io/serviceaccount/token` for K8s in-cluster, K8s tmpfile cache state at `/tmp/netdata-cgroups-k8s-*`, `/proc/<pid>/comm` for K8s pause detection), (iii) command availability (docker, kubectl, podman, jq, curl), (iv) expected stdout, (v) expected exit code, (vi) expected stderr log payload (modulo timestamps).
- Both the shell script and the Go binary run against every fixture. Outputs must match byte-for-byte (stdout, exit code, stderr log payload) with the following EXPLICIT exclusions:
  - `SYSLOG_IDENTIFIER` field: script emits `cgroup-name.sh`, binary emits `cgroup-name` (Option A decision above).
  - First token of `ND_REQUEST` field: script emits `'cgroup-name.sh'`, binary emits `'cgroup-name'`. All subsequent tokens MUST match byte-for-byte including the trailing space and any embedded brokenness from un-escaped single quotes.
  - Timestamps in log payload.
  - `INVOCATION_ID` (different per invocation; passed through from env).
  All other bytes of the log payload (PRIORITY, THREAD_TAG=cgroup-name, ND_LOG_SOURCE=collector, MESSAGE, newline handling, EOFLOG empty-line requirement) MUST match byte-for-byte.
- A **replay harness — DUAL-RUN MODE (round-7 fix 2026-05-26)**. Earlier drafts described an argv/env/stdout-only capture and a separate "live-system replay" that ran the Go binary later against captured inputs. That design cannot prove byte-for-byte parity because the observed system state (command outputs, /sys, /proc, /etc/pve, tmpfile cache, Docker/Podman/K8s API responses, inherited PATH/HOME/proxy/XDG_RUNTIME_DIR env) drifts between capture time and replay time. The corrected design REQUIRES the wrapper to dual-run BOTH the shell script and the Go binary in the same invocation, on the same live system, capturing the SAME instantaneous system state for both, then compare outputs immediately.

  **Wrapper installation:** install at `${libexecdir}/netdata/plugins.d/cgroup-name.sh` (the existing path that `cgroups.plugin` invokes). The real shell script is moved to `cgroup-name.sh.real`. The Go binary candidate is staged at `cgroup-name` in the same directory. The wrapper, on each invocation:

  1. Records `ts_ns_before`, `pid` (its own), `argv` (passed through), `env` (the FULL parent env — every variable inherited, not just the script-read subset; this is the only way to capture inputs that matter to subprocesses the script spawns).
  2. Forks/execs `cgroup-name.sh.real` with the captured argv + env, capturing its stdout/stderr/exit_code and `duration_shell_ms`.
  3. IMMEDIATELY forks/execs the Go binary at `cgroup-name` with the SAME captured argv + SAME captured env (no env mutation between the two runs), capturing its stdout/stderr/exit_code and `duration_go_ms`.
  4. Compares the two outputs IN THE WRAPPER (byte-for-byte, modulo the explicit exclusions: SYSLOG_IDENTIFIER, first ND_REQUEST token, timestamps, INVOCATION_ID). Computes a `diff_status` field: `match` / `mismatch` / `mismatch_in_excluded_fields` (only the allowed-different fields differ — still a match).
  5. Appends a single JSON record to `${TMPDIR:-/tmp}/cgroup-name-capture.jsonl`:
     ```json
     {
       "ts_ns_before": 1716700000000000000,
       "pid": 12345,
       "argv": ["cgroup-name.sh", "/docker/abc...", "docker_abc..."],
       "env": { "PATH": "...", "HOME": "...", "NETDATA_HOST_PREFIX": "", "DOCKER_HOST": "...", "KUBE_CONFIG": "...", "TMPDIR": "...", "XDG_RUNTIME_DIR": "...", "HTTP_PROXY": "...", "HTTPS_PROXY": "...", "NO_PROXY": "...", "...": "FULL parent env" },
       "shell": { "stdout": "...", "stderr": "...", "exit_code": 0, "duration_ms": 42 },
       "go":    { "stdout": "...", "stderr": "...", "exit_code": 0, "duration_ms": 4 },
       "diff_status": "match",
       "diff_detail": null
     }
     ```
  6. Emits the SHELL SCRIPT'S stdout to the wrapper's own stdout (production behaviour unchanged — `cgroups.plugin` continues to receive the script's verified output), passes the script's exit code through.

  **Why dual-run.** The instantaneous system state — exact Docker container ID-to-name map at this microsecond, exact set of pods kubectl reports, exact contents of `/etc/pve/qemu-server/123.conf`, exact contents of the tmpfile cache after the previous invocation's cache update, exact contents of `os.Environ()` including ephemeral vars like `XDG_RUNTIME_DIR` for the netdata user's runtime — is reproducible ONLY by running both implementations in the same wallclock window before any of that state changes. Recording inputs and replaying later WOULD require mocking every external dependency (every `docker inspect` response, every `kubectl get pods -o json` response, every `/sys/fs/cgroup/.../cgroup.procs` content, every `/proc/<pid>/comm`, every Docker socket HTTP response, every K8s API server response, every kubelet API response, every GCP metadata API response, every tmpfile snapshot). That mocking is intractable for live-system parity proof; the dual-run wrapper sidesteps it by letting the script and the Go binary observe the same live state.

  **Cache-coherence note.** Both implementations read AND write the same tmpfile cache (`${TMPDIR:-/tmp}/netdata-cgroups-*`). Running the script FIRST and the binary IMMEDIATELY AFTER means: the script may write the cache as a side effect, and the binary will then see the just-written cache state. This is FINE for parity proof: the binary should produce the same output the script just produced, and either (a) it hits the cache the script just populated and returns the cached value, or (b) it re-derives the same value from the same primary source (kubectl/API). Either path is correct as long as the resolved NAME matches. Document this in `diff_detail` when `diff_status=mismatch_in_excluded_fields` (cache-hit vs cache-miss path is distinguishable in stderr log payload but both are correct).

  **Wrapper source committed at** `src/collectors/cgroups.plugin/cgroup-name/capture-wrappers/cgroup-name.sh.logger`. The wrapper is a bash script (NOT Go — it must work even when the binary it's testing is broken).

  **Lifecycle:** install the wrapper before merge. Run for several hours on a representative validation host. Once `${TMPDIR}/cgroup-name-capture.jsonl` shows `diff_status=match` for every record covering every detection branch present on the host, the binary is cleared for the SOW close commit. The wrapper is uninstalled by the close commit (which replaces `cgroup-name.sh` with `cgroup-name`). The captured `*.jsonl` corpus is archived under SOW artifacts (or `.local/audits/cgroup-name/` per the project's local-only convention; not committed if it contains production data, sanitised + committed otherwise).

  **Performance impact and deployment scope (round-8 addition 2026-05-26 per reviewer feedback).** The wrapper DOUBLES cgroup-name resolution latency during the validation window (both implementations run sequentially per invocation, plus the wrapper's own diff/JSON-encode/file-append overhead). For a Netdata agent monitoring 100 cgroups this adds roughly 100× (sum of script + Go binary durations) per discovery cycle. This is acceptable on a representative validation host but is NOT acceptable for production hosts. Constraints:
  - The dual-run wrapper MUST NOT ship via any packaging artefact (rpm, deb, snap, container image, makeself). It is a developer-only validation tool, installed manually by the SOW implementer on the validation host(s) only.
  - The validation window is BOUNDED: several hours up to a maximum of 24 hours per validation host. The wrapper is explicitly uninstalled by the SOW close commit; if a regression is found mid-validation the wrapper stays in place only until the next validation iteration closes.
  - Validation hosts MUST NOT be customer/production Netdata deployments. a representative validation host is the primary validation host; additional hosts may be added with the user's explicit consent, but always opt-in, never via auto-deploy.
  - The wrapper's existence in `src/collectors/cgroups.plugin/cgroup-name/capture-wrappers/` does not change its packaging exclusion — the path is in the repo for reproducibility, but no CMake `install(...)` rule references it.

  **Coverage gate.** Before the close commit, scan the corpus and confirm at least one record per detection branch enumerated in the fixture matrix above. Missing branches mean the live workload didn't exercise that path — synthesize a fixture-matrix test for it (via mocked external commands), do NOT close the SOW with branches unexercised. Document the branch-by-branch coverage in the Validation section.

  **Mocked-fixture mode (separate from dual-run).** For the controlled-environment fixture matrix (every detection branch above), use `docker`/`kubectl`/`podman` stubs returning fixed outputs, allowing reproducible tests across machines. These tests run the Go binary alone and compare against a recorded shell-script output captured ONE time on a reference workstation with the same stubs in place. The stub set is committed alongside the fixtures under `src/collectors/cgroups.plugin/cgroup-name/fixtures/`.
- A **speed measurement**: on a fixture set with N cgroups (say, K8s with 100 pods), time `cgroups.plugin` discovery before vs after this SOW. Document the speedup.
- multi-reviewer protocol per master plan workflow signs off on BOTH the flow document AND the implementation.

## Analysis

Sources checked: `cgroup-name.sh.in` (741 lines), `cgroup-discovery.c:235` (invocation site), `cgroup-internals.h` (`cgroup_parse_resolved_name_and_labels` consumer of the output), CMakeLists.txt (build + install), `netdata.spec.in` (rpm), `packaging/cmake/pkg-files/deb/*/postinst` (deb), `packaging/makeself/install-or-update.sh` (static).

Current state:

- Single-script, sequential, well-loved, no timeout.

Risks:

- **Regression risk is the only real risk and it is the biggest risk in this entire integration**. cgroup-name.sh is widely deployed; any deviation in its behaviour shows up as wrong container names in countless production dashboards. The flow document + fixture matrix + replay harness are the discipline to manage this risk. They are NOT optional.
- Go runtime startup overhead (~5 ms cold start) is dwarfed by external subprocess overhead. No concern.
- File capabilities: not needed; runs as netdata user.
- Locale handling: the script uses `LC_ALL=C`. The binary MUST also do this (set in env passed to subprocesses).
- PATH handling: the script extends PATH with `/sbin:/usr/sbin:/usr/local/sbin:@sbindir_POST@`. The binary MUST do the same (resolved at install time via configure_file or equivalent).

## Pre-Implementation Gate

Status: filled 2026-05-26 (round-7 fix). SOW depends on SOW-0032 approval for the workflow gate, but the in-content Pre-Implementation Gate below is concrete and binding.

**Problem / root-cause model.** `cgroup-name.sh` resolves a single cgroup name by spawning many short-lived subprocesses (`sed`, `grep`, `jq`, `curl`, `docker`, `kubectl`, `podman`) per invocation, and `cgroups.plugin` invokes it once per cgroup at discovery time. On hosts with hundreds of containers the cumulative `fork+exec` cost dominates discovery latency. The script's algorithm is correct and battle-tested; only the execution model is slow. Root cause = subprocess spawning per invocation × hundreds of cgroups × every discovery cycle. The Go binary replaces the execution model verbatim — same calls, same dependencies, same priority order, same bugs — and gains speed from (a) in-process Go code instead of `sed/grep/jq/curl` subprocesses and (b) bounded parallelism on the one chain (GCP metadata lines 267-269) where independence is provable.

**Evidence reviewed (with file:line citations).**
- `src/collectors/cgroups.plugin/cgroup-name.sh.in` (741 lines) — read end-to-end; all branches, regexes, sed pipelines, BASH_REMATCH captures, env reads, tmpfile writes, and exit codes catalogued in the Implementation Understanding section above.
- `src/collectors/cgroups.plugin/cgroup-discovery.c:235` — single caller via `spawn_popen_run_variadic`.
- `src/collectors/cgroups.plugin/cgroup-internals.h:163` — comment referencing `cgroup-name.sh`; in removal/update list.
- `src/collectors/cgroups.plugin/sys_fs_cgroup.c:406-407` — default-path constant + config-knob description.
- `src/collectors/cgroups.plugin/README.md:104,111` — documentation references.
- `src/collectors/cgroups.plugin/cgroup-discovery.c:826` — frozen-commit GitHub permalink to the shell script.
- `CMakeLists.txt:3628-3642,3671-3676` — `*_POST` substitutions and `configure_file`/`install(PROGRAMS …)` rules.
- `netdata.spec.in:631` — rpm packaging manifest entry.
- `.gitignore:93` — generated-file entry.
- `src/libnetdata/sanitizers/chart_id_and_name.c:78` — sanitiser comment.
- `src/collectors/guides/proxmox/integrations/proxmox_ve_monitoring.md:137` — Proxmox troubleshooting guide.
- `src/collectors/guides/proxmox/metadata.yaml:133` — Proxmox metadata.
- `src/libnetdata/inicfg/MIGRATE_TO_YAML_WIP.md:1312` — historical WIP migration doc describing the legacy config knob.
- `CHANGELOG.md:2441` — historical changelog entry.
- `packaging/cmake/Modules/NetdataGoTools.cmake` — `add_go_target` macro (4 positional args, `${ARGN}` silently dropped).
- `packaging/cmake/pkg-files/deb/*/postinst`, `packaging/makeself/install-or-update.sh` — verified absent of `cgroup-name.sh` references.
- `.agents/sow/pending/SOW-0032-20260526-topology-containers-ipc-integration-master.md:210` — D9 fixes Go binary location.

**Affected contracts and surfaces.**
- Public CLI contract: argv[1]=cgroup-id, argv[2]=cgroup-path (slash-escaped per script line 648). stdout = `NAME` or `NAME LABELS`. Exit codes 0/1/2/3 per the EXIT_* enumeration above. Stderr = systemd-cat-native structured log (PRIORITY, INVOCATION_ID, SYSLOG_IDENTIFIER, THREAD_TAG, ND_LOG_SOURCE, ND_REQUEST, MESSAGE).
- Environment-variable contract: every variable in the Implementation Understanding env enumeration MUST be read with identical defaulting and unset-vs-empty semantics.
- Tmpfile-cache contract: 3 paths under `${TMPDIR:-/tmp}`; non-atomic writes; overwrite-every-miss; persist across invocations; no expiry.
- External-call contract — the subset that may be in-process replacements is explicitly enumerated in the new "In-process replacement scope" sub-section under Acceptance Criteria below. Operations that observe the host environment (docker/podman/kubectl invocation, snap presence, kubelet `ps -C` probe, jq presence via `hash`/`command -v`, the Proxmox `head`/`grep`/`sed` pipeline) MUST use the same external commands with identical argv and identical stdout/stderr/exit-code parsing.
- Build/install contract: CMakeLists.txt build target + install rule; rpm spec; deb (no entry — confirmed absent); makeself (no entry — confirmed absent); `.gitignore` cleanup.
- Caller contract: `cgroups.plugin` keeps the existing `spawn_popen_run_variadic` invocation; only the configured path changes from `…cgroup-name.sh` to `…cgroup-name`.

**Existing patterns to reuse.**
- `add_go_target` macro in `packaging/cmake/Modules/NetdataGoTools.cmake` (after the extension flagged in the Implementation gate).
- Other CMake-built Go binaries in the repo: `go-plugin`, `nd-mcp-target`, `scripts-plugin`, `topology-ip-intel-downloader-target` — read these call sites before extending the macro to keep extension backward-compatible.
- `cgroups.plugin` C install rule layout for placement of the new install line.
- systemd-cat-native structured-log payload format already used by other plugins (search `THREAD_TAG=` for examples).

**Risk and blast radius.** Behavioural regression in cgroup name resolution shows up as wrong container names in every dashboard with container metrics on every Netdata deployment that upgrades. The mitigations are the flow document (algorithmic fidelity), the fixture matrix (controlled-environment coverage of every detection branch), and the replay harness (real-world coverage with dual-run verification — see updated Validation gate). All three are mandatory; none is optional. Performance regression risk is negligible; the new model strictly removes subprocess spawn cost.

**Sensitive data handling.** SOW, FLOW.md, fixtures, capture corpora, validation reports, and code comments are all commit-ready artifacts (per `AGENTS.md`). They MAY contain cgroup path syntax, container ID format examples, and label key examples. They MUST NOT contain: real container IDs from representative validation hosts or production hosts, real label values from production workloads (only synthetic examples), real K8s namespace/pod/cluster names, real Docker registry image refs from private registries, real Proxmox VM/LXC names (`/etc/pve/qemu-server/<id>.conf` content), real K8s service-account tokens (`/var/run/secrets/kubernetes.io/serviceaccount/token`), real `KUBE_CONFIG` content, or any value that identifies a specific environment. Capture-wrapper outputs at `${TMPDIR}/cgroup-name-capture.jsonl` are LOCAL artefacts (live under `.local/` if checked into the workstation; never committed). Replay-corpus snippets included in `FLOW.md` or fixtures MUST be hand-sanitised before commit (placeholders `[REDACTED_CONTAINER_ID]`, `[NAMESPACE]`, `[POD]`, `[VM_NAME]`, etc.). Any error-message text preserved verbatim from the script (`docker_validate_id` / `podman_validate_id` log payloads) is safe — it contains only cgroup path syntax. Code comments in the Go port that quote shell-script lines for cross-reference are safe — the shell script is itself public.

**Implementation plan.**
1. **Flow document.** Line-by-line read of `cgroup-name.sh.in` → structured `src/collectors/cgroups.plugin/cgroup-name/FLOW.md` covering every function, every external call (with argv/stdout-parsing), every conditional, every env read, every regex, every BASH_REMATCH capture, every sed pipeline, every parameter expansion, every exit-code path, every log line, every tmpfile write, every output transform (with the K8s-skip vs everything-else-truncated split documented per the round-6 correction). Reviewer pass: FLOW.md vs script, then multi-reviewer sign-off.
2. **Fixture matrix.** Capture the matrix enumerated in the Validation gate by running the shell script in controlled environments (real + mocked external commands). One fixture per detection branch + edge cases enumerated above.
3. **Replay harness setup.** Install capture wrapper at `cgroup-name.sh.logger` per the updated Validation gate (dual-run mode: shell + Go in the same invocation, BEFORE the binary ships as the sole executable). Capture corpus from a representative validation host for several hours of real workload. Archive corpus as a frozen test asset under SOW artifacts.
4. **Implement Go binary** at `src/collectors/cgroups.plugin/cgroup-name/` (per SOW-0032 D9) verbatim against FLOW.md.
5. **Extend `add_go_target` macro** in `packaging/cmake/Modules/NetdataGoTools.cmake` to accept extra ldflags; verify backward compatibility with every existing call site (grep first).
6. **Run fixture matrix.** Byte-for-byte equivalence vs the script (modulo SYSLOG_IDENTIFIER/ND_REQUEST/timestamp/INVOCATION_ID exclusions). Iterate.
7. **Run replay harness.** Byte-for-byte equivalence on the captured corpus. Iterate.
8. **Build / install / packaging.** Add the binary; remove the shell script; apply every reference removal/update in the enumerated list (including `cgroup-internals.h:163` and the `MIGRATE_TO_YAML_WIP.md` evidence-backed decision).
9. **Speed measurement.** Quantify `cgroups.plugin` discovery latency before/after on a representative workload (e.g., K8s with 100 pods).
10. **multi-reviewer pass** on the implementation.
11. **SOW close commit.** Implementation + artifact updates + SOW status change + SOW move to `done/` in one commit.

**Validation plan.** Per the Validation gate below — flow-document reviewer sign-off; fixture-matrix byte-for-byte parity for every enumerated branch; replay-harness dual-run byte-for-byte parity on the captured corpus; explicit compatibility test pass-list for external-environment-observing calls (TLS/proxy/socket/error/empty-output edge cases for curl, kubectl, docker, podman); speed measurement with documented delta; lessons extracted; followup mapping (currently None planned).

**Artifact impact plan.** All durable artifacts updated in the same SOW:
- Source: `cgroup-name.sh.in` removed; Go module + binary added at `src/collectors/cgroups.plugin/cgroup-name/`; `FLOW.md` committed permanently.
- Build/packaging: CMakeLists.txt entries removed for the shell script and added for the Go binary; `NetdataGoTools.cmake` macro extended; `netdata.spec.in` updated; `.gitignore` updated.
- C source: `sys_fs_cgroup.c:406-407` default path + description string; `cgroup-internals.h:163` comment; `cgroup-discovery.c:826` permalink replacement; `chart_id_and_name.c:78` comment.
- Docs: `README.md`, Proxmox guide + metadata, `MIGRATE_TO_YAML_WIP.md` (evidence-backed decision documented in the removal list — see "Open decisions" below).
- SOW lifecycle: status moves open → in-progress → completed; file moves pending/ → current/ → done/ in lockstep with the implementation commit.
- Specs/skills: no spec or runtime project skill currently encodes the shell-script algorithm or its environment contract; FLOW.md becomes the new authoritative spec for the binary. No `.agents/skills/project-*` updates required (verified — no existing skill mentions `cgroup-name` or this resolver).
- End-user docs/skills: the `cgroup-name.sh` token appears in user-facing docs (Proxmox guide, README). Both are updated to `cgroup-name`. No `docs/netdata-ai/skills/*` mentions the binary (verified).

**Open decisions resolved.**
- Go binary location: `src/collectors/cgroups.plugin/cgroup-name/` (per SOW-0032 D9, no longer open here).
- FLOW.md retention: KEEP permanently at `src/collectors/cgroups.plugin/cgroup-name/FLOW.md` so the algorithm remains auditable after the shell script is removed.
- `MIGRATE_TO_YAML_WIP.md:1312` handling: KEEP unchanged. This is a historical migration WIP document describing the legacy `[plugin:cgroups]` config knob `script to get cgroup names`. The runtime keeps that same user-facing key for backward compatibility while changing the default executable to `cgroup-name`. Add a single brief footnote/marker near line 1312 noting the legacy knob is now satisfied by a binary (not a script) without rewriting the historical wording.
- `CHANGELOG.md:2441` handling: KEEP unchanged. Historical changelog entries describe shipped state at the time of release; the Go binary gets its own new entry when this SOW ships.

**No remaining "TBD" items.** Every gate sub-section above has concrete content. The only items deferred to in-progress are the actual artifact contents (FLOW.md, fixture matrix, replay corpus, Go source) — those are the SOW's deliverables, not gate prerequisites.

## Implications And Decisions

Decisions inherited from SOW-0032 (D1-D7).

Local binding rules:

- **No algorithmic deviation from cgroup-name.sh.** Verbatim port.
- **Two new behaviours** (corrected 2026-05-26 — earlier "parallelism is the ONLY new behaviour" wording contradicted the Purpose section at L13):
  - **PRIMARY: in-process subprocess replacement.** The dozens of `sed`/`grep`/`jq`/`curl` subprocess spawns per invocation are replaced with in-process Go code (regex, string ops, `encoding/json`, `net/http`). This is where the bulk of the speedup comes from.
  - **SECONDARY: bounded parallelism within the GCP metadata fetch chain only.** The 3 `curl --fail -s -m 3 --noproxy "*"` calls at script lines 267-269 are independent and can run as 3 parallel goroutines, each preserving its own `-m 3` timeout, with first-error cancellation matching the script's `&&` short-circuit. No other parallelism is introduced; the main dispatch is a switch (one branch per cgroup), the K8s API vs kubectl paths are mutually exclusive on env, and the `||` chains at lines 598/623 are precondition-gated single-branch dispatches (NOT parallel races, NOT API-with-CLI-fallback-on-failure).
  - Both behaviours preserve script semantics (priority order, dependency gates, error semantics) unchanged.
- **First artifact is the flow document.** No Go code before the flow document is reviewed.
- **Validation gate is byte-for-byte equivalence** against fixtures + replay.

Local decisions resolved in the Pre-Implementation Gate above:

- Go binary location: `src/collectors/cgroups.plugin/cgroup-name/` (per SOW-0032 D9 at `.agents/sow/pending/SOW-0032-20260526-topology-containers-ipc-integration-master.md:210`). Stale "open decision" wording removed round-7 2026-05-26.
- FLOW.md retention: KEEP permanently at `src/collectors/cgroups.plugin/cgroup-name/FLOW.md` so future maintainers can audit the algorithm without re-reading the (now-removed) script.
- `MIGRATE_TO_YAML_WIP.md:1312` and `CHANGELOG.md:2441` handling: evidence-backed KEEP decisions documented in the Pre-Implementation Gate.

## Plan

1. **Flow document.** Line-by-line read of `cgroup-name.sh.in`; produce structured flow doc. Reviewer pass: flow doc vs script.
2. **Fixture matrix.** Capture inputs/outputs from controlled environments covering every detection branch.
3. **Replay harness.** Capture real-world invocations from production-like Netdata for several hours.
4. **Implement Go binary** against the flow document. Primary speedup via in-process subprocess replacement; bounded parallelism only within the GCP metadata fetch chain (script lines 267-269), preserving semantic order, dependency gates, and per-call `-m 3` timeouts.
5. **Run fixture matrix.** Byte-for-byte equivalence. Iterate.
6. **Run replay harness.** Byte-for-byte equivalence. Iterate.
7. **Build / install / packaging.** Add the binary. Remove the shell script and all its references.
8. **Speed measurement.** Quantify the discovery speedup on a representative workload.
9. **Multi-reviewer pass** on the implementation.

## Execution Log

- 2026-05-27: Moved SOW from `pending/` to `current/`, set status to `in-progress`, and sanitized planning-review names from the durable SOW artifact.
- 2026-05-27: Added permanent flow document at `src/collectors/cgroups.plugin/cgroup-name/FLOW.md`.
- 2026-05-27: Added Go module at `src/collectors/cgroups.plugin/cgroup-name/` with the `cgroup-name` binary, focused unit tests, and developer-only dual-run wrapper source.
- 2026-05-27: Extended `add_go_target` with target-local `EXTRA_LDFLAGS` and wired CMake to build/install `cgroup-name` with `-X main.sbindirPost=${sbindir_POST}`.
- 2026-05-27: Removed `src/collectors/cgroups.plugin/cgroup-name.sh.in`; updated default resolver path, rpm manifest, `.gitignore`, cgroups comments, README, Proxmox metadata/generated integration page, and the YAML migration WIP note.

## Validation

- Acceptance criteria evidence:
  - `src/collectors/cgroups.plugin/cgroup-name/FLOW.md` documents startup/env/logging, Docker/Podman parsing, Kubernetes cache/API/kubectl flow, GCP metadata handling, main dispatch order, known bugs, output transforms, and exit codes.
  - `src/collectors/cgroups.plugin/cgroup-name/main.go` preserves the known `ystem.slice...` extraction pattern and the Podman error typo, keeps `docker`/`podman`/`kubectl`/`snap`/`ps` as subprocess-observed environment operations, checks `jq` presence before the in-process JSON paths, and invokes `systemd-cat-native --log-as-netdata`.
  - CMake generated rule verified at `/tmp/topology-containers-sow0038-min-build/CMakeFiles/cgroup-name-target.dir/build.make:76`; final `go build` command includes `-X main.sbindirPost=/usr/local/usr/sbin`.
- Tests and equivalent validation:
  - `go test -count=1 ./...` from `src/collectors/cgroups.plugin/cgroup-name` passed.
  - `go vet ./...` from `src/collectors/cgroups.plugin/cgroup-name` passed.
  - `shellcheck src/collectors/cgroups.plugin/cgroup-name/capture-wrappers/cgroup-name.sh.logger` passed.
  - `cmake -S . -B /tmp/topology-containers-sow0038-min-build -DDEFAULT_FEATURE_STATE=OFF -DENABLE_CGROUP_NAME=ON` passed.
  - `cmake --build /tmp/topology-containers-sow0038-min-build --target cgroup-name-target -j1` passed.
  - Built binary smoke: `/tmp/topology-containers-sow0038-min-build/cgroup-name machine.slice_demo.service machine.slice_demo.service` returned `demo`.
  - Shell-vs-Go parity checks were run before deleting the script for: `kubepods`, `kubepods-burstable`, systemd-nspawn, libvirt/qemu, legacy libvirt qemu, LXC payload, invalid Docker id fall-through, libvirt/LXC slash-to-underscore behavior, Docker CLI inspect with `netdata.cloud/*` labels including `=` in value, and cached Kubernetes container labels. All compared stdout and exit code matched.
  - `python3 integrations/gen_docs_integrations.py -c guides/proxmox` was run; the generated page was then restored to the pre-existing related-integration link style while keeping the `cgroup-name` helper wording.
  - `rg -n "cgroup-name\\.sh|cgroup-name\\.sh\\.in" . --glob '!/.agents/sow/done/SOW-0038-20260526-cgroup-name-go-binary.md' --glob '!CHANGELOG.md' --glob '!src/collectors/cgroups.plugin/cgroup-name/FLOW.md' --glob '!src/collectors/cgroups.plugin/cgroup-name/capture-wrappers/cgroup-name.sh.logger' --glob '!/.git/**'` returned no matches. The legacy INI key `script to get cgroup names` remains intentionally for compatibility.
  - `git diff --check` passed.
- Real-use evidence:
  - The new CMake-built binary was executed locally through the same argv contract used by `cgroups.plugin`.
  - Live multi-hour dual-run on a representative host was not run in this implementation pass; the source wrapper is committed for that purpose, but current instructions only allow external host/prod-style validation with explicit user consent.
- Reviewer findings:
  - No external implementation reviewers were run in this pass because current repository instructions allow external reviewers only when explicitly requested by the user in the turn. Local review focused on script parity, build integration, packaging references, and same-failure search.
- Same-failure search:
  - Repo-wide cgroup-name shell reference search found no remaining unexpected `cgroup-name.sh` or `cgroup-name.sh.in` references outside intentional historical/SOW/FLOW/wrapper contexts. The legacy `script to get cgroup names` INI key remains intentionally in runtime config, README, and migration notes.
- Artifact maintenance gate:
  - `AGENTS.md`: no update needed; workflow rules did not change.
  - Runtime project skills: no update needed; no project skill contained the cgroup-name resolver contract.
  - Specs: no existing SOW spec contained this resolver; `FLOW.md` is the new local source contract for the binary.
  - End-user/operator docs: updated cgroups README and Proxmox metadata/generated integration text from `cgroup-name.sh` to `cgroup-name`.
  - End-user/operator skills: no update needed; no public skill references this resolver.
  - SOW lifecycle: this SOW is marked `completed` and moved to `done/` with the implementation commit.
- SOW status/directory consistency:
  - Status is `completed`; final file location is `.agents/sow/done/`.
- Lessons:
  - The Proxmox docs generator rewrote related integration links while regenerating the page. The link style was restored manually to keep this SOW scoped to the resolver rename.
- Follow-up mapping:
  - No follow-up SOW required. Optional multi-hour dual-run deployment remains an operator validation activity before rollout, using the committed wrapper source and local-only capture output.

## Outcome

Completed. The shell resolver is replaced by a Go `cgroup-name` binary, the old generated shell script is removed from build/install/packaging, and user-facing references now name the helper without the `.sh` suffix.

## Lessons Extracted

- Keep the resolver algorithm in `FLOW.md` after removing the shell source; otherwise future maintainers would have to reconstruct shell-era compatibility from commit history.
- Regenerating Proxmox guide output can rewrite unrelated link formatting; review generated diffs before committing.

## Followup

None planned. Optional live dual-run validation can be performed with `src/collectors/cgroups.plugin/cgroup-name/capture-wrappers/cgroup-name.sh.logger` before broad rollout; captures must remain local because they include full environment and workload data.

## Regression Log

None yet.
