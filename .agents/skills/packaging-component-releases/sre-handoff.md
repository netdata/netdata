# Packaging review and first publication

Write the PR so a packaging reviewer can tell what to review, what has been proved locally, and who owns the next
operation. Put the installation routes, outstanding decisions and test limitations in the visible description.

1. **Describe the operator's path.** Show the opt-in action, download source, size, installation owner, update and
   activation behavior, and removal for each supported install method. In DEM, native users install an optional
   package. Static, Docker and source deployments explicitly provision a managed archive or preload. Native hooks
   can restart a previously active Agent. A managed update prepares a release for activation on restart.
2. **Map the diff to reviewers.** Point packaging/SRE to CPack metadata, maintainer scripts, installer changes,
   build matrices, signing and repository upload. Point runtime maintainers to verification, extraction, service
   identity and readiness checks. Identify the contract they share, such as the capability a package hook needs.
3. **Show the actual evidence.** For each tested artifact record its identity, architecture, host and operation.
   Separate metadata inspection, package extraction, local-file package transactions, and installation from a
   published repository. For an upgrade, show what happened to a running service, an inactive one, file ownership
   and existing state. For browser sandbox claims, name the enforced host policy and observed process context.
   A container on the builder shares its kernel. An SELinux-enforcing host can still run an unconfined service.
4. **Separate decisions by milestone.** A merge can approve a build-only default and the delivery design while
   production publication remains gated. Mark each outstanding item with its owner, the action or decision needed,
   and whether it blocks merge, first publication or user rollout. Record omissions such as an untested architecture
   or a post-install check run only with fixtures. Keep user rollout separate from artifact availability.

| When | Question for the named owner | Evidence to link |
|---|---|---|
| Code review | Which catalog, repository and existing signing conventions does this component reuse? Which exceptions need agreement? | Workflow, generated destination matrix, dependency metadata and signing/verifier test |
| Before first publication | Which storage remote and public HTTPS base serve the exact bytes? Who configures immutable retention and caching of mutable pointers? | Confirmed route, storage and cache settings, and release runbook |
| Before first publication | Who owns key custody/rotation, environment reviewers, branch restrictions, host keys and scoped credential access? | Names of required settings, which already exist and which need provisioning |
| Before first publication | Which exact artifacts were qualified and who approved the matching sources and licenses? | Artifact digests, supported host results and corresponding-source review |
| Before promoting the channel pointer | Can clients fetch the immutable runtime and sources through the public URL? | Public read-back sizes and digests matching the signed release |
| Before availability is announced | Did each repository ingest the signed package, and can a client install it and use the feature? | Signed repository metadata and a fresh install/update through the published channel |

If a reviewer asks to reuse a signing key, clarify **both** the signature protocol and how an installed client
obtains replacement public keys. Record offline behavior and what happens when refresh fails. If someone proposes
a bucket or hostname by analogy, mark it as proposed until the owner confirms its public routing. For DEM,
`packaging/dem-runtime/RELEASE.md#infrastructure-configuration` owns the setting names and trust/rotation limits.
In a PR, request identifiers and ownership only. Credential values belong in the protected release environment.
