---
name: packaging-component-releases
description: Design or review an independently released Netdata component, a native package and R2 bundle release workflow, signing and promotion, repository coverage, or a packaging/SRE handoff before merge or first publication.
---

# Independently released components

Start with the installation routes. For each one, identify who builds, installs, updates, activates and removes the
payload. An independently released component needs a compatibility contract that an installed consumer can check.
The DEM runtime is a worked example. Its workflow and release guide own its current settings.

| Task | Start here |
|---|---|
| Change a DEB/RPM relationship, payload or maintainer script | `.agents/skills/packaging-native-packages/SKILL.md#independently-versioned-payloads` |
| Select build runners and native repository destinations | `./SKILL.md#runners-and-native-distribution` |
| Review a signed archive, channel update or publication retry | `./SKILL.md#archive-trust-and-promotion` |
| Prepare a packaging PR for SRE or assess release readiness | `./sre-handoff.md#packaging-review-and-first-publication` |

A design or review can use source, logs and existing artifacts. Building packages, installing them on a host,
using signing credentials, publishing and contacting SRE each require scope for that action.

## Runners and native distribution

- Use the architecture runner from `.github/data/distros.yml` for native build jobs. Architecture-independent
  orchestration can use `ubuntu-latest` when it has no host-version requirement. Pin the payload's toolchain and
  donor inputs separately. Changing a CI runner should not silently select a different browser or library.
- Derive repository targets from the active `include` entries with package metadata. Intersect each entry's
  `packages.arches` with the architectures the artifact supports, map DEB/RPM architecture names, and deduplicate
  actual destinations. The catalog's separate `legacy` list is outside the DEM publisher's inputs.
- A user normally has one Agent release channel configured. For a component shared across channels, submit the
  signed packages to stable, edge and devel on **both** package hosts. The hosts are independently populated.
  Repoconfig is for repository configuration packages. A new repository needs a plan for client configuration and
  backend support. See `packaging/dem-runtime/RELEASE.md#infrastructure-configuration` for the actual destinations.
- Inspect the shared uploader's file selection and staging. A reused temporary tree can send a previous target's
  package to the next destination. Exercise the real uploader with the network transport intercepted. Assert the
  package identity per host/channel/distro/architecture and propagation of a failed submission. See
  `packaging/dem-runtime/ci.py`, `.github/scripts/package-upload.sh` and `packaging/dem-runtime/test_ci.py`.

## Archive trust and promotion

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
  size and digest. In DEM the next steps are native package submission, then promotion of the signed per-architecture
  pointers. The exact order and retry procedure live in `packaging/dem-runtime/RELEASE.md#release-sequence`.
- Check partial success across destinations. Reuse authenticated, identical release payloads on retry. Signing
  timestamps can change signature bytes for the same content. Conflicting immutable content or a backwards channel
  sequence must fail. Verify mutable channel state against its authoritative store and test what happens after one
  architecture succeeds and another fails. A successful SSH submission still requires repository ingestion.

Use `.github/workflows/dem-runtime-release.yml` for job boundaries and
`packaging/dem-runtime/RELEASE.md#local-validation-and-limits` for the scope of publisher and package tests.
