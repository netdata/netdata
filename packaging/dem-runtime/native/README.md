# Optional native DEM runtime package

`netdata-dem-runtime` is an optional component of the Agent's native package build.
The Agent's CPack configuration owns its version, distro revision, signing and
repository submission. This directory supplies component rules, hooks and policy;
its `CMakeLists.txt` is included by `Packaging.cmake`, not configured separately.
The initial lifecycle qualification targets remain Ubuntu 24.04 and Rocky 9 with
live systemd; package build coverage is separate from host qualification.

Native CI supplies a completed `stage-native` tree from the exact paired runtime
build. Its `release-info` must match the checkout's generated
`packaging/release-info` byte for byte. CI validates `native.json`, architecture
and the complete release identity before the distro builder runs. CMake consumes
the explicit staging directory without downloading components or executing a
browser, and requires no additional Python dependency in the Agent build.

From the Agent checkout inside a native package builder:

```sh
DEM_RUNTIME_PAYLOAD_DIR=/build/paired-runtime \
  packaging/build-package.sh DEB /build/native-package
```

Use `RPM` for RPM builders. Without `DEM_RUNTIME_PAYLOAD_DIR`, the build and
ordinary installation have no runtime payload. Runtime install rules are excluded
from full installs and selected only by their package component. The build script
uses the ordinary Agent CPack pass, then `CPackDEMRuntimeConfig.cmake` for the
runtime alone. That second pass sets runtime epoch `1` so Agent-version packages
upgrade experimental calendar-version packages. Agent/plugin epochs remain
unchanged. Both passes write to the normal package output directory and share
native version/revision encoding. No separate runtime package publisher is used.
DEB filenames omit the epoch, for example `netdata-dem-runtime_2.12.0_amd64.deb`,
while control metadata retains `Version: 1:2.12.0`. APT can ignore local-file
requests containing the epoch's colon; artifact tests cover this filename boundary.

The plugin declares Debian `Breaks` / RPM `Conflicts` against runtime versions
before epoch `1`. Calendar packages required only the helper API, so that old
dependency cannot force a paired upgrade. The transition constraint prevents a
completed new-plugin/old-runtime installation without making the runtime mandatory.
Debian uses `Breaks` so either package can unpack first within the coordinated
transaction. Missing or held companions leave the pair unchanged in the normal
updater, which does not permit removing the runtime to resolve the transaction.

The package preserves staged bytes, modes and relative symlinks with root
ownership. Stripping, RPM ELF rewriting and private-library dependency/provides
generation are disabled for the runtime. Source/license inventories remain inside
the payload; corresponding-source publication remains part of the paired release.

The runtime requires the exact final `netdata-plugin-dem` package version, which
in turn requires the matching Agent. Lifecycle prerequisites remain separate:
Debian Pre-Depends and RPM pre/preun require `netdata-dem-runtime-api = 1`, so the
old installed helper can safely drain before unpacking. Final configure/posttrans
uses the target helper and validates the exact pair. Internal `native-status` also verifies that pair for the updater.
Ordinary execution and `runtime status` instead check the admitted payload’s contract, platform, integrity and
provenance, without comparing plugin release stamps. Copying a compatible development plugin therefore needs no
special bypass. The Agent recommends the DEM plugin;
installations that disable weak dependencies may omit it. The plugin suggests this
runtime package, so installing the plugin alone does not install a browser. RPM
weak relationships are emitted only on distributions that support them; older
package managers receive no hard-dependency fallback.

The only executable payload prefix is `/usr/lib/netdata/dem-runtime`. The package
MUST NOT own or purge `/var/lib/netdata-dem-runtime`: root admission and
service-owned lifetime proof survive interruption/removal. Hooks embed their
lifecycle implementation because pre-unpack cannot depend on new package files.
They call the installed `/usr/libexec/netdata/plugins.d/dem.plugin` helper.

Before replacement/removal, hooks record whether the Agent was active in a
root-owned 0600 `/run/netdata-dem-runtime/restart-pending` file, stop it, verify
inactive state and invoke `runtime native-prepare`. A failed stop or unresolved
consumer refuses payload mutation. Debian service actions use
`deb-systemd-invoke`, preserving `policy-rc.d`; hooks do not enable, unmask or
force-start the service. Unknown or transitional systemd states are refused
explicitly. A failed service is stopped and verified inactive before admission
closure; it does not acquire restart intent. An originally inactive service stays
inactive.

