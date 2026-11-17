# DEM companions of Agent releases

Runtime releases use the Agent's canonical version, channel and full source commit.
`packaging/release-info` supplies that identity to both architecture builds. The
runtime remains optional; a normal Agent image or installation does not include
browser bytes. Actual signed public publication and public acceptance are pending.

This runbook owns portable artifact assembly and publication. The normal Agent
CPack, signing and upload jobs own optional native packages. There is no separate
runtime version, publication sequence, native uploader or all-channel fan-out.

Exact release identity governs publication and installation/update transactions. Execution separately checks the
runtime contract, architecture and integrity/provenance. A compatible installed runtime can serve a plugin with
different or empty release stamps; this does not change publication pairing or create an independent update stream.

## Build and publication order

1. Select one immutable Agent checkout and generate its `packaging/release-info`
   with `packaging/release_info.py`. Use the same version, channel and source for
   Agent builds and both runtime architectures. Keep component and donor pins in
   `inputs.json` and `locks/`; changed content requires a new Agent release identity.
   The central release workflow freezes the commit after its version/changelog
   update. Development builds use `0.<full-source-SHA>`; preview is a stable
   destination, not another runtime channel.
2. Native `amd64` and `arm64` workers build portable archives and complete matching
   source archives. Runners follow `.github/data/distros.yml`. The donor receives
   the plain identity file and shared helper sources through read-only mounts.
   Archives retain sources/licenses, SBOM, provenance and deterministic bounds.
3. `release.py assemble` verifies the built identity and corresponding-source digest,
   then calls the existing Go `stage-native` command to create a prebound payload.
   It packages that tree as `native-payload.tar.zst` for normal Agent packaging;
   it does not run CPack, sign native packages or submit repositories.
4. CI must run actual sandboxed Playwright and Lighthouse workloads on the exact
   candidates, through the Agent and `nd-run`, on both architectures before
   publication. Record artifact identities, service UID/GID, host policy and
   process drainage. Assembly and Node wire tests are not browser smoke evidence.
   Installation/transaction tests remain separate acceptance evidence. Explicit
   local `runtime qualify` is optional and reports its calling environment.
5. The publisher uses the existing package OpenPGP signer and fetches
   public trust from `https://repository.netdata.cloud/netdatabot.gpg.key`.
   `release.py stage` checks both architecture inventories before signing format-2
   descriptors containing the exact Agent tuple, runtime digest/bounds and sources.
6. `publish` authenticates any existing immutable descriptors, uploads all runtime
   and source objects before either descriptor, and reads every uploaded object
   through the public HTTPS route to verify exact SHA256 and size. Retry reuses
   authenticated immutable descriptor bytes when their complete payload matches.
7. Companion publication must complete before the corresponding Agent exposure. Native
   packages use the same source and parent-run payload artifacts through the normal
   Agent pipeline and its repository routing. Repository ingestion is asynchronous.

`dem-runtime-release.yml` is a reusable prerequisite of the central release and
manual Agent lanes. It uses the normal Agent release access model: shared secrets
inherited from its callers and repository or organization configuration variables,
without a separate GitHub deployment environment.
It emits `dem-runtime-publication-receipt` only after successful
publication. The shared companion check verifies the publishing job,
both architectures and exact identity. Ordinary source/static/Docker lanes consume
this small receipt; native jobs additionally consume and verify the actual payload.
Only same-repository manual release invocations or the scheduled central release
can supply this receipt; pull-request build artifacts are excluded.
Source/static/Windows and base-image compilation can run before companion
publication. Their GitHub artifacts are internal build transport; the public
artifact preparation/release and registry push jobs require the successful
companion receipt. Native packaging consumes companion bytes and remains gated
before its build. Documentation, metadata and unit-test-only edits run focused
contract checks without assembling both browser archives. Every actual companion
build still runs the real browser smoke and focused ARM checks.
Docker passes the canonical tuple as build arguments and reconstructs its metadata
before compilation, preserving the generated file's Git exclusion.

Publication is serialized and must not cancel an in-flight writer.
These helpers do not provide a global transaction across R2, registries or package
repositories. A partial immutable upload can already exist
when a job fails; retry the same authenticated inputs to finish it. Never replace
immutable bytes or bypass signature checks to recover.

## Exact CI artifacts and native preparation

Each parent-run artifact `dem-runtime-<amd64|arm64>` contains:

- `<sha256>.tar.zst` and `<sha256>.sources.tar.zst`;
- `manifest.json`, `source-archive.json`, `provenance.json`, `sbom.cdx.json` and
  `sources-required.json`;
- the exact plain `release-info` and `native-payload.tar.zst`;
- `artifacts.json` format 2, with architecture, Agent tuple and every file's digest
  and byte count.

Native jobs consume the artifact from the same release run, before entering their
older distro container:

```sh
python3 packaging/dem-runtime/ci.py prepare-native \
  --artifacts /build/dem-runtime-amd64 --arch amd64 \
  --receipt /build/dem-runtime-publication-receipt \
  --release-info packaging/release-info --output /build/native-payload
```

The helper checks the complete inventory, its exact match with the successful
publication receipt, and the expected Agent tuple/architecture
before extraction, requires a new or empty output, uses standard filtered tar
extraction, and checks the unpacked `native.json` and exact `release-info` bytes.
The normal Agent build receives that completed tree through
`DEM_RUNTIME_PAYLOAD_DIR`. The CPack component checks it against its Agent source
identity and owns final package versions, revisions, epochs and dependencies.
No native transport archive is published as an end-user R2 artifact.
If a later workflow attempt rebuilds different bytes, the earlier receipt cannot
authorize them. Retry with the published artifacts or use a new Agent identity.

## Public objects

The public base is `https://artifacts.netdata.cloud`. SRE owns the authenticated
storage route and signing/access configuration used by the release workflow;
internal infrastructure details do not belong in this runbook.

```text
dem-runtime/<stable|nightly|devel>/<Agent-version>/<amd64|arm64>/<sha256>.tar.zst
dem-runtime/<stable|nightly|devel>/<Agent-version>/<amd64|arm64>/<sha256>.sources.tar.zst
dem-runtime/<stable|nightly|devel>/<Agent-version>/<amd64|arm64>/release.json
dem-runtime/<stable|nightly|devel>/<Agent-version>/<amd64|arm64>/{manifest,provenance,sbom.cdx,...}.json
```

Signed descriptors reference versioned bytes. Runtime acquisition always selects
its exact Agent target. Runtime publication has no mutable latest aliases or
version-order promotion. Normal Agent latest channels and release cadence remain
owned by the Agent release lanes.

Immutable upload uses `rclone copyto --immutable --checksum`, never synchronization
or deletion. Public readback checks exact serving bytes.
Sources remain beside their exact runtime identity. Storage retention is
infrastructure policy; these helpers implement no publisher garbage collection.

## Trust and retries

Portable clients authenticate signed descriptors. Native package/repository
signatures are the native installation trust boundary. Neither route distributes
private keys. Offline users explicitly provision the public keyring; online
clients fetch canonical public trust for each preparation attempt.

OpenPGP signatures can differ across retries because of signing timestamps. The
publisher verifies the immutable stored payload, reuses its original signed bytes,
and refuses changed or unverifiable content. Plan key rotation across Agent
releases with valid overlap: an existing immutable release keeps its original
signature. Missing, revoked or expired signers block authenticated retry. Emergency
trust recovery requires the release owner's explicit procedure.
Signature validity does not detect withheld releases, certify every host policy,
or establish fresh key state through a stale cache.

## Local validation and limits

The installation-owner upgrade matrix runs on amd64. Native ARM CI covers build/package checks, focused pair and
recovery lifecycle tests, and genuine signed-runtime installation with sandboxed browser smoke tests. The shared
upgrade matrix is not duplicated under ARM emulation.

PR checks follow the changed inputs. `dem-runtime-tests.yml` selects workflow behavior, Python packaging/native
fixtures, Go runtime integration, installer ownership and Node wire checks separately. The installer identity
container runs only for its helper/caller tests. Prose changes skip execution tests; metadata, alert definitions and
Function schemas consumed by Go contract tests remain inputs. Test-only changes run their suite without rebuilding
Agent packages, images or browser bundles. Go/security checks retain their own repository-wide selection rules.

Agent builds follow Agent build inputs. Release orchestration changes run workflow checks; changes to the runtime
builder workflow still build and qualify both browser architectures. Scheduled and manual releases always run the
complete companion build, wire checks, signing and public verification before Agent publication, regardless of the
PR filters. The reusable Node workflow defaults to running its tests for release callers.

```sh
cd src/go
go build -trimpath -o /tmp/dem-runtime-release ./tools/dem-runtime-release
cd ../..
DEM_RELEASE_TEST_TOOL=/tmp/dem-runtime-release \
  python3 -m unittest discover -s packaging/dem-runtime -v
python3 -m unittest discover -s packaging/dem-runtime/native/tests -v
```

`assemble` and `stage` operate on exclusive local workspaces; staging needs an
explicit signing key but performs no publication. `publish` mutates
remote state and is not an ordinary test command. Source assembly and transport
archives use zstd level 19, one worker and an 8 MiB window.

Tests cover identity consistency, signed staging/tampering, immutable retry,
stable/nightly/devel paths, partial publication, public digest checks,
native transport validation and absence of an independent native uploader. GPG
integration cases require the real tool and explicitly report missing prerequisites.
Tests do not establish actual R2/CDN behavior, key custody, repository ingestion,
source-license sufficiency, native package transactions or every host's confinement.
