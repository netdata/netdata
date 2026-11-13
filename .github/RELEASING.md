# Agent releases

Run the **Release** workflow on `master`. It validates the requested version, updates the changelog and
`packaging/version`, pushes the tag and branches, then dispatches artifact, package and Docker builds.

| Build type | Version example | Source branch | Publication |
|---|---|---|---|
| `nightly` | `nightly` | `master` | Nightly artifacts; edge packages; `netdata/netdata-nightlies` GitHub release |
| `release-candidate` | `v2.13.0-rc1` | `master` for the first RC; `v2.13` thereafter | Preview artifacts/packages; GitHub prerelease draft |
| `minor` | `v2.13.0` | Existing `v2.13` | Stable and eligible preview destinations; GitHub release draft |
| `major` | `v3.0.0` | Existing `v3.0` | Stable and eligible preview destinations; GitHub release draft |
| `patch` | `v2.13.1` | Existing `v2.13` | Stable and eligible preview destinations; GitHub release draft |

## Running a release

1. Choose the build type and version from the table. For a stable release, ensure the intended changes are on
   the existing `vMAJOR.MINOR` branch; preparation does not merge changes from `master` into that branch.
2. Dispatch the [Release workflow](workflows/release.yml) on `master` with those `type` and `version` inputs.
   Supply the leading `v` for numbered releases; use `nightly` for both inputs for a manual nightly.
3. Monitor preparation and the dispatched [artifact](workflows/build.yml), [native package](workflows/packaging.yml)
   and [Docker](workflows/docker.yml) workflows. Major, minor and patch inputs become `release` for downstream
   artifact/package builds; release candidates remain `release-candidate`.
4. For stable releases and RCs, review and manually publish the GitHub release draft in `netdata/netdata`.
   Artifact, native package and Docker publication runs independently of this manual step; the draft is not a
   publication gate for those destinations.

Release workflow runs are serialized across dispatch refs. Each preparation pushes its branch and tag updates
atomically, so a failed push cannot publish only part of that update.

## Nightly releases

- The Release workflow is scheduled daily at 00:00 UTC. A scheduled run skips publication when `HEAD` is already
  the last nightly version-update commit. A manual `nightly` dispatch does not use that skip condition.
- Preparation runs from `master`, generates the changelog and writes a version derived from the nearest tag and
  the number of commits since it, in the form `<tag>-<count-plus-one>-nightly`. When the nearest tag is a release
  candidate (first RCs are tagged on `master`), its `-rcX` suffix is dropped, so `v2.13.0-rc1` yields
  `v2.13.0-<count-plus-one>-nightly`. The dispatch version remains `nightly`; downstream builds read the generated
  version from `packaging/version`.
- Source, static installer and Windows artifacts go under the `nightly` artifact prefix, including `latest`.
  GitHub releases are published automatically in `netdata/netdata-nightlies`, rather than drafted in the main repository.
- Native packages go to the edge repository on both package hosts. Docker publishes `edge` and `latest` to
  Docker Hub, Quay and GHCR. The Docker `latest` tag therefore tracks nightly builds.
- Nightlies do not trigger the stable Helmchart or MSI follow-up workflows.

## Stable releases: major, minor and patch

Use a canonical `vMAJOR.MINOR.PATCH` tag without leading zeroes, prerelease suffixes or build metadata.
An existing tag cannot be reused.

| Type | Version rule | Example progression |
|---|---|---|
| `major` | Minor and patch must both be zero | `v3.0.0-rc1` → `v3.0.0` |
| `minor` | Patch must be zero | `v2.13.0-rc1` → `v2.13.0` |
| `patch` | Version must be newer than the branch's current core version | `v2.13.0` → `v2.13.1` |

- All three types require an existing `vMAJOR.MINOR` branch and matching major/minor numbers in its
  `packaging/version`. Start a new series with an RC first; stable major/minor releases do not create branches.
