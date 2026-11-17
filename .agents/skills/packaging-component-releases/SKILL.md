---
name: packaging-component-releases
description: Design or review a Netdata component release, paired Agent/runtime publication, native package and R2 bundle workflows, signing and promotion, repository coverage, or a packaging/SRE handoff before merge or first publication.
---

# Component releases

Start with the installation routes. For each one, identify who builds, installs, updates, activates and removes the
payload. Decide whether it follows the Agent release identity or has a separate release train before adding update
policy. The DEM runtime follows the Agent; its exact pair and publication order are owned by
`packaging/dem-runtime/RELEASE.md#build-and-publication-order`. An independently released component would need a
separate compatibility/overlap policy; do not inherit that model from historical DEM guidance.

| Task | Start here |
|---|---|
| Change a DEB/RPM relationship, payload or maintainer script | `.agents/skills/packaging-native-packages/SKILL.md#versioned-payloads` |
| Select build runners and native repository destinations | `./SKILL.md#runners-and-native-distribution` |
| Review a signed archive, exact companion or publication retry | `./SKILL.md#archive-trust-and-publication` |
| Prepare a packaging PR for SRE or assess release readiness | `./sre-handoff.md#packaging-review-and-first-publication` |

A design or review can use source, logs and existing artifacts. Building packages, installing them on a host,
using signing credentials, publishing and contacting SRE each require scope for that action.

## Runners and native distribution

- Use the architecture runner from `.github/data/distros.yml` for native build jobs. Architecture-independent
  orchestration can use `ubuntu-latest` when it has no host-version requirement. Pin the payload's toolchain and
  donor inputs separately. Changing a CI runner should not silently select a different browser or library.
- Derive repository targets from the active `include` entries with package metadata. Intersect each entry's
  `packages.arches` with the architectures the artifact supports, map DEB/RPM architecture names, and deduplicate
  actual destinations. Use the normal Agent native workflow for paired components; it owns distro/channel routing.
- A user normally has one Agent release channel configured. For a component shared across channels, submit the
  signed packages to stable, edge and devel on **both** package hosts. The hosts are independently populated.
  Repoconfig is for repository configuration packages. A new repository needs a plan for client configuration and
  backend support. This fan-out applies only to genuinely channel-independent components, not paired DEM releases.
  `.github/workflows/packaging.yml` owns the Agent's destinations.
- Inspect the shared uploader's file selection and staging. A reused temporary tree can send a previous target's
  package to the next destination. Exercise the real uploader with the network transport intercepted. Assert the
  package identity per host/channel/distro/architecture and propagation of a failed submission. See
  `.github/scripts/package-upload.sh` and the normal native packaging workflow.

## Archive trust and publication

- Measure the same payload with the candidate codecs. When the application owns extraction, choose a format its
  shipped decoder supports and test maximum window and expanded-size bounds. DEM uses zstd level 19 with an 8 MiB
  window for runtime and corresponding-source archives. Its Go decoder avoids a host executable dependency.
  A change to that format also affects names, signed sizes and hashes, extraction fixtures and offline instructions.
- Reuse established package signing and its canonical public key source when the installed verifier supports them.
  A CI secret and a client trust anchor have different owners. If the signature format changes, test the real
  producer against the installed verifier and reject malformed signatures, untrusted keys and altered payloads.
  DEM signs release descriptors with the existing package OpenPGP key. Online preparation retrieves the public
  keyring from the canonical package-repository HTTPS URL. Offline import requires administrator-provisioned trust.
  A release descriptor cannot choose its own verification key.
- Keep build-only CI separate from protected publication. Identify what the public can fetch by immutable identity,
  including matching sources, licenses and provenance. Read the objects through the public route and verify their
  size and digest. DEM publishes and verifies companions before Agent release lanes proceed. The exact artifact
  receipt binding and retry procedure live in
  `packaging/dem-runtime/RELEASE.md#exact-ci-artifacts-and-native-preparation`.
- Check partial success across destinations. Reuse authenticated, identical release payloads on retry. Signing
  timestamps can change signature bytes for the same content. Conflicting immutable content must fail. DEM publishes exact
  immutable identities without runtime latest aliases, promotion or Agent-version ordering. Test retries after one
  architecture succeeds and another fails. A successful SSH submission still requires repository ingestion.

Use `.github/workflows/dem-runtime-release.yml` for job boundaries and
`packaging/dem-runtime/RELEASE.md#local-validation-and-limits` for the scope of publisher and package tests.

Keep CI proportional: browser/source assembly follows artifact or execution inputs, excluding documentation,
metadata and unit-test-only changes. Gate public exposure and payload consumers rather than unrelated compilation;
GitHub intermediates are build transport. Reuse the Node wire workflow with the frozen source input. Retain release
smoke, focused ARM checks, same-run receipts and native inventory equality.
