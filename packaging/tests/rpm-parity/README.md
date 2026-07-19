# RPM packaging parity check

Verifies that RPMs produced by the CPack path (`packaging/build-package.sh
RPM`, used by the `v2` package-builder images) match the RPMs produced from
`netdata.spec.in` via rpmbuild (the `v1` images) package for package: package
set, header metadata, dependencies (including weak dependencies), per-file
modes/ownership/flags/capabilities, scriptlets, and changelog.

## Usage

Build both sets from the same source tree and version, each inside its
distro's package-builder container:

```sh
# reference (spec) build
docker run --rm --security-opt seccomp=unconfined -e DISABLE_TELEMETRY=1 \
    -e VERSION="$(tr -d 'v' < packaging/version)" -v "$PWD":/netdata \
    netdata/package-builders:<distro>-v1
mv artifacts ref-rpms

# candidate (CPack) build
docker run --rm --security-opt seccomp=unconfined -e DISABLE_TELEMETRY=1 \
    -e VERSION="$(tr -d 'v' < packaging/version)" -v "$PWD":/netdata \
    netdata/package-builders:<distro>-v2
mv artifacts cpack-rpms

packaging/tests/rpm-parity/compare-rpms.sh ref-rpms cpack-rpms \
    packaging/tests/rpm-parity/allowlist
```

The comparison itself only needs the `rpm` binary on the host.

## Limitations

The comparison covers RPM metadata and file attributes, not payload bytes:
two builds that package the same paths with the same modes but differently
compiled binaries (for example after a compiler-flag drift that changes
optimization but not the linked sonames) compare equal. Dependency
generation catches the common cases because soname and versioned-symbol
requirements are part of the compared metadata.

## Allowlist

`allowlist` holds extended regexes for reviewed, intentionally accepted
deviations; matching diff lines are ignored. Keep it minimal and keep the
reason for every entry as a comment above it.
