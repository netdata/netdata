# Agent releases

Run the **Release** workflow on `master`. It validates the requested version, updates the changelog and
`packaging/version`, pushes the tag and branches, then dispatches artifact, package and Docker builds.

| Build type | Version example | Source branch | Publication |
|---|---|---|---|
| `nightly` | `nightly` | `master` | Nightly artifacts; edge packages; `netdata/netdata-nightlies` GitHub release |
| `release-candidate` | `v2.13.0-rc01` | `master` for the first RC (creates `release-v2.13`); `release-v2.13` thereafter | Preview artifacts/packages; GitHub prerelease draft |
| `minor` | `v2.13.0` | Existing `release-v2.13` | Stable and eligible preview destinations; GitHub release draft |
| `major` | `v3.0.0` | Existing `release-v3.0` | Stable and eligible preview destinations; GitHub release draft |
| `patch` | `v2.13.1` | Existing `release-v2.13` | Stable and eligible preview destinations; GitHub release draft |

## Version forms

`master` never carries release tags. It carries annotated development marker tags (`vMAJOR.MINOR`) that name the
series under development; releases are tagged only on their `release-vMAJOR.MINOR` release branch. Examples for the period
around the `v2.14` series:

| Build | Version (`packaging/version`, tags, artifacts, Docker) | DEB/RPM | Windows MSI |
|---|---|---|---|
| Stable | `v2.14.0`, `v2.14.1` | `2.14.0`, `2.14.1` | `2.14.100`, `2.14.101` |
| Release candidate | `v2.14.0-rc01`, `v2.14.0-rc02` | `2.14.0~rc01` | `2.14.1`, `2.14.2` |
| Nightly before `v2.14.0-rc01` | `v2.14-29-nightly` | `2.14.0~nightly.29` | `2.14.0.29` |
| Nightly after `v2.14.0-rc01` | `v2.15-37-nightly` | `2.15.0~nightly.37` | `2.15.0.37` |
| Nightly after a stable tag on `master` (pre-marker transition) | `v2.13.0-29-post-nightly` | `2.13.0.29.post` | `2.13.100.29` |
| Git build | `v2.15-617-g3340f53a85` | `2.15.0~git.617.g3340f53a85` | `2.15.0.617` |

- Within one major.minor version, nightlies sort before release candidates, and release candidates before the
  stable release. `-post` nightlies sort after the stable release they follow. Git builds are not ordered against
  nightlies.
- MSI only compares the first three fields, so stable patch numbers are offset by 100 to sort after RCs (rc01-rc99).
- [NetdataVersion.cmake](../packaging/cmake/Modules/NetdataVersion.cmake) derives the package and MSI versions;
  the updater's `parse_version` and [release_versions.py](scripts/modules/release_versions.py) implement the same
  ordering.

## Running a release

1. Choose the build type and version from the table. For a stable release, ensure the intended changes are on
   the existing `release-vMAJOR.MINOR` branch; preparation does not merge changes from `master` into that branch.
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
- Preparation runs from `master`, generates the changelog and writes `<tag>-<count>-nightly`, where `<tag>` is the
  nearest tag (normally the development marker, such as `v2.15`) and `<count>` is the number of commits since it,
  so a nightly of the marker commit itself is number `0`. When the nearest tag is a stable release (before the first
  marker exists), the suffix is `-post-nightly`, because that nightly follows the release.
- The nightly commit only changes `packaging/version` and the changelog; the build system uses that version instead
  of `git describe` when it builds that commit. The dispatch version remains `nightly`.
- Source, static installer and Windows artifacts go under the `nightly` artifact prefix, including `latest`.
  GitHub releases are published automatically in `netdata/netdata-nightlies`, rather than drafted in the main repository.
- Native packages go to the edge repository on both package hosts. Docker publishes `edge` and `latest` to
  Docker Hub, Quay and GHCR.
- Nightlies do not trigger the stable Helmchart or MSI follow-up workflows.

## Stable releases: major, minor and patch

Use a canonical `vMAJOR.MINOR.PATCH` tag without leading zeroes, prerelease suffixes or build metadata.
An existing tag cannot be reused.

| Type | Version rule | Example progression |
|---|---|---|
| `major` | Minor and patch must both be zero | `v3.0.0-rc01` → `v3.0.0` |
| `minor` | Patch must be zero | `v2.13.0-rc01` → `v2.13.0` |
| `patch` | Version must be newer than the branch's current core version | `v2.13.0` → `v2.13.1` |

