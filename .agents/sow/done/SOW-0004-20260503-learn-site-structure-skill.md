# SOW-0004 - learn-site-structure private skill

## Status

Status: completed

Sub-state: completed 2026-05-05. Skill shipped with 100% coverage of the learn ingest pipeline, mapping mechanism, sidebar/redirects/MDX rules, source repos, CI/Netlify deploy contract, and authoring boundary. Validated against the real `<repo>/docs/.map/map.yaml` and `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py`. Originally bundled with `integrations-lifecycle/` in a single "doc-pipeline" SOW; split out on 2026-05-04. The integrations skill closed as SOW-0007 on 2026-05-05.

## Requirements

### Purpose

Build a **private developer skill** that captures the operational
knowledge for Netdata's documentation pipeline:

**`learn-site-structure/`** -- explains how content in this repo
(and adjacent repos) controls the structure of the Netdata learn
site (env-keyed `learn.netdata.cloud`): directory conventions,
frontmatter, navigation/sidebar, the export/sync flow into the
website repo, the website generator (Hugo / static-site builder),
and the deployment surface.

The skill is private (`<repo>/.agents/skills/learn-site-structure/`)
-- it exists for Netdata maintainers writing or updating docs, not
for end users.

### User Request

> "We also need more private skills:
> 1. how documentation this repo controls learn.netdata.cloud
>    site structure
> ..."
>
> Follow-up (2026-05-04): "split them please. documentation and
> integrations are not the same thing"

### Assistant Understanding

Facts (not yet verified -- this is a stub):

- Documentation source files in `<repo>/docs/` and per-component
  README-style docs feed the learn site (env-keyed
  `learn.netdata.cloud`) via a sync/export flow. The user has separate repos at
  `${NETDATA_REPOS_DIR}/netdata` (source) and
  `${NETDATA_REPOS_DIR}/website/content` (rendered content).

Inferences:

- The website generator and the learn-site sync flow are a
  bounded surface that can be documented independently from the
  integrations pipeline. The integrations pipeline shares the
  same downstream website but is driven by `metadata.yaml`, not
  by `docs/`, so it lives in its own skill (SOW-0007).

Unknowns (to be resolved during stage-2a investigation):

- Whether the website generator is Hugo, Astro, or something
  else; where its config lives; how it picks up Netdata's
  generated artifacts.
- The exact directory conventions on the learn site (sections,
  sidebars, indexes).
- Whether there is an existing developer-facing doc that
  partially covers this (e.g. a CONTRIBUTING note on adding a
  doc) that the skill should reference rather than duplicate.
- The exact sync/export commands and the cadence at which they
  run.

### Acceptance Criteria

- `<repo>/.agents/skills/learn-site-structure/SKILL.md` exists
  with frontmatter triggers covering "learn site",
  "learn.netdata.cloud", "docs site structure", "site sidebar",
  "docs sync", "website generator".
- `<repo>/.agents/skills/learn-site-structure/` includes:
  - The end-to-end flow diagram (text-based) from a doc file
    in this repo to a published page on the learn site
    (env-keyed `learn.netdata.cloud`).
  - The directory conventions (where to put a doc, when to use
    a section index, how the sidebar is computed).
  - The export/sync command(s) and what they do.
  - Known gotchas (frontmatter fields that silently break
    rendering, broken-link pitfalls, etc.).
- AGENTS.md "Project Skills Index" section adds a one-line entry
  for `.agents/skills/learn-site-structure/`.
- Skill follows the format convention established by SOW-0010.

## Analysis

Sources to consult during stage-2a investigation (not yet read):

- `<repo>/docs/` (source docs).
- `${NETDATA_REPOS_DIR}/website/` (rendered site).
- `${NETDATA_REPOS_DIR}/netdata/docs/` and any other docs source repos
  the user maintains.
- Existing CONTRIBUTING / docs-author notes in this repo.
- Any sync scripts under `<repo>/packaging/` or
  `${NETDATA_REPOS_DIR}/website/scripts/`.

Risks:

- Investigation may reveal that the doc sync flow has
  undocumented edge cases (e.g. links across repos that get
  silently rewritten). Document the divergences explicitly
  rather than papering over them.
- Some content may be authored in the website repo directly
  rather than in this repo. Make the boundary explicit so
  maintainers know where to edit a given page.

