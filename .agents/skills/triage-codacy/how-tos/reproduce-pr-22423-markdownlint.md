# How-to: reproduce PR #22423's 864 markdownlint findings locally

## When to use

You want to check whether the exclusions PR #22423 added still suppress markdownlint findings under the current local configuration. PR #22423 is the canonical fixture for this skill: its first CI run reported **864 markdownlint findings**; commit `3a54c9afbc` cleared them by excluding `.agents/**` and `docs/netdata-ai/skills/**` from analysis.

The CLI's markdownlint analyzer is its own bundled version, so treat the counts below as advisory, not as Codacy Cloud parity. For parity evidence, run markdownlint directly (see "Cloud parity" below).

## Prerequisite

- Codacy CLI v2 (`codacy-cli`) installed locally.
- A reviewed `<repo>/.codacy/codacy.yaml`. CLI v2 reads this file, not `.codacy.yml`, and `analyze-local.sh` never generates configuration.
- `<repo>/.env` need NOT contain `CODACY_TOKEN` -- `analyze-local.sh` runs the CLI anonymously.

## Step 1 -- apply the pre-exclusion state

The exclusions live in `.codacy/codacy.yaml`, so back up the reviewed config and delete the `.agents/**` and `docs/netdata-ai/skills/**` entries from the copy in place:

```bash
cp .codacy/codacy.yaml /tmp/codacy-reviewed.yaml    # restored in Step 4
# delete the two exclusion entries from .codacy/codacy.yaml, then confirm the
# diff touches only those lines before analysing
git diff -- .codacy/codacy.yaml
```

## Step 2 -- run analyze-local on markdownlint only

```bash
.agents/skills/triage-codacy/scripts/analyze-local.sh --tool markdownlint
```

Expected: a SARIF dump under `<repo>/.local/audits/codacy/local-markdownlint-<ts>-<pid>.sarif`. The CLI returns non-zero when findings exist (this is normal; the script tolerates it). Analysis is full-tree, so unchanged files are analysed too.

## Step 3 -- count findings

```bash
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.sarif | head -1)"
jq '
    [.runs[]?.results[]?] | length
' "$DUMP"
```

Expected: a count in the same order of magnitude as 864. Tool-version drift between the CLI bundle and Codacy Cloud is expected.

## Step 4 -- reset the config to the reviewed state

```bash
cp /tmp/codacy-reviewed.yaml .codacy/codacy.yaml
git diff --quiet -- .codacy/codacy.yaml
```

Reset the config between fixture states. The CLI reads `.codacy/codacy.yaml` only, so a stale pre-exclusion copy silently changes what the second run analyses -- and it will not show up as a `.codacy.yml` diff.

## Step 5 -- re-run and confirm zero on excluded paths

```bash
.agents/skills/triage-codacy/scripts/analyze-local.sh --tool markdownlint
DUMP="$(ls -1t .local/audits/codacy/local-markdownlint-*.sarif | head -1)"
jq '[.runs[]?.results[]? | select(.locations[]?.physicalLocation.artifactLocation.uri | startswith(".agents/") or startswith("docs/netdata-ai/skills/"))] | length' "$DUMP"
```

Expected: zero rows in the excluded trees. Check one sample URI first -- a `./`-prefixed, absolute, or `file://` URI makes the prefix filter match nothing and prints a false zero:

```bash
jq -r '.runs[0].results[0].locations[0].physicalLocation.artifactLocation.uri' "$DUMP"
```

## Cloud parity

The count above comes from the CLI's bundled markdownlint analyzer, not Codacy Cloud's. When the question is what Codacy reports, run the direct markdownlint linter over the same trees and compare that result instead, recording the ruleset and version used. The same applies to ShellCheck: use the direct linter with this repository's `.shellcheckrc`.

## What this validates

- `analyze-local.sh` runs end-to-end against the installed Codacy CLI v2 with a reviewed config.
- The CLI honours the exclusion entries in `.codacy/codacy.yaml`.
- The count is advisory evidence about the local markdownlint analyzer, not Codacy Cloud parity.

## Troubleshooting

- **First run installs tools**: Codacy CLI v2 installs its configured analyzers. Keep the installed CLI and tool versions pinned when comparing results.
- **Different count than 864**: tool-version drift between the installed CLI analyzers and Codacy Cloud is normal. Check the CLI version and `.codacy` tool configuration before comparing counts.
- **`no reviewed config`**: the script exits 2 when `.codacy/codacy.yaml` is missing. Run `codacy-cli init`, review the result, and re-run.
- **CLI exits with non-zero**: that's expected when findings are present. The script preserves the SARIF dump and validates its result count.
