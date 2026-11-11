# Browser runtime host setup

DEM journeys and Lighthouse need an explicitly installed browser runtime. RUM does not. The runtime includes its own
Node and Chromium; installing it does not require a system browser. See [runtime configuration and release
verification](/src/go/plugin/dem/README.md#runtime-setup-and-retirement) before preparing it.

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
host's namespace restrictions stay enabled. Persist `/var/lib/netdata` on local storage. For the first install,
the container needs access to a published signed release. Currently you must set `runtime.release_url` in its
`dem.conf` to the signed metadata URL for the container's architecture. On the daemon host, replace `netdata`
with the running container's name:

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
then use the [explicit runtime CLI](/src/go/plugin/dem/README.md#runtime-setup-and-retirement) as the service user.

## Preloads and independent updates

The high-level `preload` command accepts `--root /absolute/final/path` with the same signed offline or download inputs.
It prepares policy before the existing qualification and sealing operation. Never move a sealed preload. Its separate
immutable/deployment-owned lifecycle still applies.

The allowance matches the registered root followed by `generations/*/chrome`. The generated launcher is stable across
compatible independent runtime releases. New generations and changed internal Chromium directories therefore require
no new root action. Moving the runtime root requires explicit host setup for the new root.

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

## Security scope

The profile permits user namespaces so Chromium can run its own sandbox; it does **not** add AppArmor confinement or
exclusively authorize signed browser bytes. A service account can replace files in its managed store. Other unconfined
processes may adopt named profiles, and matching paths in other mount namespaces may receive the same attachment.
Root-owned native payloads protect file replacement but do not, by themselves, prevent profile adoption.

The helper leaves global AppArmor settings, administrator parser settings and other profiles unchanged. Browser
sandboxing, unprivileged execution, supervision and runtime verification remain required. Successful terminal
qualification does not establish that a differently confined service or container will work; verify both journeys and
Lighthouse through the actual deployment after setup and restart.