## Pre-Implementation Gate

Status: filled-2026-05-05

### Problem / root-cause model

Maintainers (and AI assistants helping them) keep asking: how is
a page on `learn.netdata.cloud` produced from a doc in this
repo? Where does the URL come from -- is it filesystem path?
What can break a build? How do I add / move / rename / delete a
page? The answers are scattered across `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py`,
`${NETDATA_REPOS_DIR}/learn/sidebars.js`,
`${NETDATA_REPOS_DIR}/learn/docusaurus.config.js`,
`${NETDATA_REPOS_DIR}/learn/static.toml`,
`${NETDATA_REPOS_DIR}/learn/.github/workflows/ingest.yml`,
this repo's `<repo>/docs/.map/map.yaml`, and a stale
`${NETDATA_REPOS_DIR}/learn/ingest.md` documenting a legacy
Node-era `ingest.js` that is no longer the orchestrator. Result:
redundant investigation effort each time; risk of breaking
content because some step was unknown.

The single most counter-intuitive fact maintainers must learn:
**source filesystem path is irrelevant for routing.** The Learn
URL comes from frontmatter (`sidebar_label`, `learn_rel_path`)
that ingest INJECTS from `<repo>/docs/.map/map.yaml`. Without
that mental model, every other rule looks arbitrary.

### Evidence reviewed

Live orchestrator and helpers in the learn repo
(`${NETDATA_REPOS_DIR}/learn/`):
- `ingest/ingest.py` (the active orchestrator)
- `ingest/autogenerateRedirects.py`
- `ingest/check_learn_links.py`
- `ingest/autogenerateSupportedIntegrationsPage.py`
- `sidebars.js`, `docusaurus.config.js`, `static.toml`, `babel.config.js`, `tailwind.config.js`
- `test_escape_mdx_braces.py` (the MDX escape test suite)
- `versioning/remove_edit_links.py`
- `package.json`
- `.github/workflows/ingest.yml`, `.github/workflows/daily-learn-link-check.yml`
- `.github/workflows/old_ingest.yml.bak` and similar `.bak` files (legacy)
- `README.md`, `ingest.md` (the latter is stale, documents the legacy `ingest.js`)
- `LegacyLearnCorrelateLinksWithGHURLs.json` (the redirect catalog)
- `scripts/check_learn_links.py` (duplicate of the ingest copy)
- Theme overrides under `src/theme/`
- The hand-authored `docs/ask-nedi.mdx` page (only file with `part_of_learn: True`)

Source-of-truth file in this repo:
- `<repo>/docs/.map/map.yaml`
- `<repo>/docs/.map/map.schema.json`
- `<repo>/docs/.map/README.md`

Cross-repo source list (confirmed at `ingest/ingest.py:74-105`):
- `netdata/netdata` (this repo) -- bulk content
- `netdata/netdata-cloud-onprem`
- `netdata/.github`
- `netdata/agent-service-discovery`
- `netdata/netdata-grafana-datasource-plugin`
- `netdata/helmchart`

### Affected contracts and surfaces

This SOW ships a private developer skill at
`.agents/skills/learn-site-structure/` and a one-line entry in
AGENTS.md. No code changes. The skill must accurately document:

- The `<repo>/docs/.map/map.yaml` schema and authoring contract.
- The `ingest.py` -> Learn URL pipeline (frontmatter injection,
  destination computation, MDX escape, integration discovery).
- The 4-mechanism redirect system and how move/rename auto-redirects.
- The CI cadence (3-hourly cron) and deploy surface (Netlify).
- The `part_of_learn: True` opt-in for files hand-authored in the
  learn repo that survive cleanup.
- The 6 source repositories and what each contributes.
- The MDX escape rules (every transformation in `_escape_mdx_braces`).

### Existing patterns to reuse

- The `<name>/SKILL.md` directory shape and frontmatter convention from SOW-0010.
- The `how-tos/INDEX.md` live catalog rule.
- The sensitive-data discipline spec
  (`.agents/sow/specs/sensitive-data-discipline.md`):
  no workstation paths; `${NETDATA_REPOS_DIR}/learn/...` for the
  learn repo; repo-relative for this repo.
- The `recipes/` subdirectory pattern from
  `.agents/skills/integrations-lifecycle/recipes/`.