- All three types require an existing `release-vMAJOR.MINOR` branch and matching major/minor numbers in its
  `packaging/version`. Existing series using the old `vMAJOR.MINOR` branch name must be renamed to `release-vMAJOR.MINOR` before
  dispatching this workflow; the `vMAJOR.MINOR` development marker tag stays in place.
- Start a new series with an RC first; stable major/minor releases do not create branches.
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
- Docker publishes the exact version tag to Docker Hub, Quay and GHCR (`latest` is nightly-only). The major, major/minor and
  `stable` tags are added only when the release is not older than the image currently published at that alias
  in the destination registry. The same comparison controls `preview`. Each registry is checked independently,
  so a failed artifact or registry publication cannot make another registry's aliases move backwards.
- The Docker workflow dispatches the Agent Version PR workflow in `netdata/helmchart` and the stable Build
  workflow in `netdata/msi-installer`. Check those follow-up runs as part of the release.

The [release preparation script](scripts/prepare-release-base.sh) owns version and branch validation.
The [shared release ordering helper](scripts/modules/release_versions.py) owns release ordering and artifact/package
preview eligibility. The [Docker tag generator](scripts/gen-docker-tags.py) reads each alias's
`org.opencontainers.image.version` label from its Linux/amd64 image using
[`skopeo inspect`](https://github.com/containers/skopeo/blob/main/docs/skopeo-inspect.1.md), without pulling layers.
Registry publication jobs install `skopeo` and use the credentials established by the existing registry login step.
Each job checks aliases after building images, immediately before pushing the manifest, and serializes that check
and push across release refs for its registry, using a queued concurrency group. An absent alias can be created; missing/invalid version labels,
authentication errors, rate limits and other inspection failures stop publication. Retry the Docker workflow after
correcting those failures; artifact metadata is never used as a fallback.

## Release candidates

- Tags use `vMAJOR.MINOR.0-rcNN`, where core numbers follow SemVer and `NN` is two digits from `01` to `99`.
  Release candidates are only made for major and minor releases. Build metadata, other prerelease names and
  nonzero patch numbers are rejected.
- The workflow also accepts `rcN` and the package-style `~rcN` and normalizes them to `-rcNN`: Git tags and Docker
  tags cannot contain `~`. Native package versions use `MAJOR.MINOR.0~rcNN`, derived by the build system.
- Each version starts at `rc01`. Subsequent RC numbers must exceed every existing RC number for that version;
  gaps are allowed. Existing tags cannot be reused.
- An RC is rejected if a regular release of the same version already exists.
- The first RC must not be older than the series under development on `master` (its marker tag, or the latest
  stable tag before markers exist); a new major requires minor zero. It creates the `release-vMAJOR.MINOR` release branch
  from `master` as it stands, commits the changelog/version there and tags the RC on that branch. It then adds an
  empty commit to `master` with the annotated marker tag for the next series (`v2.15` after `v2.14.0-rc01`,
  `v3.1` after `v3.0.0-rc01`). Branch, tags and `master` are pushed atomically. Subsequent RCs require the branch.
- Downstream builds receive type `release-candidate`. Source/static/Windows artifacts go exclusively to the
  `preview` artifact prefix; native packages go exclusively to the preview repository on both package hosts.
  RCs can update preview's `latest` alias according to the existing version comparison, but do not create major/minor aliases.
- GitHub release drafts are created in the main repository and marked as prereleases. The existing manual
  draft-publication step still applies.
- Docker publishes the exact version tag, such as `v2.13.0-rc01`, to each registry, plus `preview`
  when the RC is not older than the image at that registry's `preview` alias. RCs do not update `stable`, major or minor Docker
  aliases. Stable Helmchart and MSI follow-up workflows are skipped.

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
python3 .github/scripts/test-version-scheme.py
```

These tests use temporary Git repositories and intercepted upload/download commands. They do not publish releases
or require credentials. The **Release process tests** workflow runs them for affected pull requests.

The Build workflow's source and static installer checks use the versioned artifact directory named by
`packaging/version`. Verification therefore runs even when an older release or a repeated build does not advance
the channel's `latest` alias. The offline tests exercise both verification jobs with older/repeated RCs, an older
stable release and a nightly, checking the selected versioned manifests and artifact checksums without installation.
