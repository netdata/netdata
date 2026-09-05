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
  entry has a directory, and every `.agents/skills/` path named in a tracked file (or relative `../` path in a skill
  file) exists.

## Areas

| Area | Covers |
|---|---|
| `collectors` | any data-collection plugin or module, across plugin families |
| `integrations` | the `integrations/` pipeline: schemas, generators, taxonomy, generated pages |
| `health` | `src/health/`: alerts and alert templates |
| `topology` | topology producers, payload schema, correlation, Cloud aggregation fixtures |
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
description is its trigger.
