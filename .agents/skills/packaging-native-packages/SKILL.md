---
name: packaging-native-packages
description: Change, review or troubleshoot Netdata native DEB/RPM packaging, CPack component dependencies, optional plugin packages, account creation, scriptlet ordering and installed ownership. Use for package transaction failures or adding a packaged plugin. Static makeself builds use packaging-static-installer.
---

# Native Package Changes

Use this skill for the repository's CPack DEB/RPM packages. Static artifacts and their build checks use
`.agents/skills/packaging-static-installer/SKILL.md`; collector runtime behavior uses
`.agents/skills/collectors-authoring/SKILL.md`.

Review and diagnosis start from source and existing artifacts. Loading this skill does not authorize package
installation, service changes or publication. An authorized test transaction SHOULD use a disposable target matching
its distro; a build request alone does not authorize installing packages on the workstation.

## Choose The Affected Contract

| Task | Read |
|---|---|
| Add a component or change default plugin delivery | `./SKILL.md#preserve-dependency-policy`; component registry and plugin install rules |
| Diagnose a missing account or wrong installed owner | `./SKILL.md#account-creation-and-payload-ownership`; selected scriptlets and transaction log |
| Validate a native packaging change | `./SKILL.md#validate-at-the-relevant-layer`; metadata and transaction evidence |

## Source Owners

- `packaging/cmake/Modules/Packaging.cmake`: package names, dependency strengths, distro predicates, scriptlet
  selection, file attributes and component registry.
- `packaging/cmake/Modules/NetdataOSRelease.cmake`: distro family/version detection, including test release overrides.
- `packaging/cmake/Modules/NetdataOptions.cmake` and the affected plugin module under `packaging/cmake/Modules/`:
  feature defaults and component install rules.
- `packaging/cmake/pkg-files/deb/` and `packaging/cmake/pkg-files/rpm/`: maintainer scripts; open the selected file,
  including templates, instead of inferring behavior from its name.
- `packaging/build-package.sh` and `.github/workflows/packaging.yml`: native build/test orchestration and target matrix.
- `packaging/runtime-check.sh`: runtime artifact checks, including executable presence and selected plugin privileges.

## Preserve Dependency Policy

- A preferred runtime collector or an enabled build option MUST NOT, by itself, promote its native package to a main
  package hard dependency. Determine the component's established installation policy before editing dependencies.
- For optional hardware integrations such as IPMI, preserve the main package's `Suggests` policy. The plugin can be
  built and available without being required by the main package. Operator setup MUST say when separate installation
  is required; collector metadata and its hand-maintained README are the documentation owners.
- Check dependency direction: `CPACK_COMPONENT_PLUGIN-IPMI_DEPENDS` and the plugin's dependency on `netdata` do not
  mean that `netdata` requires the plugin. Inspect the main DEB/RPM dependency lists separately.
- Preserve weak-dependency tiers. `NETDATA_RPM_HAVE_WEAK_DEPS` and `NETDATA_RPM_PLUGIN_SUGGESTS` own the RPM policy;
  optional suggestions are omitted on unsupported tiers, not promoted to hard requirements. The fallback for
  selected recommended components is a separate policy in `packaging/cmake/Modules/Packaging.cmake`.
- Preserve package-family conventions: the DEB suggestions list and conditional, versioned RPM suggestions have
  different forms. Validate both with the feature enabled and disabled; do not impose symmetry without evidence.

## Account Creation And Payload Ownership

- A payload naming the `netdata` account MUST have a working account-creation and transaction-ordering path on its
  target distro. A successful package build does not prove the account exists when RPM installs the payload.
- Treat `NETDATA_RPM_USER_POST`, `NETDATA_RPM_HAVE_SYSUSER` and `NETDATA_RPM_USER_PREDEP` as related but distinct
  decisions. Check the selected user scriptlet, its dependencies and the consuming payload's `Requires(pre)` together.
  DEB packages use their `Pre-Depends` and `packaging/cmake/pkg-files/deb/user/postinst` path.
- Distinguish an installed `systemd-sysusers` command from RPM's automatic sysusers integration. The explicit
  `packaging/cmake/pkg-files/rpm/user/post.create-user` script can invoke that command even when RPM's native hook is
  disabled. Upstream RPM capability or a newer RPM version does not establish downstream distro policy.
- Preserve verified exceptions in `packaging/cmake/Modules/Packaging.cmake`: EL uses explicit account creation;
  Fedora's native-hook predicate and SUSE's user/group provides follow different paths. Do not copy a Fedora cutoff to
  EL or add a future distro version cutoff without evidence that its package transaction supports the hook.
- For a new distro predicate, inspect that distro's packaging source or macros and validate the actual transaction.
  Keep existing explicit creation until the replacement is demonstrated; record unavailable evidence as a limitation.
- When install logs report a missing account, a scriptlet failure or fallback to `root`, investigate ordering before
  weakening ownership checks. Verify final mode and user/group names, including setuid/capabilities where applicable.

## Validate At The Relevant Layer

1. Inspect generated `CPackConfig.cmake` and, when available, the built package metadata. Test the main-to-plugin
   dependency separately from the plugin-to-main dependency, component inclusion and installed file paths.
2. For a changed predicate, cover both sides of its boundary and affected distro families. Include feature opt-outs
   and weak-dependency tiers when relevant; generated metadata fixtures do not execute package-manager scriptlets.
3. For account ordering or ownership fixes, use a fresh disposable target without a preexisting `netdata` account.
   Exercise the generated package/scriptlets in a real transaction and inspect the final owner and mode. An existing
   account can hide the failure. Add upgrade/removal evidence when those paths change.
4. Separate configure, build, transaction and runtime failures in CI. Read the failing step and diagnostics before
   changing compiler flags or relaxing runtime assertions. Report fixture checks and actual package transactions as
   different evidence; neither alone establishes all supported architectures.

The focused IPMI regression suite provides examples for dependency, account-ordering metadata, component staging and
privileges in `packaging/installer/tests/test_ipmi_delivery.py`. From the repository root, with Python 3, Bash and
CMake supporting the RPM packaging configuration on `PATH`:

```bash
python3 -m unittest discover packaging/installer/tests 'test_*ipmi*.py' -v
```

`.github/workflows/installer-tests.yml` (`ipmi-delivery`) owns the test-tool setup. Use its CMake requirement when
reproducing CI; missing CMake skips the metadata test class. Check for skips before reporting coverage. These tests
use temporary fixtures and do not prove a real DEB/RPM install transaction.

Capture reusable findings under `AGENTS.md#knowledge-capture`. Keep task logs and generated metadata under
`<repo>/.local/audits/native-packages/`; do not copy incident history into this skill.
