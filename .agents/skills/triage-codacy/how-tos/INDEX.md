# triage-codacy how-tos catalog

Capture timing and authorization follow `AGENTS.md#knowledge-capture`. Add authorized recipes under `how-tos/` and
keep this catalog consistent with those changes.

## Entries

- [fetch-large-pr-issue-list](fetch-large-pr-issue-list.md) -- fix or verify the `jq: Argument list too long` failure mode by passing large Codacy issue arrays through a temporary file and `jq --slurpfile`.
- [handle-malformed-local-json](handle-malformed-local-json.md) -- handle `analyze-local.sh` dumps that have a `.json` suffix but contain Codacy tool-runner logs instead of parseable JSON.
- [reproduce-pr-22423-markdownlint](reproduce-pr-22423-markdownlint.md) -- reproduce the 864 markdownlint findings PR #22423 saw on its first CI run, locally via `analyze-local.sh --tool markdownlint`.
- [triage-action-required-without-token](triage-action-required-without-token.md) -- fetch public Codacy v3 PR issue details when GitHub check-run annotations are empty and no `CODACY_TOKEN` is available.