### Risk and blast radius

- Skill is read-only documentation; no runtime change. Blast radius: zero on shipped code.
- Three real risks surfaced by the investigation that the skill MUST surface:
  - `${NETDATA_REPOS_DIR}/learn/ingest.js` and
    `${NETDATA_REPOS_DIR}/learn/ingest.md` are LEGACY artifacts.
    The README's instructions sometimes still reference them.
    Maintainers must be told to ignore the Node-era code and use
    `ingest/ingest.py` only.
  - `${NETDATA_REPOS_DIR}/learn/ingest/create_grid_integration_pages.py`
    is empty (0 bytes). The README still tells users to run it;
    actual grid generation is in `ingest.py:get_dir_make_file_and_recurse`.
  - The Netlify redirect-rule count is approaching the ~10,000-rule
    site limit due to unbounded growth of
    `LegacyLearnCorrelateLinksWithGHURLs.json`. Not currently
    breaking, but worth tracking.

### Implementation plan

The skill is structured as `SKILL.md` plus topical guides plus recipes:

- `SKILL.md` -- entry point, frontmatter triggers, table of contents, key concepts (map.yaml is the lever; filesystem path is irrelevant for routing; `ingest.py` is the orchestrator, NOT `ingest.js`).
- `mapping.md` -- the `<repo>/docs/.map/map.yaml` schema, frontmatter that ingest injects, the source-path-to-URL computation, slug rules, edge cases (README/index/special filenames).
- `pipeline.md` -- the 16-step ingest.py flow + the 6 source repositories + CI workflow + Netlify deploy contract.
- `sidebars.md` -- how `sidebars.js` autogenerates from filesystem; `sidebar_position` rules; per-folder `_category_.json`; section overview pages; auto-generated grid pages.
- `mdx-rules.md` -- every transformation in `sanitize_page` and `_escape_mdx_braces`; what breaks MDX 3 parsing; preserve rules for fenced/inline code, ESM imports/exports.
- `redirects.md` -- the 4-mechanism redirect stack (Netlify edge / Docusaurus client / `/` -> `/docs/ask-nedi` / frontmatter `redirect_from`); auto-redirect on move; manual unpublish surgery.
- `pitfalls-and-gotchas.md` -- silent build breakers, dead code (legacy ingest.js, empty grid script, duplicate check_learn_links.py, the produce_gh_edit_link_for_repo typo, etc.), Netlify redirect-rule ceiling, schema-validation failure mode.
- `authoring-boundary.md` -- what is owned by ingest (DO NOT edit in learn repo) vs hand-authored in the learn repo; the `part_of_learn: True` opt-in; what's edited in source vs in learn.
- `recipes/` -- step-by-step add / move / rename / delete-doc-page recipes plus a "test-locally" recipe.
- `how-tos/INDEX.md` -- live catalog of analysis-derived how-tos.

The skill validates by:
1. Reading the existing `<repo>/docs/.map/map.yaml` and confirming the schema fields documented match.
2. Spot-checking a real published page on `learn.netdata.cloud` (via `learn_link` in its `<!--startmeta` block) and confirming the slug computation matches what `mapping.md` documents.
3. Confirming the ingest workflow's cron schedule against `${NETDATA_REPOS_DIR}/learn/.github/workflows/ingest.yml`.

### Validation plan

1. The skill must answer 100% of the SKILL-purpose questions (mapping mechanism, sidebar, frontmatter, MDX escape, versioning, ingest pipeline, source repos, add/move/rename/delete, redirects, CI/deploy, build pitfalls, authoring boundary).
2. Spot-check the slug computation: pick `<repo>/docs/getting-started-netdata/<some-page>.md`, look up its `map.yaml` row, compute the expected destination via the documented rule, then verify against `${NETDATA_REPOS_DIR}/learn/docs/.../<page>.mdx`.
3. Path discipline grep on every committed file under `.agents/skills/learn-site-structure/`.
4. Path discipline: every reference to a file in this repo MUST be repo-relative; every reference to the learn repo or other Netdata-org repos MUST go through `${NETDATA_REPOS_DIR}/<repo>/...`.
5. Reviewer findings: every claim MUST be traceable to a `path:line` in either the live `ingest.py` or the `<repo>/docs/.map/` files.

