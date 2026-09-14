# Triage `action_required` Without A Codacy Token

Use this when the GitHub check-run says Codacy is `action_required`, but
GitHub exposes no annotations and the local checkout has no `.env` with
`CODACY_TOKEN`.

1. Get the Codacy check-run summary from GitHub:

   ```bash
   gh api repos/netdata/netdata/check-runs/<check-run-id> \
     --jq '{conclusion:.conclusion, output:.output, details_url:.details_url}'
   ```

2. If a live lookup is within scope, try the public Codacy v3 endpoint. Anonymous availability is service-dependent;
   failure is an evidence gap, not an empty result. Use a fresh private capture directory:

   ```bash
   mkdir -p .local/audits/codacy
   codacy_capture="$(mktemp -d .local/audits/codacy/public-issues.XXXXXX)"
   chmod 700 "$codacy_capture"
   curl -fsS \
     "https://api.codacy.com/api/v3/analysis/organizations/gh/netdata/repositories/netdata/pull-requests/<PR>/issues?limit=100" \
     -o "$codacy_capture/issues.json"
   ```

3. After a successful fetch, inspect added issues. This is one page: follow `pagination.cursor` before claiming
   complete coverage. Verify each candidate against the current source and current-head gate:

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
   ' "$codacy_capture/issues.json"
   ```

4. `deltaType == "Fixed"` records reflect Codacy's resolved classification. Preserve commit provenance and
   investigate conflicting current-head evidence; an older anchor alone does not prove resolution.

Safety note: the public response can contain `commitInfo` fields with personal
metadata. Keep raw dumps under `.local/audits/codacy/`, which is gitignored,
and do not copy names or email addresses into committed artifacts.
