# Skills

Runtime skills for this repository: one directory per skill, holding `SKILL.md` plus whatever supporting files and
directories the skill needs (topic files, `scripts/`, `how-tos/`, `recipes/`, `decisions/`), directly under this
directory. Skill directories are never nested, because skill loaders discover one `SKILL.md` per directory here. The
three `query-*` entries are relative symlinks to the public skills under `docs/netdata-ai/skills/`; their names are a
published contract and are exempt from the naming rule below.

## Naming

- Every runtime skill MUST be named `<area>-<topic>`: the area, one lowercase alphanumeric word (it is the text before
  the first hyphen), says where in the product or the repo process the work lands, so a sorted listing groups related
  skills; the topic, lowercase alphanumeric words joined by hyphens, says what the skill does. Within `collectors`, the
  topic starts with the sub-area when there is one (`go`, `snmp`, `prometheus`, `metadata`) or with the activity
  (`authoring`).
- The frontmatter `name` MUST equal the directory name.
- `.agents/sow/audit.sh` enforces both, and also that every skill directory has a `SKILL.md`, every public symlink
  points into `docs/netdata-ai/skills/` and resolves, every skill is in the `AGENTS.md` skills index and every index
  entry has a directory, every `.agents/skills/` path named in a tracked file (or relative `../` path in a skill
  file) exists, and every owner-section citation resolves (below).

## Owner-Section Citations

- A skill points at facts rather than restating them. When a skill depends on a specific section of the document that
  owns a fact, it MUST cite the section as `<path>/<doc>.md#<anchor>`, where `<anchor>` is the heading's slug as
  GitHub renders it for an ATX heading with a plain ASCII title (code spans flattened, inline HTML removed, links
  reduced to their text; then lowercase; letters, digits, spaces, hyphens, and underscores kept, everything else
  dropped; spaces become hyphens; a repeated heading gets `-1`, `-2`, and this repository's check gives a further
  suffix to a generated slug that collides with a literal title, which GitHub may not). Cite only such headings:
  non-ASCII titles, setext headings, and titles that collide with generated suffixes are outside the convention. Paths
  are repo-relative (with or without a leading `/`), or relative to the citing file when they start with `./` or
  `../`. URLs are written with their scheme.
- `.agents/sow/audit.sh` fails when the cited file or a heading with that slug is missing, so renaming or removing a
  heading in an owner document is caught until every citing skill is updated. Headings and citations inside fenced
  code blocks or multi-line HTML comments are not scanned.
- A privately published owner document (one not mapped in `docs/.map/map.yaml`) that skills cite by section SHOULD
  carry a short "Place in the documentation set" paragraph naming the citing skill(s), so an editor of the document
  knows the dependency exists; Learn-published documents carry no such paragraph.

## Areas

| Area | Covers |
|---|---|
| `collectors` | any data-collection plugin or module, across plugin families |
| `integrations` | the `integrations/` pipeline: schemas, generators, generated pages |
| `health` | `src/health/`: alerts and alert templates |
| `topology` | topology producers, the `netdata.topology.v1` payload contract, correlation, the aggregator contract |
| `tests` | repository test suites under `tests/` |
| `packaging` | installers, packages, and images under `packaging/` |
| `docs` | `learn.netdata.cloud` and the docs pipeline |
| `triage` | classifying defects reported by external systems or the fleet (Coverity, SonarCloud, Codacy, CodeQL, agent events) |
| `repo` | repo-wide process and workstation setup that belongs to no component (PR review iteration, source mirrors) |

- An area MUST name a product component or a repo-wide process. A language (`go`, `rust`, `c`) or a vendor or protocol
  (`snmp`, `aws`) is the start of the topic under its component, never an area.
- An area's row MUST be added to this table in the same change that introduces it. The audit reads the first cell of
  every row in this section (a lowercase word in backticks) as the allowed prefixes.

## Finding A Skill

The per-skill grouped index is the "Skills index" in the root `AGENTS.md`: skills under their area, an area's entry
point marked where it has one, and a skill that serves two areas cross-referenced from the other. A skill's frontmatter
description is its trigger. How to create, edit, slim, split, or review a skill, and the rot signals to watch for:
`repo-skill-authoring`, which cites sections of this file and of the root `AGENTS.md` by heading anchor.

Selection by operation and affected contract, review applicability, reference depth and action boundaries are owned by
`AGENTS.md#skill-selection`. The index and descriptions support that selection; they do not authorize execution.

## Verification Scenarios

Offline invocation cases and their grading rubric live under `.agents/skill-verification/invocation/`. Use them when
changing discovery, task routing or shared selection rules. They test selection and action boundaries without running
the operational procedures they describe. The query-specific question sets beside them are separate operational
seeds; loading a verification file does not authorize its live queries.
