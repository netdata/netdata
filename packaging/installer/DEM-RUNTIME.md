# Browser runtime host setup

DEM journeys and Lighthouse audits need an explicitly installed browser runtime. RUM does not. The runtime is one
signed, portable bundle for Linux amd64 or arm64 that contains its own Node, Chromium, Playwright, Lighthouse and their
JavaScript adapters. Installing it does not require, use or discover a system browser, Node or npm.

:::info

The browser runtime is experimental, and its public release publication is not yet confirmed. `runtime.release_url` in
`dem.conf` has no default, and finding the latest release needs it: kickstart, the Docker command,
`--allow-download` without `--release`, and automatic updates. Set it to the signed release URL for the host's
architecture. Offline setup needs a signed release descriptor, its archive and an administrator-provisioned keyring;
install offline with `--manual-updates` unless `release_url` is set, or each daily update check logs a failure.

:::

Enabling a browser monitor never downloads or installs the runtime. Without one, browser jobs wait for it and report
no website failures. **Restart the Agent after every installation, update or repair, including the first.** A running
plugin keeps the release it acquired at startup. The native package's hooks restart a running Agent themselves.

On Ubuntu 24.04, default AppArmor restrictions prevent an ordinary unconfined process from creating Chromium's
sandbox. `netdata-dem-runtime.sh` prepares the host allowance and then runs the existing runtime CLI as Netdata's
nonroot service account. Neither DEM nor its automatic updater receives host-policy privileges.

| Installation | Explicit setup | Policy owner |
|---|---|---|
| Native DEB/RPM | Install the optional `netdata-dem-runtime` package | Package manager and package hooks |
| Static or source | Installed `netdata-dem-runtime.sh`, or kickstart with `--install-dem-runtime` | Administrator-run helper |
| Local rootful Docker | Run kickstart on the daemon host for the named running container, then restart it after successful setup. | Host deployment administrator |
| Protected preload | Prepare the policy before qualification/sealing at the preload's final path | Preload/deployment administrator |

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
  runtime consumer (such as a standalone CLI qualification) still runs, qualify the new payload and restart the Agent
  if it was running. Installing into an image or chroot is not supported yet.
- **AppArmor.** On hosts with AppArmor, the package ships a profile for its root-owned Chromium executable and loads it
  before qualification. The profile permits the user namespaces Chromium needs for its sandbox; it does not add browser
  confinement.
- **Failures.** Administrator policy stays authoritative. A policy or qualification failure keeps browser admission
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

The same setup is included in `kickstart.sh --install-dem-runtime`. Existing `runtime.release_url` and trust
requirements still apply. Initial installation enables daily automatic preparation unless `--manual-updates` is
specified. **Restart the Agent explicitly to activate the first installation or an update.** The helper does not
restart it or change the Agent's separate update policy.

Offline setup uses the same helper with an administrator-provisioned `runtime.offline_keyring`:

```sh
sudo /opt/netdata/usr/libexec/netdata/netdata-dem-runtime.sh install \
  --agent /opt/netdata/usr/sbin/netdata \
  --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
```

Do not add `--allow-download` to an offline import. Future automatic runtime preparation remains enabled unless
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
published signed release. Currently you must set `runtime.release_url` in its `dem.conf` to the signed metadata URL for
the container's architecture. On the daemon host, replace `netdata` with the running container's name:

```sh
sudo sh ./kickstart.sh --install-dem-runtime --docker-container netdata
```

Use this command with a Netdata image you trust to supply code that runs as root on the host. Kickstart
reads the running deployment’s exact local image ID and creates a temporary container from that image.
It copies the host setup scripts out and removes the temporary container and its anonymous volumes.
The temporary container never starts. Changes inside the running deployment cannot replace these scripts.

