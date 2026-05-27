# SOW-0043 - SNMP Traps: go.d.plugin Privileged-Port Capability Packaging

## Status

Status: completed

Sub-state: implementation, validation, review, and artifact maintenance complete.

## Requirements

### Purpose

Ensure installed Netdata Agents can run the SNMP trap listener on the standard UDP/162 trap port without requiring operators to choose a non-standard port on network devices.

### User Request

The user first requested local configuration on UDP/9162, then corrected the requirement: some devices cannot configure the trap destination port, so UDP/162 is required. The user explicitly requested adding the needed capability to installer/CMake/packaging so all users receive it.

### Assistant Understanding

Facts:

- UDP/162 is a privileged port on Linux because it is below 1024.
- Linux `CAP_NET_BIND_SERVICE` permits binding privileged Internet-domain ports, per `capabilities(7)` / `ip(7)` from man7.org.
- The SNMP trap collector already models bind failure as a job-creation failure surfaced to DynCfg, so missing capability causes a clear but avoidable job-creation error.
- Current packaging already grants file capabilities to `go.d.plugin` in four places:
  - `netdata-installer.sh`
  - `netdata.spec.in`
  - `packaging/makeself/install-or-update.sh`
  - `packaging/cmake/pkg-files/deb/plugin-go/postinst`
- `packaging/cmake/Modules/Packaging.cmake` wires `plugin-go/postinst` into the CPack Debian package.
- The installed `netdata.service` also constrains children with `CapabilityBoundingSet`; a file capability cannot be used by `go.d.plugin` unless that capability is allowed by the service bounding set.
- CMake generates and installs the packaged systemd service from `system/systemd/netdata.service.in`; the native service installer copies the same generated service from the installed service source directory.

Inferences:

- Adding `cap_net_bind_service` to the existing `go.d.plugin` file-capability sets and allowing `CAP_NET_BIND_SERVICE` in the installed `netdata.service` bounding set is the smallest packaging change that preserves the existing least-privilege pattern and enables the default SNMP trap port.

Unknowns:

- No unknowns block implementation. RPM/native distro-specific postinstall paths were searched in this branch; `go.d.plugin` capability paths were found in `netdata-installer.sh`, `netdata.spec.in`, the makeself installer, and the CMake Debian package path. The systemd service source path was identified and included.

### Acceptance Criteria

- Native installer path in `netdata-installer.sh` grants `cap_net_bind_service` while preserving existing capabilities.
- RPM spec path in `netdata.spec.in` grants `cap_net_bind_service` while preserving existing capabilities.
- `go.d.plugin` install/update path in makeself grants `cap_net_bind_service` while preserving existing capabilities.
- CMake Debian `plugin-go` postinst grants `cap_net_bind_service` while preserving existing capabilities.
- The systemd service installed by packages and the native installer allows `CAP_NET_BIND_SERVICE` in `CapabilityBoundingSet`.
- SNMP trap spec and stock config comments reflect that Netdata packaged installs grant and allow this capability, while custom/manual installs still need equivalent capability/root if using UDP/162.
- Local installed Agent has `go.d.plugin` capability set including `cap_net_bind_service`, config bound to UDP/162, and a smoke trap can be received.

## Analysis

Sources checked:

- `packaging/makeself/install-or-update.sh`
- `packaging/cmake/pkg-files/deb/plugin-go/postinst`
- `packaging/cmake/Modules/Packaging.cmake`
- `netdata-installer.sh`
- `netdata.spec.in`
- `.agents/sow/specs/snmp-traps/netdata.md`
- `src/go/plugin/go.d/config/go.d/snmp_traps.conf`
- `src/go/plugin/go.d/collector/snmp_traps/config_schema.json`
- man7.org `capabilities(7)` and `ip(7)` for Linux privileged port capability semantics.

Current state:

- `netdata-installer.sh` currently sets `cap_dac_read_search+epi cap_net_admin+epi cap_net_raw=eip` on `go.d.plugin`.
- `netdata.spec.in` currently sets `%caps(cap_dac_read_search,cap_net_admin,cap_net_raw=eip)` on `go.d.plugin`.
- makeself currently sets `cap_dac_read_search+epi cap_net_admin+epi cap_net_raw=eip` on `go.d.plugin`.
- Debian `plugin-go` postinst currently sets `cap_dac_read_search+epi cap_net_admin=eip cap_net_raw=eip` on `go.d.plugin`.
- `system/systemd/netdata.service.in` contains a constrained `CapabilityBoundingSet` without `CAP_NET_BIND_SERVICE`; adding only the file capability causes `go.d.plugin` exec to fail with `Operation not permitted` under that service.
- The installed workstation binary was manually updated to include `cap_net_bind_service`, but repository packaging and the installed service do not yet preserve that for users.