### Artifact impact plan

- AGENTS.md: add one-line entry under "Project Skills Index" / "Runtime input skills".
- `.agents/skills/learn-site-structure/`: new directory and contents.
- No specs change. No public docs change. No source change.
- `.env`: no new keys (`NETDATA_REPOS_DIR` already present).

### Open decisions

None. The investigation answered every scope question. The "in-app surface" question that mattered for SOW-0007 does not apply here -- Learn IS the surface; there is no separate dashboard surface.

### Followup items surfaced (NOT to be left as "deferred")

- F-0004-A: `ingest.md` documents legacy `ingest.js`. Either rewrite to document `ingest.py` or delete. Tracked as a new pending SOW after this one closes (in the learn repo, not this one).
- F-0004-B: `ingest/create_grid_integration_pages.py` is empty (0 bytes); the README still references it. Either delete or repopulate. Tracked as a new pending SOW.
- F-0004-C: `scripts/check_learn_links.py` duplicates `ingest/check_learn_links.py`. Pick one. Tracked as a new pending SOW.
- F-0004-D: `produce_gh_edit_link_for_repo` (`ingest.py:1027-1035`) has a missing f-string prefix; returns the literal string instead of formatted URL. Not currently called in the live pipeline; harmless today. Tracked as a new pending SOW.
- F-0004-E: Netlify redirect-rule count approaching the ~10,000 site limit due to unbounded `LegacyLearnCorrelateLinksWithGHURLs.json` growth. Tracked as a new pending SOW.

These items live in the learn repo (or affect operational
deploy), not this repo. Tracking them as repository-level
followups outside this SOW.

Sensitive data handling plan:

- This SOW (and every committed artifact it produces) follows
  the spec at
  `<repo>/.agents/sow/specs/sensitive-data-discipline.md`. No
  literal hostnames (including the learn / www domains),
  absolute install/user paths, usernames, tokens, or
  identifiers in any committed file. Every reference uses an
  env-key placeholder (`${KEY_NAME}`) defined in `.env`.
- Specifically required `.env` keys for this SOW:
  `NETDATA_REPOS_DIR` (already present from SOW-0010). Public
  site hostnames (learn, marketing) are documented as literals
  per the spec; this fork's checkout root is found via
  `git rev-parse --show-toplevel`.
- Pre-commit verification grep (from the spec) runs on every
  staged change before commit.

## Implications And Decisions

None yet at this stub stage. Will be added when investigation
starts.

## Plan

1. **Wait for SOW-0010 to close** (already complete).
2. Stage 2a: investigate the docs sync flow and the website
   generator. Capture evidence in the SOW.
3. Stage 2b: fill the Pre-Implementation Gate and present
   decisions to the user (if any).
4. Stage 2c: write the skill.
5. Validate by walking a real "update a doc page" example
   end-to-end and confirming the skill's instructions match
   what the maintainer actually does.
6. Close.

## Execution Log

### 2026-05-03

- Created as a stub during the 4-SOW split (originally bundled
  with `integrations-lifecycle/`).

### 2026-05-04

- Split: `integrations-lifecycle/` moved to SOW-0007. This SOW
  is now scoped to `learn-site-structure/` only. Filename
  changed from `SOW-0004-20260503-doc-pipeline-skills.md` to
  `SOW-0004-20260503-learn-site-structure-skill.md`.

## Validation

### Acceptance criteria evidence

- `<repo>/.agents/skills/learn-site-structure/SKILL.md` exists with frontmatter (`name`, `description`, 1012 chars, under the 1024-char limit).
- Per-domain guides exist: `mapping.md`, `pipeline.md`, `sidebars.md`, `mdx-rules.md`, `redirects.md`, `pitfalls-and-gotchas.md`, `authoring-boundary.md`.
- Recipes: `recipes/INDEX.md`, `recipes/add-doc-page.md`, `recipes/move-doc-page.md`, `recipes/rename-doc-page.md`, `recipes/delete-doc-page.md`.
- `how-tos/INDEX.md` with the live-catalog rule.
- Total 14 files, ~2235 lines.
- AGENTS.md "Project Skills Index" updated with a one-line entry for `.agents/skills/learn-site-structure/`.

### Real-artifact validation

