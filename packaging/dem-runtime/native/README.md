# Optional native DEM runtime package

This independent CPack recipe assembles `netdata-dem-runtime` from a completed
`dem-runtime-release stage-native` tree. It does not rebuild the Agent, download
runtime components, execute the browser during packaging, or publish packages.
The first service-policy qualification targets are Ubuntu 24.04 and Rocky 9 using
systemd. Actual transaction and confinement qualification is still required.

Build the release tool from `src/go`, then stage the archive and configure this
project on a Linux package builder with CMake 3.21 or newer and dpkg/rpmbuild:

```sh
/path/to/dem-runtime-release stage-native \
  --manifest /build/manifest.json --archive /build/runtime.tar.gz \
  --output /build/payload
cmake -S packaging/dem-runtime/native -B /build/native-package \
  -DDEM_PAYLOAD_DIR=/build/payload -DDEM_ARCH=arm64 \
  -DDEM_VERSION=2026.10.29.1 -DDEM_RELEASE=1 -DDEM_PACKAGE_FORMAT=DEB
cpack --config /build/native-package/CPackConfig.cmake
```

Use `RPM` for Rocky. `DEM_ARCH` is explicitly `amd64` or `arm64` and must match
`native.json`; package version must equal its manifest version. Package release
is a positive integer. Use fresh build and staging directories. The recipe copies
staged modes/symlinks and assigns root ownership in the package. Neither CPack
stripping nor RPM ELF post-processing may rewrite the prepared payload. Automatic
private-library dependency/provides generation is disabled. Component notices,
SBOM/provenance and source inventories remain inside the staged runtime; source
archives must be published separately. The RPM license field refers to that
multi-component notice inventory and still needs release/legal validation.

Packages require `netdata-dem-runtime-api = 1`, provided by a compatible installed
`netdata-plugin-dem`. Debian uses versioned Pre-Depends; RPM declares ordinary and
pre/preun/posttrans dependencies. The Agent package must publish this capability
before these packages can be installed. This is a helper interface version, not
an exact Agent/runtime release pairing.

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
is active, then `runtime native-configure` qualifies the package
and opens admission. Restore the previously active Agent to activate it. Failed
qualification keeps admission closed, restores eligible non-browser monitoring
and preserves the qualification's nonzero status. Final removal restores that
monitoring after safe preparation and AppArmor profile unloading, before erasure, while Agent
and helper dependencies still exist. Removal post-hooks do not restart services.
This behavior needs real multi-package removal testing; shell fixtures establish
ordering, not native runtime safety or service availability.

The package owns `/etc/apparmor.d/netdata-dem-browser`, loaded by the host's
normal AppArmor boot service. It attaches only to the root-owned
`/usr/lib/netdata/dem-runtime/browsers/chromium-debian/chromium` executable.
This is a deliberate user-namespace exception, **not browser confinement**: the
special unconfined profile declares ABI 3, whose compatibility rules permit
user namespaces on restricted Ubuntu. The same file parses with AppArmor 3 and
4. Chromium still runs its own namespace/seccomp sandbox as the service user;
qualification must pass without disabling that sandbox.

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
is ready. Resolve the reported host policy or qualification failure, then reinstall
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
qualification result. Managed Docker provisioning remains a separate route.

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
qualification. Package signatures/repositories and runtime source publication are
outside this recipe.
