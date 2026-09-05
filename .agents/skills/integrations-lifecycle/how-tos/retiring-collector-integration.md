# Retiring a collector integration completely

Removing a collector means retiring its whole ownership surface, not only its implementation directory.

## 1. Establish the replacement and breaking contract

Before deletion, record whether the collector has a replacement and prove which contracts do not migrate automatically:

- module and stock-config identity;
- file and DynCfg job configuration;
- auto-discovery behavior;
- chart contexts, dimensions, alerts, dashboards, and queries;
- integration name and published Learn URL.

Collector and chart removal is a public-contract change. Obtain explicit approval for immediate removal or a staged
deprecation/migration plan before implementation.

## 2. Map the complete authoritative surface

Search the package path, module name, integration ID, stock-config path, emitted contexts, and discovery rule identity.
Include dynamically constructed references and all installation paths.

For a `go.d` collector, check at least:

- `src/go/plugin/go.d/collector/<module>/`;
- `src/go/plugin/go.d/collector/init.go`;
- `src/go/plugin/go.d/config/go.d.conf`;
- `src/go/plugin/go.d/config/go.d/<module>.conf`;
- `src/go/plugin/go.d/config/go.d/sd/`;
- `src/go/plugin/go.d/README.md`;
- `src/health/health.d/` and `src/health/guides/`;
- packaging/install/update scripts;
- replacement metadata, tests, and related-resource links;
- CI, CODEOWNERS, taxonomy, and repository-wide path/content references.

Classify every surviving product-name match. A separate collector, alert owner, log parser, OS group, exporter profile,
or product deployment guide is not a trace of the retired collector merely because it names the same product.

## 3. Preserve discovery without creating duplicate jobs

Read the discovery engine's rule evaluation semantics before changing a service rule. Some engines emit every matching
rule rather than stopping at the first match. If a generic exporter rule already covers the replacement, delete the
dedicated rule; changing its `module` can create two replacement jobs for one endpoint.

Validate the rendered stock configuration for a representative target and require exactly the intended replacement job.

## 4. Clean installed stock artifacts safely

Stopping shipment of a stock file does not guarantee that overlay/tarball upgrades remove an existing copy. Check every
packaging path:

- native package managers normally remove files no longer owned by the new package;
- overlay/static installers may need an explicit retired-artifact cleanup;
- cleanup MUST target only the installed stock path;
- cleanup MUST NOT delete `/etc/netdata` or another user-owned configuration path.

Use the installer's existing traced command wrapper for destructive file operations.

## 5. Handle metadata, taxonomy, and generated outputs by ownership

Delete authoritative `metadata.yaml` and `taxonomy.yaml` with the collector. Remove backlinks and ownership assertions in
replacement metadata/tests.

Generated outputs follow two cases:

1. A generated integration page and README symlink live inside the directory being retired. Delete them with the complete
   directory; retaining a generated-only directory is not a clean collector removal. The sensors retirement
   (`aba1472fe81a2c9367e6120e202846ce90adcaa9`) is the repository precedent.
2. Generated pages that survive elsewhere, umbrella pages, and gitignored catalogs are not hand-edited. Regenerate them
   locally for inspection and let the documented post-merge generated-artifact workflow publish their final changes.

Before delivery, ensure gitignored `integrations/integrations.js`, `integrations/integrations.json`, and
`integrations/taxonomy.json` are not staged.

## 6. Route deleted Learn pages

Integration pages are inserted into Learn by the integration pipeline. Deleting their source does not automatically
retarget historical redirects. Follow `.agents/skills/docs-learn-site-structure/recipes/delete-doc-page.md`:

1. identify every redirect anchored to the deleted GitHub source;
2. choose a live replacement or explicitly accept 404 behavior;
3. check whether source-PR CI ingests Learn after regenerating pages. If it does, land the cross-repository redirect
   surgery before the source deletion; otherwise land it no later than the deletion.

## 7. Validate the clean end state

Run:

- focused tests for the registry, replacement collector/profile, and discovery engine;
- the collector package test sweep so registration/import drift fails;
- packaging script syntax and focused upgrade-path inspection;
- integration metadata, taxonomy, and generated-doc validation;
- zero-reference searches for the retired package path, integration ID, stock-config path, contexts, and module wiring;
- a retain-list audit proving replacement and independent same-product integrations remain.

Inspect generated diffs, then restore or leave unstaged every generator-owned output that belongs to the post-merge route.

## How I figured this out

Files read:

- `src/go/plugin/go.d/collector/init.go` and `src/go/plugin/go.d/config/go.d.conf`;
- `src/go/plugin/go.d/config/go.d/sd/net_listeners.conf` and
  `src/go/plugin/agent/discovery/sd/pipeline/{services.go,promport.go}`;
- `packaging/makeself/install-or-update.sh`;
- replacement collector metadata and ownership tests;
- `integrations/gen_integrations.py`, `integrations/gen_docs_integrations.py`, and both integration CI workflows;
- the sensors collector-removal commit `aba1472fe81a2c9367e6120e202846ce90adcaa9`;
- `netdata/learn` redirect ownership documented by the Learn structure skill.

Commands run:

- `rg '<module>|<collector-path>|<integration-id>|<context-prefix>' src integrations packaging`;
- focused registry, replacement, and discovery tests;
- integration metadata, taxonomy, and generated-document validation;
- `git diff --check`.
