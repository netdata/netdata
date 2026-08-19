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

Safety note: the public response can contain `commitInfo` fields with personal
metadata. Keep raw dumps under `.local/audits/codacy/`, which is gitignored,
and do not copy names or email addresses into committed artifacts.