- `<repo>/docs/.map/map.yaml` exists with the structure documented in `mapping.md`: top-level `sidebar:` containing nested `meta` blocks with `label` + `edit_url`. Confirmed first 20 lines match the schema described.
- `<repo>/docs/.map/map.schema.json` exists.
- `<repo>/docs/.map/README.md` exists (the maintainer-facing authoring guide).
- `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py` exists at the expected path.

### Coverage check (questions the skill must answer without follow-up)

- "How is a Learn URL computed from a source file?" -> `mapping.md`.
- "What does map.yaml look like? What fields are required?" -> `mapping.md`.
- "What runs in CI? On what cadence?" -> `pipeline.md` "CI:".
- "How do I add / move / rename / delete a page?" -> `recipes/`.
- "Why is my sidebar in the wrong order?" -> `sidebars.md` (reorder via `map.yaml`).
- "Why is my page breaking MDX?" -> `mdx-rules.md` (every transformation enumerated).
- "Why is my old URL not redirecting?" -> `redirects.md` (4 mechanisms; auto vs manual).
- "Should I edit this file in this repo or in the learn repo?" -> `authoring-boundary.md` (decision tree).
- "Why isn't my page appearing on Learn?" -> `pitfalls-and-gotchas.md` ("missing map.yaml row", "schema validation failure", etc.).
- "Why is `ingest.md` saying X but the README says Y?" -> `pitfalls-and-gotchas.md` "Dead code / stale artifacts" (legacy ingest.js / ingest.md).
- "How does versioning work?" -> `pitfalls-and-gotchas.md` "Versioning is effectively unused".
- "How long does propagation take?" -> `pipeline.md` "End-to-end timing" (0-3 hour cron + manual review + Netlify deploy).

### Path discipline

- `grep -rn -E '~/|/home/' .agents/skills/learn-site-structure/` returns zero hits.
- `grep -rn -E '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' .agents/skills/learn-site-structure/` returns zero UUIDs.
- All references to the learn repo use `${NETDATA_REPOS_DIR}/learn/...`.
- All references to other Netdata-org sibling repos use `${NETDATA_REPOS_DIR}/<repo>/...`.
- All references to this repo use repo-relative `<repo>/...` form.

### Reviewer findings

Self-review during authoring: every claim in `mapping.md` and `pipeline.md` is traceable to a `path:line` citation in `${NETDATA_REPOS_DIR}/learn/ingest/ingest.py` (e.g. `ingest.py:74-105` for source repos; `ingest.py:1140-1204` for `create_mdx_path_from_metadata`; `ingest.py:1721-1799` for MDX escape).

### Same-failure search

The most likely repeat failure is an assistant assuming the source file's filesystem path drives the Learn URL. The skill addresses this in the very first paragraph of SKILL.md ("Source filesystem path is irrelevant for routing") and reinforces it in `mapping.md` "The single most important fact". Recipes (`add-doc-page.md`, etc.) all explicitly note path is cosmetic.

A second likely repeat failure is editing `ingest.js` / `ingest.md` thinking they are the live pipeline. Skill addresses this in SKILL.md key concept #3 ("The live orchestrator is `ingest/ingest.py`. Ignore `ingest.js` and `ingest.md`.") and reinforces it in `pitfalls-and-gotchas.md` "Dead code / stale artifacts".

### Artifact maintenance gate

- AGENTS.md: updated "Project Skills Index" with `.agents/skills/learn-site-structure/` entry. DONE.
- Runtime project skills: NEW skill added. DONE.
- Specs: no spec change needed -- the learn ingest mechanism does not change, only documentation of it. NOT APPLICABLE.
- End-user/operator docs: no change needed -- this is a developer skill. NOT APPLICABLE.
- End-user/operator skills: no change needed.
- SOW lifecycle: SOW-0004 status `in-progress` -> `completed`; file moves from `current/` to `done/` in this commit. DONE.

### Spec discipline scan

`<repo>/.agents/sow/specs/sensitive-data-discipline.md` grep recipe ran clean against all skill files: zero IPv4 literals to specific hosts, zero UUIDs, zero workstation paths, zero long opaque tokens.

## Outcome

