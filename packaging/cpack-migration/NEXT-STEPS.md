# From this branch to production: the switch-over process

This document describes the path from the current state — a parity-proven
CPack RPM path sitting unused next to the production spec path — to CPack
being the only way Netdata RPMs are built. It complements `README.md` in
this directory, which explains what the branch does and how it was
verified; this file explains how to land it and flip production over.

Like everything in this directory, this is hand-off material for the
CI/packaging maintainer and is expected to be deleted once the migration
completes.

## The one fact that shapes the whole process

Merging the netdata branch changes nothing in production. CI selects the
builder image revision per distro through `.github/data/distros.yml`, and
every RPM distro references the shared anchor at the top of the file:

```yaml
default_builder_rev: &def_builder_rev v1
```

While that anchor says `v1`, packages are built by the spec file inside the
`v1` images and the entire CPack RPM configuration on this branch is dead
code in CI. The switch is the one-line change of that anchor to `v2` —
nothing else. (DEB distros hardcode `builder_rev: v2` per entry and are
unaffected.) This is why the merge of the branch and the switch to the new
approach are two separate, independently reviewable decisions.

## Step 1: land the helper-images changes

The `cpack-rpm-cmake` branch of helper-images puts CMake 4.1.6 into all
eighteen RPM `v2` builder images (CPack emits `Recommends:` only from 4.1
on), adds `rpm-build` where it was missing, and removes libmongoc from the
EL10 images. Review and merge this first: the flip depends on the published
images, and images are published by the daily image build. After the merge,
wait for the next daily build and spot-check a couple of the published
`netdata/package-builders:<distro><version>-v2` tags for
`/cmake/bin/cmake --version` reporting 4.1.6 and `rpmbuild` being present.

## Step 2: land the netdata branch

Review and merge the netdata branch. Before merging, delete this directory
(`packaging/cpack-migration/`) — it exists only for this hand-off, and its
durable content already lives in code comments and in
`packaging/tests/rpm-parity/README.md`. Everything else on the branch
stays, including the parity harness under `packaging/tests/rpm-parity/`.

Merging is safe at any point after step 1 (strictly, even before it):
production keeps building RPMs from `netdata.spec.in` through the `v1`
images until step 3.

## Step 3: the flip PR

A separate PR changing `default_builder_rev` from `v1` to `v2` in
`.github/data/distros.yml`. One line flips every RPM distro at once,
including CentOS 7 and Amazon Linux 2 — both are parity-proven, so there is
no need to hold them back. If a staged rollout is preferred anyway,
individual distros can pin `builder_rev` per entry instead of referencing
the anchor.

Validate on the PR itself before merging — CI provides a full dry run for
free:

- Add the `run-ci/packaging` label. `packaging.yml` then builds the full
  matrix (every distro and architecture) with the `v2` images and runs
  `.github/scripts/pkg-test.sh` against each result — an actual
  install-and-run test inside the stock distro image. Publishing is gated
  off for pull requests, so nothing ships.
- Note that the parity work was done on x86_64; this matrix run is the
  first functional coverage of the CPack packages on aarch64.
- Run an upgrade-path test manually: install a spec-built nightly RPM in a
  container, then upgrade to the CPack-built package of a newer version.
  The scriptlet interleaving across old and new packages during an upgrade
  transaction is the one behavior the parity harness cannot prove, because
  it compares packages, not transactions.
- Decide the two known cosmetic divergences documented in `README.md`:
  the summary line CPack prepends to `%description` bodies, and the Vendor
  tag CPack stamps where the spec leaves none. Either align them exactly
  or accept them; accepting is fine, deciding by default is not.

Optionally, this is also the moment to wire the parity harness into a CI
workflow. Its value is confined to the window in which both paths exist —
after the spec retires (step 6) it has no reference to compare against —
so weigh the wiring cost against how long that window is expected to stay
open.

## Step 4: the nightly window

After the flip merges, nightly builds publish CPack-built RPMs to the
repositories, and nightly users' routine upgrades exercise the
spec-RPM-to-CPack-RPM transition at scale. Watch this window: package
install failures surface in the usual support channels, and agent-side
breakage (a service that did not restart, a user that was not created)
surfaces in agent-events. Any regression here is trivially revertible —
flip the anchor back to `v1` and the next nightly builds from the spec
again.

## Step 5: the first stable release

The first stable release after a healthy nightly window ships CPack-built
RPMs to the stable repositories. The spec path stays in the tree, unused
but functional, as the fallback.

## Step 6: retirement

After the CPack packages have survived one stable release, remove the old
path:

- `netdata.spec.in` in this repository;
- `fedora-build.sh`, `suse-build.sh`, and the `v1` Dockerfiles in
  helper-images (the orphaned fedora41 Dockerfiles and the unbuildable
  `v1` EL10 Dockerfiles documented in `README.md` can ride along);
- the parity harness either retires with it or is repurposed as a
  package-inspection tool — at that point it is a historical artifact, not
  a guard.

Until step 6 completes, the spec remains the documented fallback and MUST
be kept buildable: any change to the package payload made while both paths
exist has to be applied to both, which is precisely the duplication this
migration exists to end — so the window between steps 3 and 6 should be
kept short.
