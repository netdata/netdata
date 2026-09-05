# Skills

Runtime skills for this repository: one directory per skill, holding `SKILL.md` plus optional topic files and
`scripts/`, directly under this directory. The layout is flat because skill loaders discover one `SKILL.md` per
directory here; nothing is nested. The three `query-*` entries are relative symlinks to the public skills under
`docs/netdata-ai/skills/`; their names are a published contract and follow the public convention, not this one.

## Naming

Every runtime skill is `<area>-<topic>`. The area says where in the product or the repo process the work lands, so a
sorted listing groups related skills; the topic says what the skill does. The frontmatter `name` MUST equal the
directory name.

## Areas

| Area | Covers |
|---|---|
| `collectors` | any data-collection plugin or module, across plugin families; the second token names the sub-area (`go`, `snmp`, `prometheus`, `metadata`) |
| `integrations` | the `integrations/` pipeline: schemas, generators, taxonomy, generated pages |
| `health` | `src/health/`: alerts and alert templates |
| `topology` | topology producers, payload schema, correlation, Cloud aggregation fixtures |
| `tests` | repository test suites under `tests/` |
| `packaging` | installers, packages, and images under `packaging/` |
| `docs` | `learn.netdata.cloud` and the docs pipeline |
| `triage` | classifying defects reported by external systems or the fleet (Coverity, SonarCloud, Codacy, CodeQL, agent events) |
| `repo` | repo-wide process and workstation setup that belongs to no component (PR review iteration, source mirrors) |

Adding an area: it MUST name a product component or a repo-wide process. A language (`go`, `rust`, `c`) or a vendor or
protocol (`snmp`, `aws`) is the second token under its component, never an area. Add the row here in the same change;
`.agents/sow/audit.sh` reads this table to check every skill's prefix.

## Finding A Skill

The per-skill grouped index, with the entry point of each area, is the "Skills index" in the root `AGENTS.md`; a
skill's frontmatter description is its trigger. A skill that serves two areas is listed under both there.
