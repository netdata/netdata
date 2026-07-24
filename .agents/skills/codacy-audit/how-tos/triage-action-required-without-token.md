# Triage `action_required` Without A Codacy Token

Use this when the GitHub check-run says Codacy is `action_required`, but
GitHub exposes no annotations and the local checkout has no `.env` with
`CODACY_TOKEN`.

1. Get the Codacy check-run summary from GitHub:

   ```bash
   gh api repos/netdata/netdata/check-runs/<check-run-id> \
     --jq '{conclusion:.conclusion, output:.output, details_url:.details_url}'
   ```

2. Fetch public PR issue details from Codacy v3:

   ```bash
   curl -fsS \
     "https://api.codacy.com/api/v3/analysis/organizations/gh/netdata/repositories/netdata/pull-requests/<PR>/issues?limit=100" \
     -o .local/audits/codacy/pr-<PR>-public-issues.json
   ```

3. Show only still-blocking issues:

   ```bash
   jq -r '
     .data[]
     | select(.deltaType == "Added")
     | [
         .commitIssue.filePath,
         .commitIssue.lineNumber,
         .commitIssue.patternInfo.id,
         .commitIssue.message,
         .commitIssue.lineText
       ]
     | @tsv
   ' .local/audits/codacy/pr-<PR>-public-issues.json
   ```

4. Ignore `deltaType == "Fixed"` entries for the current blocker. They are
   historical issue records that Codacy already considers resolved.

5. Validate constant-condition reports against test intent. Cppcheck's
   `knownConditionTrueFalse` pattern can legitimately identify a compile-time
   result in a unit-test assertion whose purpose is to prove exactly that
   result, such as verifying that a zero timeout disables expiration. This is
   not a runtime defect; either mark the finding false positive or use a
   semantically equivalent analyzer-friendly test shape if the PR gate cannot
   accept administrative triage.

Safety note: the public response can contain `commitInfo` fields with personal
metadata. Keep raw dumps under `.local/audits/codacy/`, which is gitignored,
and do not copy names or email addresses into committed artifacts.