After complete installation, hooks load the native AppArmor profile when AppArmor
is active, then `runtime native-configure` validates the complete package payload
as the service UID and opens admission. This verifies identity, integrity, bound
paths and drained ownership without starting Node or Chromium. Restore the
previously active Agent to activate it. Failed validation keeps admission closed,
restores eligible non-browser monitoring and preserves the nonzero status. Final
removal restores that monitoring after safe preparation and AppArmor profile
unloading, before erasure, while Agent and helper dependencies still exist. Removal post-hooks do not restart services.
This behavior needs real multi-package removal testing; shell fixtures establish
ordering, not native runtime safety or service availability.

Browser smoke tests are not package configuration gates. After installation,
`dem.plugin runtime qualify` is an optional local diagnostic that runs a sandboxed
journey and Lighthouse in the calling account and environment. Run it as the
service account; success does not certify a different service policy. A diagnostic
failure does not change package configuration success.

`ValidateAndReady` retains the persisted admission phase name `qualifying` for
older readers and interrupted transactions, but it now means candidate validation.
Healthy metadata from another execution contract remains readable for inspection
and admission closure; execution still refuses an incompatible payload. Missing or
corrupt ownership evidence continues to block destructive mutation.

The package owns `/etc/apparmor.d/netdata-dem-browser`, loaded by the host's
normal AppArmor boot service. It attaches only to the root-owned
`/usr/lib/netdata/dem-runtime/browsers/chromium-debian/chromium` executable.
This is a deliberate user-namespace exception, **not browser confinement**: the
special unconfined profile declares ABI 3, whose compatibility rules permit
user namespaces on restricted Ubuntu. The same file parses with AppArmor 3 and
4. Chromium still runs its own namespace/seccomp sandbox as the service user;
qualification must pass without disabling that sandbox.
Root ownership protects payload replacement; it does not make this profile an
exclusive browser authorization boundary. Other unconfined processes may adopt
named profiles, depending on host policy.

The profile is a Debian conffile and RPM `%config(noreplace)` file. Administrator
body/ABI edits and parser ABI overrides remain authoritative; keep the profile
name and attachment unchanged. An administrator disable marker at
`/etc/apparmor.d/disable/netdata-dem-browser` prevents package activation; hooks
never re-enable it. Active AppArmor requires working `apparmor_parser` and readable
kernel profile inventory for removal. Load/unload errors retain closed admission,
refuse the operation, and restore eligible non-browser monitoring. A removal retry
recognizes an already absent kernel profile. Old upgrade post-hooks never unload
new policy. Hosts without active AppArmor skip policy operations, including Rocky
with SELinux; no AppArmor dependency is imposed there.

Debian `remove` retains administrator conffiles, so the boot loader may later load
the residual policy even though its exact executable has been removed. `purge`
removes that conffile. If a reboot between remove and purge reloaded the residual
profile, purge does not unload it; it disappears at the next reboot. Its executable
is absent throughout. Hooks never delete administrator configuration themselves.
Managed/static runtime paths are not covered by this fixed native exception.

RPM can report a successful transaction despite a failing `%posttrans` scriptlet.
Check the scriptlet diagnostics and `dem.plugin runtime status` as the Netdata
service account; a successful package-manager exit alone does not mean the runtime
is ready. Resolve the reported host policy or payload validation failure, then reinstall
the package to rerun its coordinated preparation. Debian records a failing
configuration script as unconfigured; retry with `dpkg --configure netdata-dem-runtime`
after resolving the cause.

Restart intent survives same-boot hook retries and expires at reboot. Successful
restoration clears it. Policy-denied start clears it with an explicit manual
restart error, preventing an unexpected deferred start. Other restoration errors
retain it for repair. A partial/corrupt marker fails closed for inspection. The
durable native gate remains closed independently of this boot-scoped intent.
Abort hooks never infer payload health or reopen admission from hook arguments.

Chroot/image installation is explicitly refused before mutation when there is no
running systemd host (or `DPKG_ROOT` is set). Deferred image configuration is not
implemented; it needs a separate supported lifecycle rather than a false
ready result. Managed Docker provisioning remains a separate route.

Run the unprivileged observable hook tests:

```sh
python3 -m unittest discover -s packaging/dem-runtime/native/tests -v
```

Lifecycle tests replace service commands and the native helper with isolated stubs.
On Linux builders with CPack and the corresponding package tools, package tests
also build and extract real artifacts to compare executable bytes, modes, ownership
metadata and relative symlinks. They do not install the packages. Release
acceptance additionally requires real DEB/RPM upgrade/reinstall/remove/abort,
helper dependency ordering, interrupted hooks/reboot, both architectures,
AppArmor/SELinux/systemd confinement, policy-denied actions and actual browser
qualification. Generic distro-container package tests exclude the optional runtime
and assert no browser was installed; they do not replace live-systemd tests.
Package signing and repository submission use the normal Agent native jobs.
