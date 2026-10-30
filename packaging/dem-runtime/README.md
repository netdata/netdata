# DEM portable runtime bundle

This builder produces one private Linux runtime for `amd64` or `arm64`: Node,
locked production npm dependencies, Debian's unbranded Chromium, the private ELF
loader/library closure, fonts, notices, and the complete release-owned execution
adapters from `src/dem-runtime`. `nd-run` remains an Agent component. DEM disables
video recording, so no Playwright FFmpeg artifact is shipped. Exact component pins
are runtime-owned; the Agent validates semantic execution contract 1 and immutable
release identity. The Agent integration remains experimental while delivery is
qualified. The builder itself does not publish. The separate [release workflow](RELEASE.md)
assembles optional native packages and defines protected signing/publication steps.

## Build from reviewed inputs

From the repository root, with Python 3.9 or newer and Docker with BuildKit:

```sh
python3 packaging/dem-runtime/build.py --arch arm64 \
  --output .local/audits/dem-runtime-delivery/build-arm64 \
  --cache .local/audits/dem-runtime-delivery/binary-inputs
python3 -m unittest discover -s packaging/dem-runtime -v
```

The output directory MUST be empty. No packages are installed on the host. An
architecture-compatible Docker worker or emulation is required; an emulated
build does not establish native execution acceptance.

`inputs.json` pins the independent runtime version, deterministic timestamp,
Debian base image and architecture manifests, Node and patchelf binary hashes,
and their source hashes. Generated `locks/<architecture>.json` files record the
immutable base inventory, complete resulting Debian inventory, exact additional
binary archive URLs/sizes/SHA256s, and source records. The base image owns unchanged
packages; every changed or added package comes from a locked binary archive.

Ordinary builds MUST NOT resolve current apt candidates. Debian files use official
Snapshot content-address URLs, so mirror rotation does not change the selected
bytes. Their SHA1 URL identifiers are locators only: the host still verifies the
signed-index SHA256 and size, using the lock's explicit filename. Builds do not
query Snapshot metadata or fall back to current packages. Docker's bootstrap transaction
uses only those local files with empty apt sources, no remote package indexes,
and BuildKit networking disabled. Local file acquisition remains enabled because
apt must copy supplied archives into its transaction cache. The container verifies the entire installed
inventory against the lock before assembling the runtime. A missing old archive
fails acquisition; it never silently selects a newer package. Preserve verified
binary caches or establish an approved immutable mirror for long-term rebuilding.

Node comes from its checksum-verified official release. Binary and source acquisition
share HTTPS-only redirect handling, a socket timeout, hash verification, and any
byte bounds recorded in the input lock. The reviewed donor inventory retains curl
until the next intentional lock refresh; ordinary builds do not invoke it. `npm ci` uses the runtime's
repository-owned `package-lock.json`, with lifecycle scripts, auditing and funding
requests disabled. The builder verifies exact Node and npm versions, Debian
source/package identity, and exact browser output against
`src/dem-runtime/manifest.json`. Playwright's internal
browser revision is not the runtime delivery identity. These installed inputs
perform no apt, npm, or browser downloads when the Agent prepares a runtime.

The output contains:

- `<sha256>.tar.gz`: sorted entries with normalized ownership/mode/time and fixed
  gzip timestamp, excluding source trees;
- `manifest.json`: format 1 identity, execution contract 1, exact compressed/tar/regular-file byte bounds,
  entry count, layout including `entrypoint: runner/run.mjs`, and authenticated ELF bindings;
- `provenance.json`: complete runtime source/test and recipe hashes, exact input locks, original copied
  library hashes, and exact redistributed Debian source records;
- `sbom.cdx.json`: CycloneDX 1.5 inventory of actual Debian browser/library/font
  packages with source versions, Node, and locked npm packages;
- `sources-required.json`: the source records needed for the copied Debian
  components, excluding unrelated build-only tools.

The runtime archive includes `runner/`, the runtime-owned component `manifest.json`,
and `runtime.json` stamped with the release version. Runtime metadata resolves
dependencies under `dependencies/` and the prepared `chrome` wrapper at the root.
The outer descriptor's digest is never embedded in the payload. It also includes
the SBOM/provenance, regular-file hashes (`files.json`),
Node's license, npm's original notices, Chromium's embedded third-party credits,
Debian package copyright files, and full referenced `/usr/share/common-licenses`
texts. Fontconfig builds its user cache; generated donor X11 indexes are omitted.
Chromium credits are exported in the root build container with Playwright's
build-time sandbox disabled. This reads embedded notices and MUST NOT count as
sandbox qualification. Runtime launchers never disable the browser sandbox.

