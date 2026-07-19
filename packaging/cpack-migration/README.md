# Migrating RPM packaging from netdata.spec.in to CPack

This document explains the work on this branch to anyone who knows the
existing RPM packaging — especially the engineer who built and maintains it.
It describes why we did it, how we approached it, how we verified the result,
and what was deliberately left out. The complete working log of the session
that produced the branch sits next to this file in `cd.txt`; this is the
condensed, human version.

## Why

Netdata ships native packages through two parallel definitions of the same
product: the DEB side is driven by CPack from the `install()` rules in the
CMake tree, while the RPM side is a hand-maintained spec file whose `%build`
carries its own copy of the cmake option matrix and whose `%files` sections
enumerate the payload by hand. Two definitions of one thing drift, and the
drift is not hypothetical: the `ebpf-code-legacy` CPack component was gated
on a misspelled option name for years, so the DEB sub-package silently
stopped being built, while the RPM side — packaged manually by the spec —
kept shipping it. Nobody noticed, because nothing fails when a package
quietly ceases to exist.

The goal of this branch is to make the CMake install rules the single source
of truth for RPMs too, using the `cpack -G RPM` path that the `v2` builder
images were already wired to call (their `cpack-rpm.sh` predates this work —
the plumbing existed, only the CPack RPM configuration in the repo was
missing). The spec file remains untouched and remains authoritative for the
`v1` builder images; nothing changes in production until `builder_rev` is
flipped in `.github/data/distros.yml`, which is intentionally not part of
this branch.

## The approach: parity first, opinions later

The single organizing principle was that the spec is the contract. We did
not try to improve the RPM packaging while porting it; we tried to make
CPack produce the same packages the spec produces, quirks included, so that
the migration itself introduces zero behavioral change and every deliberate
improvement can be argued separately later. That means the port preserves
things one might be tempted to "fix" in passing: the `amzm` typo that keeps
xenstat permanently disabled, the unversioned `Requires: netdata-dashboard`,
the main package's `%pre` creating the user even on sysusers platforms, the
supplemental-group lists that differ between the main `%pre` and the user
package's `%post`, and the static changelog with its historical typos. Each
of these is reproduced and commented as deliberate, so the next reader does
not mistake fidelity for accident.

Where the DEB layout and the spec disagree about which package owns a file,
the RPM side follows the spec. A few payloads therefore route to different
components per format: `sensors3.conf`, the otel stock configs and `nd-mcp`
belong to the main RPM (the spec sweeps them up via its `%{_libdir}` glob
and sbin entries) while the DEBs keep them with their plugins, and the
swagger files belong to the dashboard RPM while the main DEB carries them.
This is expressed through a `NETDATA_PACKAGING_FORMAT` configure knob that
`build-package.sh` passes for both formats; with the knob unset everything
behaves exactly as before, which keeps every historical caller safe.

The spec's distro conditionals became configure-time predicates derived from
`/etc/os-release` (a new shared `NetdataOSRelease.cmake` module), mirroring
the macro families the spec keys on: the sysusers-capable platforms where
the netdata-user package only manages supplemental groups, the weak-deps
downgrade on EL 7 and Amazon Linux 2, the `cap_perfmon` versus
`cap_sys_admin` split for perf.plugin, and openSUSE's `%service_*` scriptlet
macro family. The scriptlets themselves are the spec's shell, ported
verbatim into files under `packaging/cmake/pkg-files/rpm/`, with one file
per rpmbuild-time variant since CPack embeds them into the generated spec
where the distro's own macros then expand. The one wrinkle worth knowing:
CPack generates one spec per component whose `Name:` is the sub-package's
own, so the scriptlets cannot use `%{name}` the way the spec does — the
values are spelled out.

File ownership was the part least amenable to porting by reading. Rather
than trusting our reading of the `%defattr` bands and `%attr` lines, we
built the reference RPMs first and interrogated them (`rpm -qp` over modes,
owners, flags and capabilities per file), then wrote the per-component
defaults and the explicit exception lists from that ground truth. The IBM MQ
tree gets special treatment: its two-thousand-entry file list is derived at
configure time from the same `MANIFEST.Redist` the install rule already
parses, classified into the spec's 0750/0640 bands, with the directories and
the versionless library symlinks the spec never packages excluded.

## What it took beyond metadata

