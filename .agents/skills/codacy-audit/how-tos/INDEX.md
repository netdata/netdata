# codacy-audit how-tos catalog

This catalog is **live**. When an assistant performs concrete analysis on a Codacy question that requires more than one wrapper call OR more than one jq pipeline OR cross-referencing another skill, AND the answer is not already documented here or in the per-domain SKILL.md, the assistant MUST author a new entry under `how-tos/<slug>.md` AND add it here BEFORE marking the task complete.

Skipping this rule means the next assistant repeats the analysis from scratch -- that is an explicit framework violation.

## Entries

- [reproduce-pr-22423-markdownlint](reproduce-pr-22423-markdownlint.md) -- reproduce the 864 markdownlint findings PR #22423 saw on its first CI run, locally via `analyze-local.sh --tool markdownlint`.
