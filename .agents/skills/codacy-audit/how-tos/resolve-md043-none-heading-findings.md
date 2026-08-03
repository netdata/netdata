# Resolve MD043 findings with an empty heading sequence

Use this when Codacy reports one issue per new Markdown file with this shape:

```text
markdownlint_MD043: Expected: [None]; Actual: # Document title
```

## Root cause

Codacy Cloud has enabled markdownlint `MD043` with an empty required heading sequence. The analyzer therefore rejects the
first heading even when the document intentionally has a normal hierarchy. Removing headings would satisfy the configured
rule while degrading the document, so it is not the correct repository-level fix.

## Procedure

1. Fetch the current PR issues and confirm that every relevant finding is `markdownlint_MD043` with `Expected: [None]`:

   ```bash
   bash .agents/skills/codacy-audit/scripts/pr-issues.sh <PR_NUMBER> --by file
   jq -r '.data[] | [.commitIssue.filePath, .commitIssue.patternInfo.id, .commitIssue.message] | @tsv' \
     .local/audits/codacy/pr-<PR_NUMBER>-<TIMESTAMP>.json
   ```

2. Keep the document headings. Add `MD043` to a markdownlint directive before the first heading:

   ```markdown
   <!-- markdownlint-disable MD013 MD043 -->

   # Document title
   ```

   Use `markdownlint-disable-file` when the file already follows that repository pattern. A directive after the first
   heading is too late for the reported violation.

3. If the Markdown file is covered by an integrity manifest, refresh its hash and byte count.

4. Run the local markdownlint analyzer and verify the affected paths have no findings:

   ```bash
   bash .agents/skills/codacy-audit/scripts/analyze-local.sh --tool markdownlint
   ```

5. After pushing, fetch the PR issues again. Codacy's API can lag behind the check run, so compare the finding commit SHA
   with the current PR head before treating a residual result as current.

This workflow handles only the empty-sequence `MD043` configuration. Other heading-structure messages require independent
analysis rather than a blanket suppression.
