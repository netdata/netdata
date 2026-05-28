# How-to: reproduce PR #22423's 864 markdownlint findings locally

## When to use

You want to confirm `analyze-local.sh` matches what Codacy CI reported on a known fixture. PR #22423 is the canonical fixture for this skill: its first CI run reported **864 markdownlint findings** (recorded in the SOW that built this skill, SOW-0011); commit `3a54c9afbc` cleared them by adding `.agents/**` and `docs/netdata-ai/skills/**` to `.codacy.yml`.

This how-to walks through reproducing those 864 findings on the pre-exclusion state, then confirming the post-exclusion state shows zero on the affected files.

## Prerequisite

- `docker` available (or `codacy-analysis-cli` installed locally).
- `<repo>/.env` need NOT contain `CODACY_TOKEN` -- `analyze-local.sh` runs the CLI anonymously.

## Step 1 -- check out the pre-exclusion state

PR #22423 introduced the exclusion in commit `3a54c9afbc`. The parent commit `d7791e6838` is the "before" state.

```bash
git checkout d7791e6838 -- .codacy.yml      # restore the pre-exclusion .codacy.yml
# (do NOT switch branches; just stage the older .codacy.yml)
```

## Step 2 -- run analyze-local on markdownlint only

```bash
.agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint
```

Expected: a JSON dump under `<repo>/.local/audits/codacy/local-markdownlint-<ts>.json`. The CLI returns non-zero when findings exist (this is normal; the script tolerates it).

## Step 3 -- count findings

```bash
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.json | head -1)"
jq '
    if type=="array" then length
    elif type=="object" and has("issues") then (.issues | length)
    elif type=="object" and has("results") then (.results | length)
    else 0 end
' "$DUMP"
```

Expected: a count close to 864 (within ~10% tolerance for tool-version drift between the CLI bundle and Codacy Cloud).

## Step 4 -- restore the exclusion

```bash
git checkout HEAD -- .codacy.yml
```

## Step 5 -- re-run and confirm zero on excluded paths

```bash
.agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.json | head -1)"
jq '[ ... | select(.filePath | startswith(".agents/") or startswith("docs/netdata-ai/skills/")) ] | length' "$DUMP"
```

(The exact jq filter depends on the dump shape -- consult the dump structure first via `jq 'keys' "$DUMP"`.)

Expected: zero rows in the excluded trees.

## What this validates

- `analyze-local.sh` runs end-to-end against the configured runner (docker or local binary).
- The CLI honours `.codacy.yml` exclude_paths (or, if it doesn't, we have empirical evidence to handle that gap in a future SOW).
- The bundled markdownlint version produces a count consistent with what Codacy CI reports.

## Troubleshooting

- **Docker pull is slow on first run**: `codacy/codacy-analysis-cli:latest` is a few hundred MB. Subsequent runs use the warm cache.
- **Different count than 864**: tool-version drift between the bundled `codacy-analysis-cli` and Codacy Cloud is normal. ~10% tolerance is fine. Anything wider warrants checking the CLI version vs Cloud's reported version.
- **CLI exits with non-zero**: that's expected when findings are present. The script suppresses this; the JSON dump is still valid.