The host helper prepares AppArmor on hosts that need it. The running deployment downloads, verifies and unpacks the signed runtime as
the container’s nonroot service user. The command waits for a sandboxed journey and Lighthouse check to finish
while the Agent continues monitoring. Preparation has no overall time limit. An error or administrator
cancellation returns a failure. After a successful first installation, activate the runtime when ready:

```sh
docker restart netdata
```

Keep the printed registration token for retirement. On restricted hosts, run kickstart once per deployment,
even if another container already registered the same launcher path. Each registration protects the shared policy.
First installation enables daily update preparation. Restart the container after a newer release has been
prepared to activate it. Until that restart, the running plugin keeps its current release. Running kickstart
again reuses a valid selection offline and preserves its update policy. An already-selected result by itself
requires no additional restart.

The host command and sandboxed browser jobs were tested on Ubuntu 24.04 amd64 and Debian 13 amd64. Automatic
AppArmor setup on hosts that restrict unprivileged user namespaces is qualified for Ubuntu 24.04. On hosts
without that restriction, the command skips the policy installation. The standard runtime root inside the
container is `/var/lib/netdata/dem/runtime`. Rootless, nested and remote Docker, custom confinement and custom
runtime roots need separate administrator setup. For offline inputs or custom roots, use a trusted copy
of the supplied host helper with `policy-prepare --deployment TOKEN --runtime-root /execution/visible/root`,
then use the [runtime CLI](#runtime-cli-reference) as the service user.

## Preloads and independent updates

A protected preload is a sealed runtime at a fixed final path, for an immutable image or a read-only deployment. It
lives outside the managed store and its automatic retention and updates.

The high-level `preload` command accepts `--root /absolute/final/path` with the same signed offline or download inputs.
It prepares policy before the existing qualification and sealing operation. The low-level equivalent, run as the
service account after any required host policy is prepared:

```sh
dem.plugin runtime preload --root /opt/netdata-dem-runtime --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
```

Then point the plugin at it in `dem.conf`:

```yaml
runtime:
  mode: preloaded
  preload_path: /opt/netdata-dem-runtime
```

- **Root.** It must be new or empty. The command verifies and qualifies the complete release, then seals its identity
  and final location into the root. Keep it private and owned by the consuming service account; it may be mounted
  read-only.
- **Lifecycle.** Preloads are outside managed retirement and repair, and their updates are manual. Stop **every**
  consumer before replacing or removing one, prepare a replacement at a new final path, and restart DEM to switch.
  Never move a sealed preload.
- **Native packages** use their coordinated package hooks instead.

## Activation and automatic updates

Each DEM process acquires the selected release when it starts and keeps it for its whole lifetime. A process started
without a runtime stays unavailable until restarted, even after an installation succeeds. Runtime releases can update
their components without an Agent upgrade while keeping execution contract 2. Every run records the actual release
version, archive digest and execution contract.

Explicit installation (`install`, `repair`, or the helpers that call them) enables automatic preparation by default
and prints this policy before proceeding. `--manual-updates` on install or repair disables it; `update` keeps the
installed policy. With automatic preparation on, a managed runtime works like this:

- **Schedule.** The running plugin checks for a newer signed release within five minutes of starting, then every 23-25
  hours.
- **Preparation.** It downloads and qualifies the newer release beside the working one and selects it for the next DEM
  start; `runtime status` shows the selection.
- **No restart.** It never restarts the Agent, so security fixes stay inactive until the administrator or deployment
  system restarts DEM.
- **Shared slots.** Qualification shares the browser execution allowance (`browser.concurrency`), so monitors can wait
  while these local checks run.
- **Failures.** A failed check keeps the selected release and retries at the next scheduled check. The plugin logs
  `DEM runtime update failed` warnings, and a notice when a later check recovers, separately from website
  measurements.

Native packages update through the package manager. Preloads are manual.

## Release trust and offline operation

Installation, update and repair verify the archive against a signed release descriptor, prepare private paths, then
run a local sandboxed journey and a Lighthouse audit before selecting the release.

- **Online.** Online operations fetch the package public keyring from
  `https://repository.netdata.cloud/netdatabot.gpg.key` for each preparation attempt, including when `--release` is
  combined with `--allow-download`. Trust follows that HTTPS endpoint and the system TLS trust store. A key fetch or
  verification failure blocks that attempt without stopping the installed runtime, and no stale cached key is used.
- **Offline.** `--release` with `--archive` is fully offline and uses only the keyring named by
  `runtime.offline_keyring`. This local trust is never an online fallback, and keeping the offline keyring current is
  the administrator's job. An offline installation still enables future automatic online preparation unless
  `--manual-updates` is given.
- **Self-trust.** The descriptor and archive cannot supply their own verification key.
- **What signatures prove.** They authenticate publication order and the complete preparation instructions. They
  cannot detect withheld updates or guarantee freshness on a new installation.
- **Ordering.** Normal installs and updates compare against the currently selected publication sequence and reject
  older releases. An explicit repair to an older authenticated release resets that baseline.
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

Correct the administrator-owned policy, then retry the failed installation or qualification. Containers also need
namespace and seccomp settings that allow Chromium's sandbox. Do not disable Chromium's sandbox or global host
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
is stable across compatible independent runtime releases, so new generations and changed internal Chromium directories
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
  release_url: https://artifacts.example.invalid/dem-runtime/latest/arm64.json
  offline_keyring: /absolute/path/netdatabot.gpg.key  # required only for offline operations
```

| Option | Default | Meaning |
|---|---|---|
| `mode` | `native` on DEB/RPM builds, `managed` elsewhere | `managed` (a store DEM maintains), `native` (the package-manager payload) or `preloaded` (a sealed preload). |
| `cache_path` | `NETDATA_LIB_DIR/dem/runtime` | Managed store directory, absolute. Managed mode only. |
| `max_bytes` | `4294967296` (4 GiB) | Logical allowance of the managed store. |
| `previous_retention_days` | `7` | Grace for the most recently used obsolete release (0-36500). |
| `release_url` | empty | HTTPS URL of the signed latest release for the host's architecture. The value above is an example. |
| `offline_keyring` | empty | Absolute path to an administrator-provisioned OpenPGP public keyring for offline operations. |
| `preload_path` | empty | Sealed preload root. Requires `mode: preloaded`. |

## Managed store and retention

- **Lifetime.** The selected runtime stays installed until explicitly uninstalled, even with no browser monitors.
- **Allowance.** The 4 GiB logical allowance includes preparation workspace and reservations. It is not a physical
  disk quota.
- **Protection.** Processes and executions protect the releases they use. Protected files can prevent a new
  installation within the allowance.
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
| `install` | Prepares, qualifies and selects a signed release in the managed store. Enables automatic update preparation unless `--manual-updates` is given. | managed |
| `update` | Prepares, qualifies and selects a newer signed release. Keeps the installed update policy. | managed |
| `repair` | Removes damaged generations and rebuilds the requested release, even an older one or the same one. | managed |
| `uninstall` | Clears the selection and removes every managed generation, once nothing uses them. | managed |
| `preload` | Prepares, qualifies and seals a protected preload at `--root`. | every |
| `status` | Reports the selected installation and storage inventory without launching a browser. | every |
| `qualify` | Runs a sandboxed journey and a Lighthouse audit against the selected release. | every |
| `ensure` | First enrollment for the Docker host command: does nothing when a usable release is selected, otherwise downloads, qualifies and selects one with automatic preparation. Refuses to run as root. | managed |

In `native` mode, `install`, `update`, `repair`, `uninstall` and `ensure` refuse and point to the package manager; in
`preloaded` mode they require `runtime.mode: managed`. `preload`, `status` and `qualify` work in every mode.
`setup-plan` and the `native-*` commands are internal interfaces of `netdata-dem-runtime.sh` and the native package
hooks; do not call them directly.

### Flags

| Flag | Commands | Meaning |
|---|---|---|
| `--allow-download` | `install`, `update`, `repair`, `preload`, `ensure` | Download the package keyring and the archive, and the signed release metadata from `runtime.release_url` unless `--release` names a local descriptor. |
| `--release FILE` | same | Absolute path to a local signed release descriptor. |
| `--archive FILE` | same | Absolute path to the matching archive for a fully offline operation. Requires `--release`. Give exactly one of `--archive` and `--allow-download`. |
| `--manual-updates` | `install`, `repair` | Disable automatic update preparation. |
| `--root DIR` | `preload` | Absolute final preload root, new or empty. |
| `--cache DIR` | all but `preload` | Managed store directory; overrides `runtime.cache_path`. |
| `--config-dir DIR` | all | Absolute directory that holds the administrator `dem.conf`. Terminal sessions ignore the Agent's configuration-root environment variables, so use it when the configuration is not in the default place. |
| `--expected-root DIR`, `--cancel-on-stdin-close` | `ensure` | Used by the Docker host command. |

Once its configuration loads, every command prints exactly one JSON report, failures included. Its `state` is
`installed`, `qualified`, `uninstalled`, `unavailable`, `damaged` or `unknown`; `ensure` adds an `outcome` of
`already-selected`, `newly-selected` or `failed`. A failure also sets `setup_error` and a nonzero exit status.

### Install, update and qualify

```sh
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, use the administrator-configured signed latest release:
dem.plugin runtime install --allow-download
# Disable automatic update preparation for a controlled or offline deployment:
dem.plugin runtime install --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst --manual-updates
dem.plugin runtime update --release /absolute/path/new-release.json --archive /absolute/path/new-runtime.tar.zst
dem.plugin runtime status
dem.plugin runtime qualify
```

- `qualify` repeats the install-time checks for the selected release, in the calling account and environment only
  (see [Troubleshooting the browser sandbox](#troubleshooting-the-browser-sandbox)).
- A successful update does not replace files used by running processes. Install and update errors keep the previous
  selection.
- Ctrl-C or SIGTERM asks the command to cancel at the next safe point. If qualification launched a browser, the command
  waits until its supervisor confirms that the browser and its descendants have exited. A process stuck in
  uninterruptible kernel I/O can keep this wait open, and further interrupts do not bypass it. Killing the command with
  SIGKILL leaves drainage unverified: preserve the execution's files and investigate the stuck process before retrying
  removal or repair.

### Status

`status` reports the selected installation and the storage inventory. It does not report the release that another
running process holds; a running plugin keeps its startup release until restart.

- If selection or generation metadata is damaged, `status` still reports the available inventory, identifies the
  metadata errors and exits nonzero. It also exits nonzero when no release is selected.
- A healthy selected release stays identified even when an older release is damaged.
- An incomplete ownership or storage inspection is reported as unknown, not as an empty installation.

### Repair

Stop all DEM and standalone consumers and let their executions drain, then repair the managed store:

```sh
dem.plugin runtime repair --release /absolute/path/release.json --archive /absolute/path/runtime.tar.zst
# Alternatively, explicitly request the configured latest release with --allow-download.
```

- **What it does.** Repair removes damaged generations and rebuilds the requested release, including a same-release
  reinstall. It uses the same signed descriptor, service account, store and browser qualification as install.
- **Older releases.** Repair may deliberately select an older authenticated release; ordinary updates reject lower
  publication sequences.
- **On failure.** A healthy selection of a different release stays selected; ordinary retention still applies to other
  unselected releases. If repair cleared the selection it was rebuilding, a failed preparation or qualification leaves
  the runtime unavailable, and the old selection cannot be rolled back. Correct the reported problem, retry repair, then
  restart DEM.
- **Progress.** The report shows candidate progress separately from the final selected installation.
- **Scope.** Repair applies only to private managed stores. Immutable administrator-owned preloads are replaced through
  their own lifecycle.

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