Risks:

- Security: adding a capability broadens `go.d.plugin` privilege enough to bind any privileged local port, not only UDP/162. This is narrower than running as root or relying on setuid fallback and matches the Linux capability designed for privileged-port binding.
- Operational: if a filesystem does not support file capabilities or `setcap` fails, the existing fallback remains `chmod 4750`, preserving current behavior.
- Packaging coverage: only paths that already set `go.d.plugin` capabilities are changed (`netdata-installer.sh`, `netdata.spec.in`, makeself install/update, CMake Debian postinst), and the common systemd service source installed by CMake packages and the native installer is changed. If a downstream distro has separate packaging or service files outside this tree, it must carry both the file capability and the service bounding-set change.

## Pre-Implementation Gate

Status: ready

Problem / root-cause model:

- Network devices often default to UDP/162 for SNMP traps and some do not allow a non-standard destination port. The collector default uses UDP/162, but `go.d.plugin` runs as an unprivileged user in normal installs. Without `CAP_NET_BIND_SERVICE`, binding UDP/162 fails at job creation with `EACCES`. If `go.d.plugin` has that file capability but the systemd service bounding set excludes it, Linux rejects plugin exec with `Operation not permitted`.

Evidence reviewed:

- Installed runtime check showed `go.d.plugin` running unprivileged and lacking `cap_net_bind_service` before the local manual fix.
- Packaging grep found four repository paths that set `go.d.plugin` capabilities: `netdata-installer.sh`, `netdata.spec.in`, makeself install/update, and CMake Debian postinst.
- Runtime validation after the file-capability-only change showed `go.d.plugin` failed to exec with `Operation not permitted` because `netdata.service` excluded `CAP_NET_BIND_SERVICE` from `CapabilityBoundingSet`.
- CMake service install evidence:
  - `CMakeLists.txt` generates `system/systemd/netdata.service` from `system/systemd/netdata.service.in`.
  - `CMakeLists.txt` installs the generated service into `lib/systemd/system` for packages.
  - `system/install-service.sh.in` installs `${SVC_SOURCE}/systemd/netdata.service` for native service installs.
  - `system/install-service.sh.in` selects `netdata.service.v235` for older systemd versions; that template has no `CapabilityBoundingSet`, so it does not strip file capabilities.
- SNMP trap spec §6 already says UDP/162 requires `CAP_NET_BIND_SERVICE` or equivalent privilege.

Affected contracts and surfaces:

- Installer/update behavior for makeself users.
- Native installer behavior for `netdata-installer.sh` users.
- RPM package file-capability behavior for `netdata.spec.in` users.
- Debian package post-install behavior for the `netdata-plugin-go` package.
- Installed systemd service behavior for CMake packages and native installer users.
- SNMP trap operator expectations for the default UDP/162 job.
- Security posture of `go.d.plugin` file capabilities.

Existing patterns to reuse:

- Existing `setcap` commands for `go.d.plugin`; append one capability to those sets.
- Existing systemd `CapabilityBoundingSet` pattern in `system/systemd/netdata.service.in`; add one capability line near related network capabilities.
- Existing fallback behavior to `chmod 4750` if `setcap` fails.
- Existing stock config comments and spec wording for privileged-port behavior.

Risk and blast radius:

- The change affects all `go.d.plugin` collectors by granting the binary privileged-port bind ability and allowing that capability through the Netdata service bounding set. It does not grant packet sniffing beyond existing `cap_net_raw`/`cap_net_admin`, and it is narrower than setuid-root fallback.

Sensitive data handling plan:

- No secrets, SNMP communities, customer names, non-private IPs, private endpoints, or packet payloads are needed. Durable evidence uses generic port/capability names only.

Implementation plan:

1. Add `cap_net_bind_service` to all existing `go.d.plugin` `setcap` command strings.
2. Add `CAP_NET_BIND_SERVICE` to the systemd service `CapabilityBoundingSet` source installed by packages and the native installer.
3. Update SNMP trap spec and stock config comments so operator-facing artifacts match packaged behavior.
4. Validate syntax/search coverage, then verify the local installed Agent can bind UDP/162 and receive a local synthetic v2c trap.

Validation plan:

- Same-failure grep for all `go.d.plugin` `setcap` paths, including `netdata-installer.sh`.
- Service install path grep for `netdata.service.in` through CMake package install and `system/install-service.sh.in`.
- Shell syntax check for touched shell scripts.
- Local `systemctl show netdata -p CapabilityBoundingSet` verification after applying the installed drop-in equivalent.
- `git diff --check`.
- Local installed `getcap` and `ss` verification for UDP/162.
- Send local synthetic trap and query generated journal entry.

Artifact impact plan:

- AGENTS.md: no workflow change.
- Runtime project skills: no reusable workflow change.
- Specs: update SNMP trap spec privileged-port packaging behavior.
- End-user/operator docs: stock config comment update now; full docs remain SOW-0039.
- End-user/operator skills: unaffected.
- SOW lifecycle: SOW-0043 tracks this packaging change independently of SOW-0039.

Open-source reference evidence:

- No mirrored repository evidence needed; this is local packaging behavior plus Linux capability semantics from official Linux man pages.

Open decisions:

- User decision: UDP/162 is required; packaging must grant the capability by default.

## Implications And Decisions

1. Privileged-port strategy
   - Selected: add `cap_net_bind_service` to `go.d.plugin` package/installer capability sets and allow `CAP_NET_BIND_SERVICE` in the installed systemd service bounding set.
   - Reason: supports devices that cannot configure a custom trap port, avoids running the whole plugin as root, avoids systemd `execve` denial, and preserves existing job-creation failure semantics when binding still fails for other reasons.

## Plan

1. Patch native installer, RPM spec, makeself, and CMake Debian postinst capability strings.
2. Patch the systemd service source installed by packages and the native installer.
3. Patch trap spec and stock config comment.
4. Validate packaging paths, shell syntax, service bounding set, and local UDP/162 smoke test.

## Execution Log

### 2026-05-27

- SOW created for packaging-level `CAP_NET_BIND_SERVICE` support.
- Runtime validation showed file capability alone is insufficient when `netdata.service` excludes `CAP_NET_BIND_SERVICE`; service bounding-set support added to the SOW scope.
- External review found `netdata-installer.sh` also sets `go.d.plugin` file capabilities; this installer path was added to scope and patched.
- External review found `netdata.spec.in` also sets RPM file capabilities for `go.d.plugin`; this package path was added to scope and patched.
- Historical packaging check: commit `42ffc28022` removed `cap_net_bind_service` from `otel-plugin` because that binary had no confirmed privileged listener requirement. This SOW adds the capability to `go.d.plugin` for the concrete SNMP trap UDP/162 listener requirement, and pairs it with the service bounding-set change.

## Validation

Acceptance criteria evidence:

- `netdata-installer.sh` now grants `cap_net_bind_service` in the `go.d.plugin` `setcap` string.
- `netdata.spec.in` now grants `cap_net_bind_service` in the RPM `%caps(...)` declaration for `go.d.plugin`.
- `packaging/makeself/install-or-update.sh` now grants `cap_net_bind_service` in the `go.d.plugin` `setcap` string.
- `packaging/cmake/pkg-files/deb/plugin-go/postinst` now grants `cap_net_bind_service` in the `go.d.plugin` `setcap` string.
- `system/systemd/netdata.service.in` now allows `CAP_NET_BIND_SERVICE` in `CapabilityBoundingSet`.
- `src/go/plugin/go.d/config/go.d/snmp_traps.conf`, `src/go/plugin/go.d/collector/snmp_traps/config_schema.json`, and `.agents/sow/specs/snmp-traps/netdata.md` document that packaged installs grant and allow this capability.

Tests or equivalent validation:

- `bash -n netdata-installer.sh`: passed.
- `bash -n packaging/makeself/install-or-update.sh`: passed.
- `sh -n packaging/cmake/pkg-files/deb/plugin-go/postinst`: passed.
- `jq empty src/go/plugin/go.d/collector/snmp_traps/config_schema.json`: passed.
- `systemd-analyze verify` on a generated temporary service from `system/systemd/netdata.service.in`: passed.
- `rpmspec --parse` on a temporary configured spec with `@PACKAGE_VERSION@` substituted: passed, with only pre-existing unversioned `Obsoletes` warnings.
- `git diff --check`: passed.
- Temporary `sudo setcap "cap_dac_read_search,cap_net_admin,cap_net_raw,cap_net_bind_service=eip"` validation confirmed the RPM/libcap text form grants all four capabilities.

Real-use evidence:

- Installed local `netdata.service` bounding set includes `cap_net_bind_service`.
- Installed local `go.d.plugin` file capabilities include `cap_net_bind_service`.
- Installed local `go.d.plugin` is running and owns UDP/162.
- Synthetic local v2c `SNMPv2-MIB::coldStart` trap was received and written to the trap journal.

Reviewer findings:

- Finding: `netdata-installer.sh` also sets `go.d.plugin` file capabilities. Resolution: patched and added to SOW acceptance criteria and validation.
- Finding: `netdata.spec.in` also sets `go.d.plugin` RPM file capabilities. Resolution: patched and added to SOW acceptance criteria and validation.
- Finding: `netdata.service.v235.in` lacks `CapabilityBoundingSet`. Resolution: rejected as a blocker because the template has no bounding-set restriction; without that directive, systemd does not strip the file capability. Adding a partial capability set there would create a larger compatibility risk for old systems.
- Finding: pre-existing `cap_net_admin+epi` vs `cap_net_admin=eip` style differs between shell installer and Debian postinst. Resolution: not changed in this SOW because it predates the work and both commands remain authoritative full capability sets.
- Finding: Docker packaging uses setuid-root for `go.d.plugin`, not the file-capability pattern. Resolution: not changed here because it is pre-existing, functionally already allows privileged bind inside containers when the container runtime permits it, and this SOW targets native/package installer capability gaps.
- Finding: old `otel-plugin` `cap_net_bind_service` removal could confuse reviewers. Resolution: recorded in the execution log; the old change removed an unnecessary capability from a different binary, while this SOW adds the capability to `go.d.plugin` for a required UDP/162 listener.
- Second review round: no blockers found after patching `netdata-installer.sh` and `netdata.spec.in`. Remaining comments were non-blocking pre-existing consistency notes or out-of-scope documentation suggestions.

Same-failure scan:

- `rg` across `netdata-installer.sh`, `netdata.spec.in`, `packaging/`, `system/`, and touched trap docs now shows all `go.d.plugin` capability paths include `cap_net_bind_service`.
- `CMakeLists.txt` and `system/install-service.sh.in` evidence confirms `system/systemd/netdata.service.in` is the packaged/native installer source for the systemd service; the old-systemd v235 variant does not apply a bounding-set filter.

Sensitive data gate:

- No secrets, SNMP communities, packet payloads, customer identifiers, non-private customer IPs, or private endpoints were written to durable repo artifacts.
- Runtime smoke evidence recorded only generic capability names, port number, plugin name, and trap type.

Artifact maintenance gate:

- AGENTS.md: no workflow or guardrail change.
- Runtime project skills: no reusable workflow change.
- Specs: `.agents/sow/specs/snmp-traps/netdata.md` updated for packaged privileged-port behavior.
- End-user/operator docs: stock config and DynCfg schema text updated; broader public docs remain tracked by SOW-0039.
- End-user/operator skills: unaffected; no public skill behavior changed.
- SOW lifecycle: SOW-0043 tracks this packaging fix; reviewer-found missed paths were folded into the same SOW.

## Outcome

Packaging, service, specs, schema, and stock config now support UDP/162 for packaged SNMP trap installs by granting `CAP_NET_BIND_SERVICE` to `go.d.plugin` and allowing it through the installed systemd service bounding set.

## Lessons Extracted

- Privileged file capabilities under systemd need two checks: the file capability itself and the service `CapabilityBoundingSet`.
- Installer/package path searches must include root-level installer/spec files, not only `packaging/`.

## Followup

No follow-up work was created. Reviewer suggestions about Docker privilege documentation and ping collector capability documentation were rejected for this SOW because they describe pre-existing adjacent behavior, not a gap introduced by the SNMP trap UDP/162 packaging change.

## Regression Log

None yet.
