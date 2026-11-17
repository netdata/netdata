# Browser runtime host setup

DEM journeys and Lighthouse audits need an explicitly installed browser runtime. RUM does not. The runtime is one
signed, portable bundle for Linux amd64 or arm64 that contains its own Node, Chromium, Playwright, Lighthouse and their
JavaScript adapters. Installing it does not require, use or discover a system browser, Node or npm.

:::info

The browser runtime is experimental, and its first signed public release still needs deployment validation.
Release builds default to the exact matching Agent version, channel and architecture at
`https://artifacts.netdata.cloud/dem-runtime/<channel>/<version>/<arch>/release.json`.
An administrator can override `runtime.release_url` in `dem.conf`; an explicit empty value disables that default.
If an older copied `dem.conf` retains `release_url: ''`, remove that line to adopt the release default, or keep it and
supply matching offline inputs during updates. A local `--release` descriptor with explicit `--allow-download` can
still download its archive; an empty discovery URL does not prohibit every network operation.
Unstamped development builds can use an already installed compatible runtime. Installing or updating a runtime needs
a helper with the target release identity; a URL override does not supply that identity. Offline setup needs the signed
descriptor, its archive and an administrator-provisioned public keyring.

:::

Enabling a browser monitor never downloads or installs the runtime. Without one, browser jobs wait for it and report
no website failures. **Activation requires restarting the Agent, including after first setup.** A running plugin keeps
the release it acquired at startup. Native hooks and paired source/static installers restart a previously running
Agent under their service policy. Explicit CLI setup/repair and Docker/preload deployments leave restart to their owner.

On Ubuntu 24.04, default AppArmor restrictions prevent an ordinary unconfined process from creating Chromium's
sandbox. `netdata-dem-runtime.sh` prepares the host allowance and then runs the existing runtime CLI as Netdata's
nonroot service account. DEM receives no host-policy privileges.

| Installation | Explicit setup | Policy owner |
|---|---|---|
| Native DEB/RPM | Install the optional `netdata-dem-runtime` package | Package manager and package hooks |
| Static or source | Installed `netdata-dem-runtime.sh`, or kickstart with `--install-dem-runtime` | Administrator-run helper |
| Local rootful Docker | Run kickstart on the daemon host for the named running container, then restart it after successful setup. | Host deployment administrator |
| Protected preload | Prepare the policy before validation/sealing at the preload's final path | Preload/deployment administrator |

Ordinary Agent installation and enabling a monitor do not download a browser or install an unused namespace allowance.
Agent-linked `install`, `repair` and `preload` through this helper are for static/source installations. Native-packaged
Agent builds use the optional runtime package; changing `runtime.mode` does not enable mixed-mode automatic host setup.
AppArmor being absent or disabled does not cause the helper to install it. On an enabled host, missing parser tools or
unreadable policy state are reported as prerequisites; the helper does not download host packages.

## Native DEB/RPM packages

Install the optional `netdata-dem-runtime` package with the system package manager. DEM never invokes the package
manager itself, and the package owns its installation lifecycle: the low-level CLI refuses to install, update, repair
or remove a native runtime.

- **Hooks.** The package hooks need a live systemd host. They stop the Agent, refuse to change the payload while a
  runtime consumer (such as a standalone CLI qualification) still runs, validate the new payload and restart the Agent
  if it was running. Installing into an image or chroot is not supported yet.
- **AppArmor.** On hosts with AppArmor, the package ships a profile for its root-owned Chromium executable and loads it
  before final payload validation. The profile permits the user namespaces Chromium needs for its sandbox; it does not
  add browser confinement.
- **Failures.** Administrator policy stays authoritative. A policy or payload validation failure keeps browser admission
  closed while the hooks restore eligible non-browser monitoring.

Payload layout, hooks and offline provisioning: [native package guide](/packaging/dem-runtime/native/README.md).

## Static and source installations

Use the installed helper and the matching Agent executable. For the standard static prefix:

```sh
sudo /opt/netdata/usr/libexec/netdata/netdata-dem-runtime.sh install \
  --agent /opt/netdata/usr/sbin/netdata --allow-download
```

