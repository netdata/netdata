# Agent releases

Run the **Release** workflow on `master`. It validates the requested version, updates the changelog and
`packaging/version`, pushes the tag and branches, then dispatches artifact, package and Docker builds.

| Build type | Version example | Source branch | Publication |
|---|---|---|---|
| `nightly` | `nightly` | `master` | Existing nightly destinations |
| `release-candidate` | `v2.13.0-rc1` | `master` for the first RC; `v2.13` thereafter | Preview artifacts/packages; GitHub prerelease draft |
| `minor` | `v2.13.0` | Existing `v2.13` | Stable and eligible preview destinations; GitHub release draft |
| `major` | `v3.0.0` | Existing `v3.0` | Stable and eligible preview destinations; GitHub release draft |
| `patch` | `v2.13.1` | Existing `v2.13` | Stable and eligible preview destinations; GitHub release draft |

## Release candidates

- Tags use `vMAJOR.MINOR.0-rcX`, where core numbers follow SemVer and `X` is a positive decimal integer without
  leading zeroes. Build metadata, other prerelease names and nonzero patch numbers are rejected.
- Each version starts at `rc1`. Subsequent RC numbers must exceed every existing RC number for that version;
  gaps are allowed. Existing tags cannot be reused.
- An RC is rejected if a regular release of the same version already exists.
- The first RC starts a newer minor series in the current major, or a newer major with minor zero, from `master`.
  It creates the `vMAJOR.MINOR` release branch at the changelog/version commit. Subsequent RCs require that branch.
- Major, minor and patch releases require an existing release branch. Stable major/minor releases no longer create one.
  Final releases may promote a same-core prerelease, but cannot roll back a newer patch version.
- Downstream builds receive type `release-candidate`. Source/static/Windows artifacts go exclusively to the
  `preview` artifact prefix; native packages go exclusively to the preview repository on both package hosts.
  RCs can update preview's `latest` alias according to the existing version comparison, but do not create major/minor aliases.
- GitHub release drafts are created in the main repository and marked as prereleases. The existing manual
  draft-publication step still applies.
- Docker publishes only the exact version tag, such as `v2.13.0-rc1`, to each registry. RCs do not update
  `stable`, `preview`, `latest`, major or minor Docker aliases. Stable Helmchart and MSI follow-up workflows are skipped.

Release workflow runs are serialized across dispatch refs. Each preparation pushes its branch and tag updates
atomically, so a failed push cannot publish only part of that update.

## Offline validation

```sh
python3 .github/scripts/test-prepare-release-base.py
python3 .github/scripts/test-artifact-prefixes.py
python3 .github/scripts/test-gen-package-repos.py
python3 .github/scripts/test-gen-docker-tags.py
```

These tests use temporary Git repositories and intercepted upload/download commands. They do not publish releases
or require credentials. The **Release process tests** workflow runs them for affected pull requests.
