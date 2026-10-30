# DEM runtime release descriptors

This offline release tool signs and verifies the complete runtime preparation
contract through `runtime.SignRelease` and `runtime.VerifyRelease`. It performs
no network requests, publication, browser execution or release discovery. Run it
from `src/go`, using a Go version matching `go.mod`.

Use an unencrypted Ed25519 private key in PKCS8 PEM format for signing and its
public key in PKIX PEM format for verification. Supply key files through the
release environment's approved secret handling; never commit private keys.
Verification trust comes from the explicitly supplied public key, not the release
or its archives. The tool neither generates keys nor chooses production trust.

```sh
go run ./tools/dem-runtime-release sign \
  --key /protected/release-private.pem \
  --manifest /build/manifest.json \
  --archive /build/runtime.tar.zst \
  --sources /build/sources.tar.zst \
  --archive-url https://artifacts.example.invalid/dem-runtime/1/arm64/runtime.tar.zst \
  --sources-url https://artifacts.example.invalid/dem-runtime/1/arm64/sources.tar.zst \
  --sequence 17 \
  --output /build/release.json

go run ./tools/dem-runtime-release verify \
  --key /trusted/release-public.pem \
  --release /build/release.json \
  --archive /build/runtime.tar.zst \
  --sources /build/sources.tar.zst
```

The builder manifest is decoded strictly, independently of the signing host's
architecture. Both `linux/amd64` and `linux/arm64` are supported. The archive URL
must match an existing builder URL, or fill its empty offline URL. All other
manifest values, including extraction bounds, layout and ELF binding bytes,
remain unchanged. Signing streams the local runtime archive and compares its
SHA256 and byte count with the manifest. It streams the local source archive and
includes its SHA256 and byte count in the signed release. Inputs must remain
unchanged throughout signing and subsequent publication.

`--sequence` is required, positive and explicitly allocated by the publisher in
one shared channel namespace across architectures and contract changes. The tool
cannot establish sequence uniqueness or source completeness. A new release
identity must not reuse published bytes or a prior sequence for changed content.
Qualification, complete source/license assembly, and checking publication
availability remain release workflow obligations. Signing does not establish any
of them. Package/repository signing is a separate operation.

Output is a complete descriptor created atomically under the requested name.
Existing output paths are always rejected, even if identical; inspect and reuse
an existing verified result instead of overwriting it. Output creation uses a
same-directory temporary file and a hard link, so the destination filesystem must
support hard links. The output inherits the temporary file's private permissions;
the publication workflow can read and upload it without making local key material
public.

Verification authenticates the descriptor before checking optional local files.
Omitting `--archive` or `--sources` skips that file's digest check; it does not
weaken descriptor authentication. A zero exit status indicates success. Errors
are sent to stderr and return nonzero. Keys and artifact bodies are never printed.

```sh
go test -count=1 ./tools/dem-runtime-release
go vet ./tools/dem-runtime-release
```

The tests use synthetic archives and ephemeral fixture keys. They establish CLI
and metadata behavior, not actual browser execution or production publication.

## Stage an optional native package payload

The package recipe can prepare its payload offline from the trusted builder
manifest and matching archive, without a signing key or runtime execution:

```sh
go run ./tools/dem-runtime-release stage-native \
  --manifest /build/manifest.json \
  --archive /build/runtime.tar.zst \
  --output /build/package-root/usr/lib/netdata/dem-runtime
```

The output parent must already exist. The output must be an absolute new or empty
plain directory outside installed native payload/control paths. The command
accepts either supported architecture independently of the build host. It reuses
the runtime's bounded digest verification, extraction and ELF binding code, and
spools the compressed archive beside the output before extracting any bytes.
Allow space for that temporary archive plus the expanded payload.

All bindings, the Chromium launcher and font configuration target the fixed final
prefix `/usr/lib/netdata/dem-runtime`, never the staging path. The output contains
the root payload and generated `native.json`; it contains no managed generations,
selection or lifetime-proof records. Directories and executable files have mode
0755; other regular files have mode 0644. Internal symlinks remain symlinks. The
DEB/RPM recipe MUST normalize all ownership, including symlinks, to root and install
the tree at exactly that final prefix. The installed native reader additionally
checks root ownership and protected ancestors, so a staging directory alone is
not an installed or qualified runtime.

The builder API is
`runtime.PrepareNativePackage(ctx, manifest, archivePath, outputPath) error`.
Only a successful return authorizes packaging the output. Failed staging remains
for inspection and explicit cleanup; no completed `native.json` is emitted after
a digest, extraction, binding or permission failure. Existing nonempty outputs
are refused. Build workspaces must provide exclusive access to each staging
output. A package lifecycle hook must still close admission, stop/drain consumers,
and qualify the installed payload before reopening admission. This command does
not perform those operations or replace package/repository authentication.