- Major/minor releases may promote a same-core prerelease to its final version. Otherwise the requested core
  must be newer than the branch's current core. Patch releases always require a newer core. Preparation rejects
  rollback from a newer patch already recorded on the branch.
- Source, static installer and Windows artifacts go under the `stable` artifact prefix. They also go to `preview`
  when their numeric version core is at least the core advertised by `preview/latest/Version`, or that metadata
  does not yet exist. A final release qualifies against an RC with the same core.
- Artifact aliases (`latest`, major and major/minor) advance only when the release is newer than the version
  already advertised at each alias. Versioned artifacts are still published for releases of older series.
- Native packages go to the stable repository on both package hosts, and also to preview when the same preview
  eligibility check passes.
- Docker publishes the exact version, major, major/minor and `stable` tags to Docker Hub, Quay and GHCR, plus
  `preview` when eligible. These Docker aliases are assigned by the build; they do not use the artifact alias
  comparison above. Stable releases do not update Docker's `latest` tag.
- The Docker workflow dispatches the Agent Version PR workflow in `netdata/helmchart` and the stable Build
  workflow in `netdata/msi-installer`. Check those follow-up runs as part of the release.

The [release preparation script](scripts/prepare-release-base.sh) owns version and branch validation.
The [shared preview eligibility helper](scripts/modules/release_versions.py) owns stable-to-preview routing.

## Release candidates

- Tags use `vMAJOR.MINOR.0-rcX`, where core numbers follow SemVer and `X` is a positive decimal integer without
  leading zeroes. Build metadata, other prerelease names and nonzero patch numbers are rejected.
- The workflow also accepts the package-style form `vMAJOR.MINOR.0~rcX` and normalizes it to `-rcX`: Git tags and
  Docker tags cannot contain `~`. Tags, `packaging/version`, artifact names and Docker tags use `-rcX`; native
  package versions use `MAJOR.MINOR.0~rcX`, derived by the build system, so an RC sorts below its final release.
- Each version starts at `rc1`. Subsequent RC numbers must exceed every existing RC number for that version;
  gaps are allowed. Existing tags cannot be reused.
- An RC is rejected if a regular release of the same version already exists.
- The first RC starts a newer minor series in the current major, or a newer major with minor zero, from `master`.
  It creates the `vMAJOR.MINOR` release branch at the changelog/version commit. Subsequent RCs require that branch.
- Downstream builds receive type `release-candidate`. Source/static/Windows artifacts go exclusively to the
  `preview` artifact prefix; native packages go exclusively to the preview repository on both package hosts.
  RCs can update preview's `latest` alias according to the existing version comparison, but do not create major/minor aliases.
- GitHub release drafts are created in the main repository and marked as prereleases. The existing manual
  draft-publication step still applies.
- Docker publishes only the exact version tag, such as `v2.13.0-rc1`, to each registry. RCs do not update
  `stable`, `preview`, `latest`, major or minor Docker aliases. Stable Helmchart and MSI follow-up workflows are skipped.

## Other build and release workflows

Development/PR builds are distinct from the numbered Agent release process. The native package workflow supports
`devel` builds, routed to the devel repository with a `0.<commit-SHA>` version on manual dispatch. They do not
create a stable release or RC series through the Release workflow.

The DEM runtime has an independent version and release workflow. Its
[release runbook](../packaging/dem-runtime/RELEASE.md) covers build-only dispatches, target qualification,
protected publication, signing and channel promotion, including infrastructure prerequisites. An Agent release
does not replace that process.

## Offline validation

```sh
python3 .github/scripts/test-prepare-release-base.py
python3 .github/scripts/test-artifact-prefixes.py
python3 .github/scripts/test-gen-package-repos.py
python3 .github/scripts/test-gen-docker-tags.py
```

These tests use temporary Git repositories and intercepted upload/download commands. They do not publish releases
or require credentials. The **Release process tests** workflow runs them for affected pull requests.
