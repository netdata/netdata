# Independent DEM runtime releases

`.github/workflows/dem-runtime-release.yml` builds both architectures from one
reviewed checkout, independently of Agent releases. Dispatch defaults to building
artifacts only. Publication is a separate, manually requested protected-environment
job on `netdata/netdata` master. It is not ready for production until the release
environment, target qualification and publication infrastructure below are confirmed.

## Release sequence

1. Update the runtime-owned component/lock inputs and `inputs.json` version. Use a
   new version for changed content. Review the complete corresponding-source and
   license inventory. Do not reuse an identity or replace an old published object.
2. Dispatch a build. Native `amd64` and `arm64` workers assemble the portable
   archive, verified source archive and DEB/RPM payloads. The source assembly runs
   from the same checkout used to compile the staging tool and records its local
   Go dependency closure plus module pins. No browser installation occurs on the
   runner host; packaging tools are installed on the disposable worker.
3. Qualify the exact runtime artifacts on supported targets with real sandboxed
   journeys and Lighthouse, including service/package upgrade and removal.
   Successful archive/package assembly is not execution or confinement evidence.
   Verify native AppArmor profile loading, reboot persistence and safe removal
   with host restrictions unchanged. Managed/static/preload deployments on
   restricted Ubuntu require administrator-prepared policy; the native profile
   does not cover their service-owned executable paths. Record actual SELinux
   process labels: an enforcing host running `unconfined_service_t` does not
   establish support for a custom confined domain.
4. Request publication with a positive channel sequence. One sequence is shared
   across architectures and semantic-contract changes, independently of Agent
   versions. The protected environment's reviewers verify the candidate artifacts,
   qualification evidence, source/license completeness and destination/key setup.
5. Sign complete release descriptors with Ed25519. Upload immutable archives,
   corresponding sources, descriptors and inventories to R2. Read each through
   the public HTTPS route and check its full SHA256/size before promotion.
6. Sign the native packages and submit them through the existing package upload
   pipeline. Finally promote the signed `latest` descriptor for each architecture.

The workflow runs one publisher at a time and does not cancel an in-flight release.
The publisher reads current signed pointers from the storage API, avoiding a stale
CDN when checking sequence order. It rejects sequence regression and reuse for
different content. Immutable uploads use `rclone copyto --immutable --checksum`,
never synchronization or deletion. Published objects and matching sources must be
retained while any installation can reference them.

Promotion is atomic per architecture, not across architectures or native repositories.
If a step fails, an earlier immutable upload or one architecture's promotion may
already have succeeded. Rerun the failed publication job with the same artifacts,
sequence and keys; identical pointer retries are allowed. A changed rebuild must
not overwrite that release. Repository ingestion is asynchronous: successful SSH
submission does not prove repository availability. Packaging/SRE must verify the
signed repository metadata and installation from it before announcing availability.

The workflow stages native packages separately from the portable R2 objects.
Native package/repository signatures are their installation trust boundary;
managed clients authenticate the signed descriptor. Neither route distributes
private keys. The protected environment must restrict branches and require human
review; merely naming an environment in YAML does not configure those protections.

## Infrastructure configuration

Configure these in the protected `dem-runtime-release` environment after review:

| Setting | Required value or decision |
|---|---|
| `DEM_RUNTIME_PUBLIC_BASE_URL` variable | Confirmed HTTPS public base, without `/dem-runtime`; proposed existing artifact routing is not assumed valid |
| `DEM_RUNTIME_R2_REMOTE` variable | Named rclone bucket path; proposed `s3:netdata-agent-artifacts` |
| `DEM_RUNTIME_RELEASE_PUBLIC_KEY` variable | Dedicated Ed25519 PKIX public PEM; also provision this trust through Agent/admin configuration |
| `DEM_RUNTIME_RELEASE_PRIVATE_KEY` secret | Matching unencrypted PKCS8 private PEM, under approved release-key custody |
| `DEM_RUNTIME_PACKAGE_HOST` variable | Confirmed existing package ingestion SSH host |
| `DEM_RUNTIME_PACKAGE_REPOSITORY` variable | Confirmed existing repository namespace, e.g. `netdata/netdata` |
| Existing R2 secrets | `CLOUDFLARE_R2_ACCESS_KEY`, `CLOUDFLARE_R2_SECRET_KEY`, `CLOUDFLARE_R2_RELEASE_ENDPOINT` |
| Existing package secrets | `NETDATABOT_PACKAGE_SIGNING_KEY`, `NETDATABOT_PACKAGES_SSH_KEY`, `PACKAGES_KNOWN_HOSTS` |

The draft repository targets are `ubuntu/noble` and `el/9`, for both architectures.
Confirm packaging conventions, package signatures and repository ingestion before
publication. The Agent/plugin repository must already provide
`netdata-dem-runtime-api = 1`. A runtime version does not need to match an Agent
version. All native installations currently require a live systemd host; native
chroot/image provisioning is explicitly deferred.

Expected object layout:

```text
dem-runtime/<version>/<amd64|arm64>/<sha256>.tar.gz
dem-runtime/<version>/<amd64|arm64>/<sha256>.sources.tar.gz
dem-runtime/<version>/<amd64|arm64>/release.json
dem-runtime/<version>/<amd64|arm64>/{manifest,provenance,sbom.cdx,...}.json
dem-runtime/latest/<amd64|arm64>.json
```

The public route must return the exact bytes, preserve immutable paths and avoid
stale caching of mutable latest pointers. Keep every published identity and its
sources; no garbage collection of publisher objects is implemented. A changed
public verification key needs an Agent/admin trust update. The signed client
protocol still does not detect withheld releases or establish freshness for a
new installation; see the runtime protocol documentation for those approved limits.

## Local validation and limits

`release.py assemble` builds native packages from verified builder/source outputs;
`stage` signs both platform descriptors without network access; `publish` uploads
immutable objects; `promote` rechecks public runtime/source availability before
changing pointers. All publication workspaces must be exclusive and their inputs
immutable during the operation. Normal builds/tests never call the last two commands.

```sh
cd src/go
go build -trimpath -o /tmp/dem-runtime-release ./tools/dem-runtime-release
cd ../..
DEM_RELEASE_TEST_TOOL=/tmp/dem-runtime-release \
  python3 -m unittest discover -s packaging/dem-runtime -v
python3 -m unittest discover -s packaging/dem-runtime/native/tests -v
```

Publisher tests verify signed staging with the real Go tool, tamper rejection,
sequence checks, bounded public-object checks and promotion ordering. Native
artifact tests exercise CPack and payload preservation where the required Linux
tools are present. These tests do not establish live R2/CDN behavior, key custody,
repository ingestion, source-license sufficiency or native host qualification.