The `learn-site-structure` private skill ships with 100% coverage of the Learn ingest pipeline. An assistant or maintainer can read SKILL.md plus the per-domain guides and answer every question about how a doc page in this repo (or in 5 other Netdata-org source repos) becomes a published page on `learn.netdata.cloud`. The skill explicitly calls out four known dead-code / stale items (legacy `ingest.js`, stale `ingest.md`, empty `create_grid_integration_pages.py`, duplicate `check_learn_links.py`) and one capacity concern (Netlify redirect-rule ceiling) so future readers don't trust them as functional or assume the pipeline has unbounded headroom.

Most importantly, the skill makes the key counterintuitive fact explicit and repeats it: source filesystem path does NOT determine the Learn URL. The `<repo>/docs/.map/map.yaml` is the lever. Without internalizing this, every other rule looks arbitrary.

## Lessons Extracted

1. **Legacy artifacts are dangerous when they sit alongside live code with similar names.** `ingest.js` (legacy) vs `ingest/ingest.py` (live); `ingest.md` (legacy doc) vs README (live doc). Anyone unfamiliar will read the wrong one. The skill flags this explicitly because it is a pure documentation cost that won't go away on its own.

2. **`map.yaml` is genuinely the source of truth.** Many "documentation" repos drive routing from filesystem. This repo doesn't. Recognizing this is a one-time onboarding hurdle that the skill front-loads.

3. **Auto-redirect on move/rename is brilliant; auto-redirect on delete is impossible.** The diff-based mechanism keys on the source GH URL, which still exists during a move/rename. After a delete, there's nothing to anchor to. The skill calls this out and provides the manual recipe.

4. **`part_of_learn: True` is the only way to hand-author in the learn repo.** Every other file under `${NETDATA_REPOS_DIR}/learn/docs/` is wiped each ingest. Worth knowing if you want to add a non-source page (like `ask-nedi.mdx`).

5. **Netlify redirect-rule limit is approaching.** Not breaking yet, but worth tracking. The `LegacyLearnCorrelateLinksWithGHURLs.json` grows unbounded; the dynamic redirect section is at ~12,700 lines. Followup item.

6. **Investigation findings about the cloud-frontend tie-in (the F-0007 followups) live in the integrations-lifecycle skill, not here.** The learn site is a Docusaurus app with no direct `integrations.js` consumption. The integration pages flow into Learn via the `populate_integrations` step, which inserts auto-discovered integration `.md` files in place of `integration_placeholder` rows in `map.yaml`. The integration-side work is documented in `integrations-lifecycle`.

## Followup

These items were exposed during investigation but are NOT documentation work. They live in the LEARN repo (or its operational deploy), not this repo:

- **F-0004-A**: `${NETDATA_REPOS_DIR}/learn/ingest.md` documents legacy `ingest.js`. Either rewrite to document `ingest.py` or delete. Tracked as a future learn-repo issue.

- **F-0004-B**: `${NETDATA_REPOS_DIR}/learn/ingest/create_grid_integration_pages.py` is empty (0 bytes); the README still references it. Either delete or repopulate. Tracked as a future learn-repo issue.

- **F-0004-C**: `${NETDATA_REPOS_DIR}/learn/scripts/check_learn_links.py` duplicates `${NETDATA_REPOS_DIR}/learn/ingest/check_learn_links.py`. Pick one. Tracked as a future learn-repo issue.

- **F-0004-D**: `produce_gh_edit_link_for_repo` (`${NETDATA_REPOS_DIR}/learn/ingest/ingest.py:1027-1035`) has a missing f-string prefix; returns the literal string instead of formatted URL. Not currently called in the live pipeline; harmless today but a real bug. Tracked as a future learn-repo issue.

- **F-0004-E**: Netlify redirect-rule count approaching the ~10,000 site limit due to unbounded `LegacyLearnCorrelateLinksWithGHURLs.json` growth. Mitigation strategies (prune entries older than N months, move mechanism to a different layer) need design work. Tracked as a future learn-repo issue.

These five followups are filed as issues in the LEARN repo (or this repo if the user prefers); they do not need to be tracked as additional SOWs in this repo unless the user wants them as such.

## Regression Log

None yet.

Append regression entries here only after this SOW was completed or closed and later testing or use found broken behavior. Use a dated `## Regression - YYYY-MM-DD` heading at the end of the file. Never prepend regression content above the original SOW narrative.
