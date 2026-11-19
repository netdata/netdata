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
- MSI `ProductVersion` allows at most `255.255.65535`, and Win32 assembly manifests allow at most 65535 in each of
  four fields. Preparation rejects a release or nightly whose mapped Windows version exceeds those limits, before
  anything is tagged: major and minor at most 255, stable patch at most 65435, nightly count at most 65535.
  See [ProductVersion](https://learn.microsoft.com/en-us/windows/win32/msi/productversion) and
  [assembly versions](https://learn.microsoft.com/en-us/windows/win32/sbscs/assembly-versions).
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

Every serialized publication step uses a queued concurrency group (`queue: max`), so a newer release waits
instead of replacing an older pending one. Numbered release dispatches of the Build, Docker and native package
workflows do not share a workflow-level group. Nightly dispatches still replace an older pending nightly, and pull
request and push runs still cancel superseded runs for the same ref.

## Changelog generation

`packaging/generate-changelog.sh` requires Python 3.11 or newer and git-cliff. It writes `CHANGELOG.md` using
an explicit inventory of canonical stable tags after `v1.47.0`, including tags on other release branches.
The checkout must contain that cutoff tag and the complete release tags/history; release preparation uses a
full-history checkout. The generator replaces the changelog only after all sections render successfully.

- Historical sections appear in descending numeric version order. Each section uses the range from the preceding
  lower stable version to its tag. Git's revision exclusion removes shared commits even across divergent branches;
  branch-only fixes remain in the release that contains them. An older-series patch published after a newer minor
  retains its own heading. Annotated tags supply release dates; lightweight tags use their commit dates.
- The unreleased section uses the highest stable version at or below the version in `packaging/version` as its
  baseline. A reachable development marker can advance that version before the next nightly updates the file.
  Thus maintenance builds use their own series, while master excludes shared work already delivered in a prior
  series. Without a qualifying stable release, the baseline is `v1.47.0`.
- RCs, development markers and nightlies do not create historical release headings. Pending stable releases remain
  under `unreleased` until tagged, as before. An empty unreleased range is omitted; an explicit stable section may
  contain no entries when its range contains only automation commits excluded by `cliff.toml`.
- git-cliff still owns formatting, grouping, PR links and automation exclusions. The helper supplies ranges and
  exact tag selection; changing `use_branch_tags` alone cannot restore boundaries for tags off master.

For inspection without replacing the checkout's changelog:

```sh
packaging/generate-changelog.sh --output /tmp/netdata-changelog.md
```

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
| `patch` | The branch must already carry a released version (not an RC), and the version must be newer than it | `v2.13.0` → `v2.13.1` |

- All three types require an existing `release-vMAJOR.MINOR` branch and matching major/minor numbers in its
  `packaging/version`. Existing series using the old `vMAJOR.MINOR` branch name must be renamed to `release-vMAJOR.MINOR` before
  dispatching this workflow; the `vMAJOR.MINOR` development marker tag stays in place. Such a branch also needs
  the release tooling backported first; see
  [Maintenance branches created before release candidates](#maintenance-branches-created-before-release-candidates).
- Start a new series with an RC first; stable major/minor releases do not create branches.
- Major/minor releases may promote a same-core prerelease to its final version. Otherwise the requested core
  must be newer than the branch's current core. Patch releases always require a newer core. Preparation rejects
  rollback from a newer patch already recorded on the branch.
- Source, static installer and Windows artifacts go under the `stable` artifact prefix. They also go to `preview`
  when their numeric version core is at least the core advertised by `preview/latest/Version`, or that metadata
  does not yet exist. A final release qualifies against an RC with the same core.
- Artifact aliases (`latest`, major and major/minor) advance only when the release is not older than the version
  already advertised at each alias. Versioned artifacts are still published for releases of older series.
  The upload job holds one queued concurrency group shared by stable and RC publishers. Inside it, the job
  publishes the versioned directory first, then re-reads each alias's `Version` from the bucket and skips any
  alias that another release has already moved to a newer version.
- The release identity (`packaging/release_info.py`, mirrored by `NetdataVersion.cmake`) accepts only `vX.Y.Z` for
  the `stable` channel and `vX.Y.Z-rcNN` for `release-candidate`. A build dispatched with a type that does not match
  its version therefore fails before its runtime companion or any artifact is published.
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
  `v3.1` after `v3.0.0-rc01`). Branch, tags and `master` are pushed atomically. Subsequent RCs require the branch,
  still at an RC of the same version: once a release of the series is recorded there, the branch cannot go back.
- Downstream builds receive type `release-candidate`. Source/static/Windows artifacts go exclusively to the
  `preview` artifact prefix; native packages go exclusively to the preview repository on both package hosts.
  RCs can update preview's `latest` alias according to the existing version comparison, but do not create major/minor aliases.
- GitHub release drafts are created in the main repository and marked as prereleases. The existing manual
  draft-publication step still applies.
- Docker publishes the exact version tag, such as `v2.13.0-rc01`, to each registry, plus `preview`
  when the RC is not older than the image at that registry's `preview` alias. RCs do not update `stable`, major or minor Docker
  aliases. Stable Helmchart and MSI follow-up workflows are skipped.

## Maintenance branches created before release candidates

Release branches cut before release candidates existed (currently `v2.12`, whose tip is the shipped `v2.12.3`)
lack the tooling the Release workflow needs once a tag is pushed:

- The runtime companion workflows run from `master` but execute helpers from the tagged source:
  `packaging/release_info.py`, `packaging/dem-runtime/` (including `ci.py`, `build.py`, `release.py` and
  `sources.py`), the `src/dem-runtime/` manifests and the installer helpers and tests they reference.
- The Build, Docker and native package workflows run from the tag itself and must accept the `source_sha` and
  `companion_run_id` inputs the Release workflow sends, so their current versions and the scripts they call
  must be on the branch.

Before releasing from such a branch:

1. Rename it to `release-vMAJOR.MINOR` (the `vMAJOR.MINOR` marker tag stays).
2. Backport the items above to the branch through a normal pull request against it.
3. Dispatch the release. Preparation checks the branch for every helper path the companion workflows reference and
   for the dispatched workflows' inputs. It fails, naming what is missing, before anything is committed or tagged.
   Without this check, the tag would be pushed and the release would then fail, and a fresh dispatch for the same
   version would be rejected because its tag already exists.

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
python3 .github/scripts/test-generate-changelog.py
```

These tests use temporary Git repositories and intercepted upload/download commands. They do not publish releases
or require credentials. The **Release process tests** workflow runs them for affected pull requests.
Changelog tests require git-cliff, which that workflow installs, and exercise real temporary Git histories.

The Build workflow's source and static installer checks use the versioned artifact directory named by
`packaging/version`. Verification therefore runs even when an older release or a repeated build does not advance
the channel's `latest` alias. The offline tests exercise both verification jobs with older/repeated RCs, an older
stable release and a nightly, checking the selected versioned manifests and artifact checksums without installation.
