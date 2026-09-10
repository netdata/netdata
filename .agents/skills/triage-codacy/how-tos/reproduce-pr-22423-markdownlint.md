# How-to: reproduce PR #22423's 864 markdownlint findings locally

## When to use

You want to confirm `analyze-local.sh` matches what Codacy CI reported on a known fixture. PR #22423 is the canonical fixture for this skill: its first CI run reported **864 markdownlint findings**; commit `3a54c9afbc` cleared them by adding `.agents/**` and `docs/netdata-ai/skills/**` to `.codacy.yml`.

This how-to walks through reproducing those 864 findings on the pre-exclusion state, then confirming the post-exclusion state shows zero on the affected files.

## Prerequisite

- Codacy CLI v2 (`codacy-cli`) installed locally.
- `<repo>/.env` need NOT contain `CODACY_TOKEN` -- `analyze-local.sh` runs the CLI anonymously.

## Step 1 -- check out the pre-exclusion state

PR #22423 introduced the exclusion in commit `3a54c9afbc`. The parent commit `d7791e6838` is the "before" state.

```bash
git checkout d7791e6838 -- .codacy.yml      # restore the pre-exclusion .codacy.yml
# (do NOT switch branches; just stage the older .codacy.yml)
```

## Step 2 -- run analyze-local on markdownlint only

```bash
.agents/skills/triage-codacy/scripts/analyze-local.sh --tool markdownlint
```

Expected: a SARIF dump under `<repo>/.local/audits/codacy/local-markdownlint-<ts>-<pid>.sarif`. The CLI returns non-zero when findings exist (this is normal; the script tolerates it).

## Step 3 -- count findings

```bash
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.sarif | head -1)"
jq '
    [.runs[]?.results[]?] | length
' "$DUMP"
```

Expected: a count close to 864 (within ~10% tolerance for tool-version drift between the CLI bundle and Codacy Cloud).

## Step 4 -- restore the exclusion

```bash
git checkout HEAD -- .codacy.yml
```

## Step 5 -- re-run and confirm zero on excluded paths

```bash
.agents/skills/triage-codacy/scripts/analyze-local.sh --tool markdownlint
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.sarif | head -1)"
jq '[.runs[]?.results[]? | select(.locations[]?.physicalLocation.artifactLocation.uri | startswith(".agents/") or startswith("docs/netdata-ai/skills/"))] | length' "$DUMP"
```

(The exact jq filter depends on the dump shape -- consult the dump structure first via `jq 'keys' "$DUMP"`.)

Expected: zero rows in the excluded trees.

## What this validates

- `analyze-local.sh` runs end-to-end against the installed Codacy CLI v2.
- The CLI honours `.codacy.yml` exclude_paths (or, if it doesn't, we have empirical evidence to handle that gap in a GitHub issue or branch-local SOW).
- The configured markdownlint tool produces a count that can be compared with what Codacy CI reports.

## Troubleshooting

- **First run installs tools**: Codacy CLI v2 bootstraps `.codacy/codacy.yaml` and installs its configured analyzers. Keep the installed CLI and tool versions pinned when comparing results.
- **Different count than 864**: tool-version drift between the installed Codacy CLI v2 analyzers and Codacy Cloud is normal. Check the CLI version and `.codacy` tool configuration before comparing counts.
- **CLI exits with non-zero**: that's expected when findings are present. The script preserves the SARIF dump and validates its result count.