Making the packages match turned out to require making the build
environments match. The spec builds through the distro `%cmake` macro, which
exports the hardened compiler and linker flags — `-D_FORTIFY_SOURCE`,
`-Wl,--as-needed` and friends — while `build-package.sh` invoked bare cmake.
The first parity run flagged this immediately as extra soname dependencies
and different fortified glibc symbols in every binary. The RPM path now
pulls the flags from the rpm build macros the same way `%cmake` does, with
SUSE handled specially because its macro passes the linker flags as cmake
arguments rather than through the environment. Two related discoveries are
documented in the code because they defy intuition: the protobuf hint the
spec passes (`Protobuf_LIBRARY=.../libprotobuf.so`) is needed because
netdata's detection prefers static libraries and RPM distros do not ship a
static libprotobuf, and `CMAKE_BUILD_TYPE` needs no per-distro handling at
all because the top-level CMakeLists defaults an empty or unset build type
to RelWithDebInfo for every configure — the spec's rpmbuild run included.

One hard dependency came out of CPack itself: it only emits `Recommends:`
from CMake 4.1 on, and silently ignores the variables on anything older.
Since dnf installs recommends by default, silently losing them would be a
real behavioral regression, so the RPM `v2` images now carry CMake 4.1.6
under `/cmake` (the entrypoint already preferred that path — the mechanism
the spec builds use for EL 7), and the configure fails fast if the RPM
format is requested on an older CMake. The image changes live on the
`cpack-rpm-cmake` branch of helper-images, together with an `rpm-build`
addition for the openSUSE and Amazon images, which listed only `rpmdevtools`
and could never have run `cpack -G RPM`.

## How we tested it

The proof is a committed harness, `packaging/tests/rpm-parity/`, that builds
both package sets from the same source and version — the reference through
the `v1` image's rpmbuild path, the candidate through the `v2` image's CPack
path — renders every RPM into a normalized text report, and diffs them. The
report covers the package set, the header metadata, every dependency class
including the weak ones, the full per-file table of modes, owners, flags and
capabilities, the scriptlets and the changelog. An allowlist admits exactly
two reviewed deviations, each documented in place: the `.build-id` artifact
paths, which are content hashes and cannot match across two separate builds,
and a single blank line CPack inserts ahead of embedded scriptlet bodies.

Against that harness the branch reaches full parity on a four-distro sample
covering each RPM family we ship: CentOS Stream 9 with twenty-three
packages, Fedora 42 and openSUSE 15.6 with twenty-four each (nfacct exists
on those two), and Amazon Linux 2023 with twenty-two. Getting there took
three iterations on CentOS Stream 9 — which flushed out the build-flag and
directory-ownership classes — after which Fedora and Amazon passed on their
first attempt and openSUSE needed only its linker-flag and doc-path
specifics.

Because two of the shared fixes touch the DEB path, we also rebuilt the DEBs
on debian12 from the branch base and from the branch head and compared them.
All twenty-three common packages are identical in file lists, file sizes and
control metadata; the only delta is the appearance of the
`netdata-ebpf-code-legacy` package, which is precisely the misspelled-gate
fix (the same one-line change as the already-open PR #23188, applied here
identically so the branches merge cleanly). The only content difference is
the build-info cmake-cache archive, which embeds the configure command and
therefore the new option.

Beyond the harness, the branch went through five rounds of independent
multi-model code review against the full cumulative diff, with the findings
either adopted (among them the EL7/AL2 systemd-unit variant selection, a
configure-time CMake version gate, and several robustness fixes to the
harness itself), refuted against build evidence, or recorded as follow-ups.
The last three rounds returned unanimous approvals with no blocking issues.

## What is deliberately not here

The CI flip — pointing `distros.yml` at the `v2` builders, extending the
CMake install to the remaining RPM images, wiring the parity harness into a
workflow, and upgrade-path testing from spec-built to CPack-built RPMs on a
live system — is the follow-up this branch was scoped to stop short of, so
that the switch is a reviewable decision of its own. EL 7 and Amazon Linux 2
are implemented (weak-dep downgrades, python2, the v235 unit) but
unvalidated, since their images lack CMake 4.1 and the parity claims there
should be established empirically, including how their rpm macro sets
actually behave. Three latent DEB-side typos found during review were left
alone on purpose because fixing them changes DEB output — two dead
per-component debuginfo variables and a misspelled `PACHAGE_DEPENDS` that
leaves the pythond DEB without its declared python3 dependency — and they
deserve their own small PRs. Finally, two cosmetic divergences are known and
accepted for now rather than papered over: the RPM descriptions carry the
DEB-style summary line the spec's `%description` bodies lack, and CPack
stamps a Vendor tag where the spec leaves none; whether to align those
exactly is a decision for flip time.

The spec file itself is untouched. Until the flip happens and has survived a
release, it remains the production path — this branch only makes the
replacement real, measurable, and ready.