For a source installation under `/`, the paths are `/usr/libexec/netdata/netdata-dem-runtime.sh` and
`/usr/sbin/netdata`. Substitute the actual prefix for a custom installation. The helper queries the Agent for its
configured service account and directories; it does not parse configuration as shell code. The Agent and plugin
executables and their ancestors must be root-owned and not writable by other users.
Root-owned aliases, including the static install's standard paths, are supported only when every directory and target
in their resolution has the same protection. Privileged configuration queries run from `/` with a cleared environment;
runtime operations still run as the service account.

Fresh root source installations create safe executable-path directories, and static updates repair the shipped
executable directories. Historical source installations may retain group-writable ancestors. The installer cannot
establish ownership of pre-existing shared or administrator-selected parents, so it leaves them untouched and the
helper refuses unsafe paths. An administrator must inspect and reconcile those directories explicitly, or use the
policy-only route below; the helper never relaxes its root-execution checks.

Ownership and write-permission failures identify the rejected path, its numeric owner UID and octal mode. Inspect
that path and its parents, including any symbolic links, before changing them. For example, if the helper reports
`/opt/netdata/usr/sbin` with owner UID `0` and mode `0775`:

```sh
namei -l /opt/netdata/usr/sbin/netdata
```

If `/opt/netdata/usr/sbin` is confirmed to be a root-managed Netdata directory, remove its group/other write bits
without changing its contents or other permissions, then retry the original runtime setup command:

```sh
sudo chmod go-w /opt/netdata/usr/sbin
```

Apply corrections only to the reported path after checking its intended ownership. Do not recursively change
permissions or alter shared directories to satisfy the helper. A nonroot-owned executable or parent requires an
administrator to reconcile ownership or select a root-owned installation; nonroot source installations can use
the policy-only route below. Symbolic links must be root-owned, but their permission bits are not checked.

The same setup is included in `kickstart.sh --install-dem-runtime`. Initial installation permits companion downloads
during later Agent updates unless `--manual-updates` is specified. **Restart the Agent explicitly after first setup.**
The setup helper does not restart it or change the Agent's update schedule. Later source/static Agent updates coordinate
the installed pair as described below.

Offline setup uses the same helper with an administrator-provisioned `runtime.offline_keyring`:

```sh
sudo /opt/netdata/usr/libexec/netdata/netdata-dem-runtime.sh install \
  --agent /opt/netdata/usr/sbin/netdata \
  --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
```

Do not add `--allow-download` to an offline import. Future Agent updates may acquire a matching runtime online unless
`--manual-updates` is specified. The nonroot service account must be able to read the supplied files.

`repair` uses the same policy/setup path and existing signed repair contract. `--config-dir` and `--cache` select
explicit configuration and managed storage overrides; they must match the actual service deployment. The helper
does not infer a service's custom command-line or environment overrides.

For a nonroot source installation whose executables are user-owned, an administrator can use the policy-only commands
below for the canonical runtime root, then the service account can use the ordinary runtime CLI. This avoids executing
user-owned Agent code as root.

## Docker host preparation