Without `--url-prefix`, `archive.url` remains empty in the builder descriptor. An explicit
approved HTTPS base constructs
`<prefix>/dem-runtime/<version>/<architecture>/<digest>.tar.gz`. The builder does
not publish anything. The release signing step binds the reviewed manifest to its
archive and source URLs; an unsigned manifest is not an administrator CLI input.

## Local installation and qualification

Use the DEM CLI's explicit runtime install/update commands with a signed release
descriptor and its exact archive. The outer builder `manifest.json` and the
source-level component manifest inside the runtime are different documents.
`src/go/tools/dem-runtime-release` signs the complete descriptor, including the
publication sequence, archive identity/bounds and source identity. Configure trusted
public keys locally; never derive trust from a key fetched beside an archive.
Offline imports still verify a signature and do not fetch the signed HTTPS URLs.
See `src/go/plugin/dem/README.md` for CLI options, automatic/manual update consent
and restart activation. Enabling a monitor does not acquire runtime components.

Candidate qualification must run both a journey and a Lighthouse audit against
deterministic local content through the Agent and supervised `nd-run`. Runtime
`--check` verifies component versions/imports without a browser session, so it does
not establish browser execution readiness. Local build/import evidence does not
establish production hosting, native packaging or source delivery.

## Refresh Debian locks

Refresh is a deliberate maintainer action that uses mutable, signed Debian
indexes. It is separate from ordinary builds and does not change tracked locks
automatically. First update the exact qualified browser contract and, if needed,
the immutable base/tool pins. Then generate candidates for both architectures:

```sh
python3 packaging/dem-runtime/refresh-lock.py --arch arm64 \
  --output .local/audits/dem-runtime-delivery/lock-arm64
python3 packaging/dem-runtime/refresh-lock.py --arch amd64 \
  --output .local/audits/dem-runtime-delivery/lock-amd64
```

The disposable base container authenticates apt indexes using its pinned archive
keyring, upgrades the base, installs exact Chromium and the stated tools/fonts,
and records binary/source metadata and authenticated InRelease hashes. Review
both generated JSON files, particularly browser version, package changes, source
coverage and architecture differences, before copying them into `locks/`. A binary
rebuild suffix such as `+b2` is preserved on the binary version; Debian's
`${source:Version}` identifies the corresponding source without that suffix.

A refresh MAY record unavailable source metadata for unused base/tools as a
warning. Runtime assembly MUST fail if any actually redistributed Debian component
lacks its exact source record. Updating lock inputs requires a new runtime version,
clean builds, a byte-for-byte repeat, and qualification of the exact browser/
Playwright pair. Verify Node and patchelf releases against their official release
checksums when changing those separately maintained pins.

Before writing a candidate lock, refresh queries the official Snapshot API once
per source version. It matches binary version/architecture and source filenames,
then downloads each distinct candidate to verify the original SHA256 and size.
Snapshot metadata supplies SHA1 locators and sizes, not the SHA256 trust anchor.
Ambiguous names/sizes are resolved only by verified bytes. Missing or mismatched
files fail generation; newly uploaded packages may require waiting for Snapshot
ingestion. A cold refresh downloads the locked source files as well as binaries;
temporary verification files are removed after each file. Snapshot retention
avoids normal mirror rotation but does not guarantee permanent availability.

To repair locations in existing locks without apt resolution or component updates:

```sh
python3 packaging/dem-runtime/refresh-lock.py \
  --snapshot-lock packaging/dem-runtime/locks/amd64.json packaging/dem-runtime/locks/arm64.json \
  --output /absolute/new/snapshot-locks
```

The output must be new or empty. All input locks remain untouched, and no converted
lock is written until every mapping succeeds. Review the URL-only diff before
adopting the generated locks; all versions, inventories, hashes and sizes remain
unchanged. Ordinary builds still require no metadata lookup or warm cache.

## Assemble release sources

Keep sources outside the end-user runtime. With the Go toolchain specified by
`src/go/go.mod`, use the exact recipe/runtime checkout that builds the release tool
to assemble a separate release source directory:

```sh
python3 packaging/dem-runtime/sources.py \
  --runtime-output .local/audits/dem-runtime-delivery/build-arm64 \
  --output .local/audits/dem-runtime-delivery/release-sources-arm64 \
  --archive-output .local/audits/dem-runtime-delivery/source-archive-arm64
```

This fetches and hash-verifies every selected Debian `.dsc` and ALL its companions,
including Chromium's upstream, pre-generated and Debian packaging archives. The
original signed descriptor is preserved; its identity and full companion hash/size
set must match the reviewed lock. The lock derives from authenticated apt Sources
metadata, rather than trusting a descriptor downloaded beside an archive. This
route does not independently verify the descriptor's OpenPGP signature.

The assembly also verifies Node sources, patchelf sources, and exact npm published
source archives using their integrity pins. It retains the complete packaging/ELF
modification recipe and tests, native package/publisher recipes, locks, all runtime adapters/tests,
the staging tool's local Go build closure and module pins, runtime identity/provenance/SBOM, and a
`source-assembly.json` file inventory. Existing downloads are reverified on rerun. The output must stay outside the recipe
directory; unexpected existing files are rejected and preserved, not included in
the verified inventory.
The optional archive output must be a separate, new or empty directory. Before
archiving, the command verifies every declared file's digest/size, the complete
file set and the runtime identity again. It emits a deterministic
`<sha256>.sources.tar.gz` and `source-archive.json` with its digest and byte count.
Supply this archive to the release signer; a source URL alone is not source
delivery. Keep the assembly unchanged throughout archiving and signing.
Debian source sets can be unpacked with `dpkg-source -x <descriptor>` using their
preserved filenames. Actual source availability and applicable redistribution
obligations still require packaging/legal review; names, links and notices alone
are not completed source delivery. Release owners must publish the required
source material through the approved source-delivery channel.

## ELF preparation and qualification

patchelf 0.15.0 is checksum-pinned. Versions 0.18 and 0.19.1 corrupted the tested
arm64 Node PHDR/LOAD layout; changing tools requires both-architecture validation.
Dynamic objects receive `$ORIGIN`-relative private library search paths. Loader
and libc retain their donor ABI layout; libc is not an execution entry point.
Runtime-loaded NSS modules supplement the transitive library closure. Missing
libraries or conflicting private-library basenames fail construction.

Executable PT_INTERP slots are exactly 4096 bytes: Linux's maximum 4095-byte
interpreter pathname plus NUL. ELF64 headers determine the actual offset; the
builder checks architecture, segment bounds, PHDR coverage and exact original
slot bytes. Real Node, Node re-execution and Chromium probes exercise spaces/quotes
and the maximum interpreter path using separate, never-repatched executable
copies. These checks establish executable integrity, not browser sandbox support.
The runtime store additionally bounds every full archive path; a long descendant
can impose a shorter usable installation prefix than the interpreter alone.

The Agent verifies digest/bounds and fills only declared slots with the FINAL
immutable generation path. There is no host-side ELF rearrangement. `chrome`,
`fonts.conf`, and `prepared.json` are reserved generated files and MUST NOT occur
in the archive. Only internal, resolvable symlinks and ordinary files/directories
are permitted; privileged modes are removed.

Before publication, qualify the archive through the actual Store, Engine and
supervised `nd-run`, with sandbox enabled, on native amd64 and arm64. Verify the
kernel floor, native Ubuntu AppArmor/systemd, all installation methods, offline/
read-only preloads, and browser/Node re-execution. Container evidence applies only
to its tested kernel/userland. Native qualification, source/license delivery,
security rebuild ownership and SRE-confirmed public hosting remain publication
gates. Immutable archives MUST NOT be overwritten during dependency rebuilds.

The native package supplies an AppArmor namespace allowance attached only to its
root-owned Chromium executable. Qualification must pass with the host's normal
restrictions and browser sandbox enabled, including after reboot. This named
unconfined profile permits sandbox setup; it is not extra browser confinement.
Managed/static bundles and preloads have a different ownership boundary: their
executable files are service-owned. On restricted Ubuntu hosts, administrator
policy preparation is an explicit prerequisite; the installer MUST NOT grant a
generic exception to those writable paths. Container policy is also a separate
host/deployment prerequisite. See the [operator guidance](../../src/go/plugin/dem/README.md#runtime-setup-and-retirement).