This command targets a local, rootful Linux Docker daemon and the [documented Netdata container configuration](/packaging/docker/README.md#recommended-way).
On AppArmor hosts use the documented `apparmor=unconfined` container setting. Chromium's sandbox and the
host's namespace restrictions stay enabled. Persist `/var/lib/netdata` on local storage, so the installation survives
container replacement. No Agent is needed on the host. For the first install, the container needs access to a
published signed release matching its image. On the daemon host, replace `netdata` with the running container's name:

```sh
sudo sh ./kickstart.sh --install-dem-runtime --docker-container netdata
```

Use this command with a Netdata image you trust to supply code that runs as root on the host. Kickstart
reads the running deployment’s exact local image ID and creates a temporary container from that image.
It copies the host setup scripts out and removes the temporary container and its anonymous volumes.
The temporary container never starts. Changes inside the running deployment cannot replace these scripts.

The host helper prepares AppArmor on hosts that need it. The running deployment downloads, verifies and unpacks the
signed runtime as the container’s nonroot service user. Preparation verifies artifact integrity and bound paths without
executing a browser while the Agent continues monitoring. Preparation has no overall time limit. An error or administrator
cancellation returns a failure. After a successful first installation, activate the runtime when ready:

```sh
docker restart netdata
```

Keep the printed registration token for retirement. On restricted hosts, run kickstart once per deployment,
even if another container already registered the same launcher path. Each registration protects the shared policy.
Image updates require the paired deployment procedure below; the plugin does not download a new runtime in the
background. Running kickstart again reuses a valid selection offline and preserves its update policy. An
already-selected result by itself requires no additional restart.

The host command and sandboxed browser jobs were tested on Ubuntu 24.04 amd64 and Debian 13 amd64. Automatic
AppArmor setup on hosts that restrict unprivileged user namespaces is qualified for Ubuntu 24.04. On hosts
without that restriction, the command skips the policy installation. The standard runtime root inside the
container is `/var/lib/netdata/dem/runtime`. Rootless, nested and remote Docker, custom confinement and custom
runtime roots need separate administrator setup. For offline inputs or custom roots, use a trusted copy
of the supplied host helper with `policy-prepare --deployment TOKEN --runtime-root /execution/visible/root`,
then use the [runtime CLI](#runtime-cli-reference) as the service user.

## Protected preload updates

A protected preload is a sealed runtime at a fixed final path, for an immutable image or a read-only deployment. It
lives outside the managed store and its retention.

The high-level `preload` command accepts `--root /absolute/final/path` with the same signed offline or download inputs.
It prepares policy before integrity validation and sealing. The low-level equivalent, run as the service account
after any required host policy is prepared:

```sh
dem.plugin runtime preload --root /opt/netdata-dem-runtime --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
```

Then point the plugin at it in `dem.conf`:

```yaml
runtime:
  mode: preloaded
  preload_path: /opt/netdata-dem-runtime
```

- **Root.** It must be new or empty. The command verifies the complete release without executing a browser, then seals
  its identity and final location into the root. Keep it private and owned by the consuming service account; it may be mounted
  read-only.
- **Lifecycle.** Preloads are outside managed retirement and repair, and their updates are manual. Stop **every**
  consumer before replacing or removing one, prepare a replacement at a new final path, and restart DEM to switch.
  Never move a sealed preload.
- **Native packages** use their coordinated package hooks instead.

## Paired Agent updates

Each DEM process acquires the selected release when it starts and keeps it for its whole lifetime. A process started
without a runtime stays unavailable until restarted, even after an installation succeeds. A runtime belongs to one
Agent version, stable/nightly channel and source commit for publication, installation and updates. Browser fixes ship with an Agent release; keeping an old Agent
does not provide a separate browser update stream. Every run records the actual runtime identity.

Execution checks the runtime contract, architecture, provenance and integrity, independently of the plugin’s release
stamp. A copied development plugin can use an installed runtime if that contract is compatible; it needs no bypass.
Installer and deployment checks still require the exact Agent/runtime pair.

| Installation | Update owner | Activation and recovery |
|---|---|---|
| Native DEB/RPM | Package manager; the Netdata updater requests the Agent, DEM plugin and runtime together when the runtime is installed | Hooks drain, validate the exact pair and restore prior service intent. Retry or downgrade the matching packages together. |
| Source/static | Agent installer/updater, only for an enrolled runtime | Verify target inputs, stop/drain, install Agent files normally, activate and restore prior service intent. Repair/retry a failed Agent installation; runtime selection and ownership records remain protected. |
| Docker | Deployment administrator using the pinned target image | Fetch before stopping; prepare and activate against the persistent volume after drainage. Roll back selection and image together. |
| Protected preload | Deployment administrator | Retain both sealed roots and configuration artifacts; select the matching target root and Agent together. Restore both on rollback. |

No runtime is acquired by an ordinary Agent update unless it was already installed. DEM has no independent download
timer. Initial `install` or `repair` permits companion acquisition during Agent updates unless `--manual-updates` is
given. `runtime update` directs operators to this installation owner. Integrity and version checks are mandatory. Browser smoke tests run in
release CI, and `runtime qualify` remains an optional local diagnostic.

### Source/static controls and repair

The installer verifies the target descriptor and archive before changing live Agent files or stopping the service.
Trust/download failures leave the current installation running. After stopping and draining consumers, it prepares
runtime storage, installs Agent files through the normal source/static path, and activates the exact companion.
An already stopped Agent stays stopped; `--dont-start-it` also prevents starting it. This flag applies to the current
invocation and is not saved as a permanent start policy.

A prefix-specific lock serializes paired installers. For nonroot source installs, its directory and ancestors must
be owned by the installing user or root and must not be group- or world-writable, except root-owned sticky temporary
ancestors. An unsafe path is reported without changing its permissions. Source installs stage only the target helper
and authenticated inputs. Static installers extract temporarily before any live replacement.

For controlled or offline updates, supply inputs to the ordinary Agent installer/updater:

```sh
export NETDATA_DEM_RELEASE=/absolute/path/target-release.json
export NETDATA_DEM_ARCHIVE=/absolute/path/target-runtime.tar.zst
export NETDATA_DEM_KEYRING=/absolute/path/release-public.gpg
```

`NETDATA_DEM_KEYRING` is optional when `runtime.offline_keyring` is configured. A manual-policy installation may instead
use `NETDATA_DEM_ALLOW_DOWNLOAD=1`. Pass these variables through the privilege boundary deliberately; they do not
change saved consent. Temporary helper/input files are cleaned after the attempt; retain original offline inputs
if a retry must work without network access.

There is **no automatic Agent file backup or restore**. A failure after service shutdown reports its original error
and leaves the installation for repair. Rerun the same Agent installation with its matching companion, then start it
when validation succeeds. If the service was stopped by the failed attempt, the retry preserves that stopped state;
start it explicitly afterward. For a different target, resolve any pending runtime transition first. To return to an
older Agent, reinstall that Agent through its normal installer and restore its runtime selection through the runtime
CLI. `runtime rollback` changes runtime selection only; it cannot repair Agent executables.

Runtime `status` reports prepared/previous selections. Never delete ownership records to force progress. Unchanged
runtime contents are verified without creating a new transition or replacing the previous-runtime recovery point.
For an instance with custom IPC, export the same `NETDATA_PIPENAME` or `NETDATA_RUN_DIR` used by its Agent; nonempty values
must be absolute paths. Nonroot installations use their own CLI and executable for stop/start.

For a protected preload, stop the Agent and publish configuration pointing to the already sealed matching target
root. Run the installer with `--dont-start-it` and `NETDATA_DEM_PRELOAD_UPDATE=1`. The installer validates the target
and leaves the service stopped. The deployment owner retains previous Agent artifacts, configuration and sealed roots
if rollback is needed, and must stop every consumer before replacing or removing a preload. The installer does not
copy, restore or remove either preload root.

### Docker pair switching

Use `/usr/libexec/netdata/netdata-dem-docker` from the exact pinned target image, with the same service account,
configuration, `/var/lib/netdata` volume and host-policy mapping as the deployment. The wrapper runs as the service
account and supports these operations:

| Order | Target-image operation | Deployment owner action |
|---|---|---|
| 1 | `fetch --allow-download --output /var/lib/netdata/dem-pair-inputs/target` | Create an accessible input parent outside `dem/runtime`; keep the old image running during fetch. Offline `--release`/`--archive` inputs are also supported. |
| 2 | — | Stop every old consumer, including other containers sharing this store. |
| 3 | `prepare --release .../release.json --archive .../runtime.tar.zst --keyring .../release-public.gpg` | Use the retained inputs. Preparation does not activate the target. |
| 4 | `activate`, then `status` | Start the pinned matching target image only after success. |
| Recovery | `rollback`, then `status` with the previous image | Keep consumers stopped, restore the previous image and its configuration, then start it. `rollback --prepared-only` cancels an unactivated preparation. |

Keep the authenticated inputs for retry after an older image has run: its garbage collector may not recognize new
pending-state protection. Re-running `prepare` reimports from those inputs. The wrapper does not create/replace
containers, change image tags, migrate volumes or manage host policy. Preserve the existing machine-ID host mapping.

## Release trust and offline operation

Explicit CLI installation, ensure, repair and preload verify signed release metadata, archive digest and
bounds, architecture and final private paths. Integrity and execution compatibility checks remain mandatory. These
commands select or seal the verified release without executing a browser.

`dem.plugin runtime qualify` is an optional local diagnostic against the installed release. It runs the contract
checks, a sandboxed journey and Lighthouse, and requires process drainage; it reports its actual result and the calling
account and environment. A skipped diagnostic is not a passing result. Failure does not undo installation or selection.

- **Online.** Online operations fetch the package public keyring from
  `https://repository.netdata.cloud/netdatabot.gpg.key` for each preparation attempt, including when `--release` is
  combined with `--allow-download`. Trust follows that HTTPS endpoint and the system TLS trust store. A key fetch or
  verification failure blocks that attempt without stopping the installed runtime, and no stale cached key is used.
- **Offline.** `--release` with `--archive` is fully offline and uses only the keyring named by
  `runtime.offline_keyring` (or the explicit retained `--keyring` for `fetch`/`prepare`). This local trust is never an
  online fallback, and keeping the offline keyring current is the administrator's job. An offline installation still
  permits future Agent updates to acquire their companion online unless
  `--manual-updates` is given.
- **Self-trust.** The descriptor and archive cannot supply their own verification key.
- **What signatures prove.** They authenticate the Agent identity and complete preparation instructions. They
  cannot detect withheld updates or guarantee freshness on a new installation.
- **Pairing.** New descriptors bind the exact Agent version, channel and source commit. Changing that pair requires
  the installation owner's `prepare`/`activate` transition, not a floating runtime update. Legacy sequence metadata
  remains readable for migration and recovery. Downgrade restores a matching Agent/runtime pair explicitly.
- **No network at run time.** Ordinary execution and `status` fetch no key, and an installed runtime keeps working
  offline.

## Status, retry and removal

Use `status --agent /path/to/netdata` for an installed Agent. For a separately registered deployment:

```sh
sudo sh ./netdata-dem-runtime.sh policy-status --deployment netdata-monitoring
```

The helper keeps root-owned registrations under `/var/lib/netdata-dem-runtime-host-policy` and generated profiles
under `/etc/apparmor.d/netdata-dem-launcher-*`. It checks policy on disk separately from policy loaded in the kernel.
Repeat setup after a failed operation; it preserves enough ownership information to recognize its own work.
Administrator-edited files, disable markers and unknown state cause refusal rather than automatic replacement.

For managed removal, stop the Agent and every standalone runtime consumer, prevent concurrent installation commands,
then run `uninstall --agent /path/to/netdata`. The existing runtime drain check must succeed before policy cleanup.
Agent uninstallation uses that same check before erasing files. A refused drain preserves the runtime and policy.
If policy cleanup fails after runtime removal, the error distinguishes the unfinished cleanup; retry the helper.

Low-level nonroot `dem.plugin runtime uninstall` cannot remove administrator-owned policy. Use the high-level helper
to complete host cleanup. Multiple registrations sharing an attachment share one policy; removing one registration
preserves the others.

For Docker, after stopping consumers and retiring the deployment, remove its kickstart-created host registration
with the printed token. When creating a registration, the host retains a root-owned copy of its policy helper
under `/var/lib/netdata-dem-runtime-host-policy/docker-helpers`. The commands below work even after the container
and image have been deleted. Successful removal also retires that helper and leaves Docker volumes in place.

```sh
sudo sh ./kickstart.sh --dem-docker-policy-status TOKEN
sudo sh ./kickstart.sh --dem-docker-policy-remove TOKEN --deployments-stopped
```

For a registration created explicitly with the low-level helper, first stop and retire every affected deployment.
Then unregister it:

```sh
sudo sh ./netdata-dem-runtime.sh policy-remove \
  --deployment netdata-monitoring --deployments-stopped
```

This declaration is the administrator's responsibility. The helper does not infer retirement from missing container
paths or PIDs. Container deletion alone cannot revoke host policy. Registered preloads also prevent implicit Agent
prefix deletion until their deployment retirement is explicitly handled.

For a preload prepared through the high-level helper, retire its registration with
`uninstall --agent /path/to/netdata --root /absolute/preload/path --deployments-stopped`.
This removes the policy registration, retaining the administrator-owned preload files.
When uninstalling an Agent configured for protected preloads, pass `--dem-deployments-stopped` to
`netdata-uninstaller.sh` after stopping and retiring every deployment. Neither `--yes` nor `--force` makes that
declaration. The uninstaller checks managed runtime drainage before both direct file removal and native package
removal; ordinary package-owned runtime cleanup remains the package manager's responsibility.

## Troubleshooting the browser sandbox

If qualification reports a Chromium sandbox error, inspect the host's kernel audit messages for AppArmor namespace
denials, for example:

```sh
sudo journalctl -k --since '10 minutes ago'
```

Correct the administrator-owned policy, then retry the failed browser job or explicit qualification. Retry installation
only if its policy setup or integrity validation failed. Containers also need namespace and seccomp settings that allow Chromium's sandbox. Do not disable Chromium's sandbox or global host
restrictions to make qualification pass.

A successful qualification from a terminal tests that terminal's account and environment, not the Agent service:
AppArmor, SELinux and container policy can differ. After setup and restart, confirm that a journey and a Lighthouse
audit run through the actual Agent service.

## Security scope

The profile permits user namespaces so Chromium can run its own sandbox; it does **not** add AppArmor confinement or
exclusively authorize signed browser bytes. A service account can replace files in its managed store. Other unconfined
processes may adopt named profiles, and matching paths in other mount namespaces may receive the same attachment.
Root-owned native payloads protect file replacement but do not, by themselves, prevent profile adoption.
Administrator edits and disable markers stay authoritative.

The AppArmor allowance matches the registered runtime root followed by `generations/*/chrome`. The generated launcher
is stable across paired runtime releases, so new generations and changed internal Chromium directories
need no new root action. Moving the runtime root requires explicit host setup for the new root.

The helper leaves global AppArmor settings, administrator parser settings and other profiles unchanged. Browser
sandboxing, unprivileged execution, supervision and runtime verification remain required. Successful terminal
qualification does not prove that the service works (see
[Troubleshooting the browser sandbox](#troubleshooting-the-browser-sandbox)).

## Configuration

The `runtime:` section of `dem.conf` configures the runtime. Changes apply after restarting the plugin.

```yaml
runtime:
  mode: managed
  cache_path: ''
  max_bytes: 4294967296
  previous_retention_days: 7
  # Optional override; omit to use this Agent's version/channel/architecture URL.
  # release_url: https://mirror.example.invalid/dem-runtime/stable/v2.12.0/arm64/release.json
  offline_keyring: /absolute/path/netdatabot.gpg.key  # required only for offline operations
```

| Option | Default | Meaning |
|---|---|---|
| `mode` | `native` on DEB/RPM builds, `managed` elsewhere | `managed` (a store DEM maintains), `native` (the package-manager payload) or `preloaded` (a sealed preload). |
| `cache_path` | `NETDATA_LIB_DIR/dem/runtime` | Managed store directory, absolute. Managed mode only. |
| `max_bytes` | `4294967296` (4 GiB) | Logical allowance of the managed store. |
| `previous_retention_days` | `7` | Grace for the most recently used obsolete release (0-36500). |
| `release_url` | Exact Agent companion URL on release builds; empty on unstamped builds | HTTPS signed descriptor URL. An override must still match the Agent identity and architecture; an explicit empty string disables online discovery. |
| `offline_keyring` | empty | Absolute path to an administrator-provisioned OpenPGP public keyring for offline operations. |
| `preload_path` | empty | Sealed preload root. Requires `mode: preloaded`. |

The managed budget also covers temporary overlap during preparation. After A→B, importing C can still protect A
until C has been verified and prepared; allow room for A, B and incoming C. A budget that fits only B+C can therefore
refuse the attempt while preserving B. One recovery pair limits the rollback promise, not peak disk usage.

## Managed store and retention

- **Lifetime.** The selected runtime stays installed until explicitly uninstalled, even with no browser monitors.
- **Allowance.** The 4 GiB logical allowance includes preparation workspace and reservations. It is not a physical
  disk quota.
- **Protection.** Processes, executions, the prepared pair and the retained rollback pair protect their releases.
  Protected files can prevent a new installation within the allowance; pressure never silently drops the rollback pair.
  Preparing a different target replaces that rollback point with the currently selected pair.
- **Cleanup.** Unprotected old releases are removed hourly, with the `previous_retention_days` grace for the most
  recently used obsolete release; storage pressure can shorten that grace.
- **Location.** Use the same store and service account for the CLI and the running plugin. In Docker, persist the store
  (under `/var/lib/netdata`) to keep the installation across container replacement.

## Runtime CLI reference

`dem.plugin runtime` is the low-level command behind the helpers above. Run it as the consuming service account, after
any required host policy is prepared, with the same installation directory and configuration as the running plugin.
`dem.plugin` is in the Agent's plugin directory, for example `/usr/libexec/netdata/plugins.d/`
(`/opt/netdata/usr/libexec/netdata/plugins.d/` on static installations).

### Commands

| Command | What it does | Modes |
|---|---|---|
| `install` | Verifies and selects the matching release. Permits companion acquisition during Agent updates unless `--manual-updates` is given. | managed |
| `update` | Compatibility command: reports how to update through the Agent installation owner; changes nothing. | every |
| `fetch` | Verifies and retains the target descriptor, archive and public keyring outside the managed store; no selection change. | every |
| `prepare` | Imports and retains the exact target without activation; verifies identical selected contents as unchanged. | managed |
| `activate` | Selects the prepared target after drainage, or validates an already selected exact target. | managed |
| `rollback` | Restores the retained previous selection after drainage. `--prepared-only` cancels only this target's unactivated preparation. | managed |
| `repair` | Removes damaged generations and rebuilds the requested release, even an older one or the same one. | managed |
| `uninstall` | Clears the selection and removes every managed generation, once nothing uses them. | managed |
| `preload` | Verifies, prepares and seals a protected preload at `--root`. | every |
| `preload-validate` | Installation-owner check: verifies the configured preload matches this helper's exact Agent identity, without executing a browser. | preloaded |
| `status` | Reports the selected installation and storage inventory without launching a browser. | every |
| `qualify` | Optional local diagnostic: checks the execution contract and runs a sandboxed journey and Lighthouse against the installed release, requiring process drainage. | every |
| `ensure` | First enrollment for the Docker host command: reuses a usable selection or downloads and selects the matching release, allowing later paired acquisition. Refuses to run as root. | managed |

In `native` mode, managed mutations refuse and point to the package manager; in
`preloaded` mode they require `runtime.mode: managed`. `preload`, `status` and `qualify` work in every mode.
`setup-plan`, `pair-plan` and the `native-*` commands are internal interfaces of the installation owners and native package
hooks; do not call them directly.

### Flags

| Flag | Commands | Meaning |
|---|---|---|
| `--allow-download` | `install`, `fetch`, `prepare`, `repair`, `preload`, `ensure` | Download the package keyring and the archive, and the signed release metadata from `runtime.release_url` unless `--release` names a local descriptor. |
| `--release FILE` | same | Absolute path to a local signed release descriptor. |
| `--archive FILE` | same | Absolute path to the matching archive for a fully offline operation. Requires `--release`. Give exactly one of `--archive` and `--allow-download`. |
| `--manual-updates` | `install`, `repair` | Require explicit inputs or one-run download consent for later paired Agent updates. |
| `--output DIR` | `fetch` | New absolute directory for retained authenticated inputs, outside the managed store. |
| `--keyring FILE` | offline `fetch`, `prepare` | Explicit retained public keyring. |
| `--prepared-only` | `rollback` | Cancel an unactivated preparation for this target; do nothing if none exists. |
| `--root DIR` | `preload` | Absolute final preload root, new or empty. |
| `--cache DIR` | all but `preload` | Managed store directory; overrides `runtime.cache_path`. |
| `--config-dir DIR` | all | Absolute directory that holds the administrator `dem.conf`. Terminal sessions ignore the Agent's configuration-root environment variables, so use it when the configuration is not in the default place. |
| `--expected-root DIR`, `--cancel-on-stdin-close` | `ensure` | Used by the Docker host command. |

Once its configuration loads, every command prints exactly one JSON report, failures included. Its `state` is
`installed`, `inputs-verified`, `qualified`, `uninstalled`, `unavailable`, `incompatible`, `damaged` or `unknown`; `ensure` adds an `outcome` of
`already-selected`, `newly-selected` or `failed`. A failure also sets `setup_error` and a nonzero exit status.

### Install and qualify

```sh
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, fetch the signed release matching this Agent:
dem.plugin runtime install --allow-download
# Require explicit acquisition on subsequent paired updates:
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst --manual-updates
# Later updates use the Agent installer/updater or deployment owner.
dem.plugin runtime status
dem.plugin runtime qualify
```

- `qualify` explicitly tests browser execution for the installed release, in the calling account and environment only
  (see [Troubleshooting the browser sandbox](#troubleshooting-the-browser-sandbox)).
- Runtime switching waits for its consumers to stop. A failure before activation leaves the previous runtime selected;
  a later service-start failure leaves the target selected. Agent files follow the installer's normal repair behavior.
- Ctrl-C or SIGTERM asks the command to cancel at the next safe point. If qualification launched a browser, the command
  waits until its supervisor confirms that the browser and its descendants have exited. A process stuck in
  uninterruptible kernel I/O can keep this wait open, and further interrupts do not bypass it. Killing the command with
  SIGKILL leaves drainage unverified: preserve the execution's files and investigate the stuck process before retrying
  removal or repair.

### Status

`status` reports the selected installation and the storage inventory. It does not report the release that another
running process holds; a running plugin keeps its startup release until restart. Status checks execution eligibility,
not release-stamp equality. Installation owners use explicit pair validation (`native-status` for native packages and
`preload-validate` for protected preloads).

Managed reports also show any prepared transition and retained previous pair. These are persisted selections, not a
claim about the runtime used by another live process. Use normal Agent repair/reinstallation when Agent files
also need restoration; runtime rollback alone does not restore Agent executables.

- If selection or generation metadata is damaged, `status` still reports the available inventory, identifies the
  metadata errors and exits nonzero. It also exits nonzero when no release is selected.
- A healthy selected release stays identified even when an older release is damaged.
- Healthy metadata naming another execution contract remains readable. Managed `status` preserves the selected identity
  and inventory, reports `incompatible` and exits nonzero. Native/preloaded status preserves the incompatibility error
  without misreporting metadata damage. Use a compatible Agent/runtime combination; repairing ownership proof is not a
  compatibility fix. Unknown or corrupt proof still blocks destructive operations.
- An incomplete ownership or storage inspection is reported as unknown, not as an empty installation.

### Repair

Stop all DEM and standalone consumers and let their executions drain, then repair the managed store:

```sh
dem.plugin runtime repair --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, explicitly request the configured exact companion with --allow-download.
```

- **What it does.** Repair removes damaged generations and rebuilds the requested release, including a same-release
  reinstall. It uses the same signed descriptor, service account, store and integrity validation as install; it does not
  execute a browser.
- **Older releases.** Use the matching older Agent and authenticated runtime inputs. Repair still verifies the helper’s
  exact target identity; there is no independent runtime publication sequence.
- **On failure.** A healthy selection of a different release stays selected; ordinary retention still applies to other
  unselected releases. If repair cleared the selection it was rebuilding, a failed preparation or validation leaves
  the runtime unavailable, and the old selection cannot be rolled back. Correct the reported problem, retry repair, then
  restart DEM.
- **Progress.** The report shows candidate progress separately from the final selected installation.
- **Scope.** Repair applies only to private managed stores. Immutable administrator-owned preloads are replaced through
  their own lifecycle.
- **Pending pair.** Resolve an interrupted paired update through its installation owner before repair. If an external
  fault also corrupts the retained recovery generation, rollback refuses those bytes and repair keeps the pending
  ownership guard. Inspect `runtime status`, preserve the retained inputs and ownership records, and investigate the
  storage damage. Interrupted-update recovery does not promise recovery from simultaneous loss of its saved pair.

### Uninstall

```sh
# Stop every DEM and standalone consumer first:
dem.plugin runtime uninstall
```

- Uninstall refuses while any generation has a running consumer, a preparation or an unresolved execution, and the
  refusal leaves the installation intact.
- If child completion cannot be proved, execution protection remains: neither a timeout nor a missing PID allows
  removal.
- A failed selection write after publication reports uncertainty; inspection establishes the selected state before
  cleanup proceeds.
- It removes runtime state only. Host policy cleanup (the helper's `uninstall --agent`, preload and Docker
  registrations, `policy-remove`) is in [Status, retry and removal](#status-retry-and-removal).

Repair and uninstall can remove generations with damaged executable metadata only when independent ownership records
prove that removal is safe. They refuse active consumers, preparation, unresolved executions, missing safety structure
and storage access errors. There is no force-delete option: dead PIDs and timeouts do not prove that an execution
finished. Do not edit selection records or remove generation directories by hand. Deleting individual ownership records
or copying a live store is outside this recovery guarantee.
