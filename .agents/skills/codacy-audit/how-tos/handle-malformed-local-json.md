# Handle Malformed Local Codacy JSON

Use this when `analyze-local.sh` writes a `local-*.json` file but `jq` cannot
parse it.

1. Verify the dump before reading it as findings:

   ```bash
   jq empty .local/audits/codacy/local-*.json
   ```

2. If parsing fails, inspect the first lines:

   ```bash
   sed -n '1,80p' .local/audits/codacy/local-*.json
   ```

3. Treat tool-runner logs as a local-analysis failure, not as Codacy findings.
   A known failure mode is a Dockerized tool trying to read `/.codacyrc` as a
   file and reporting `read /.codacyrc: is a directory`.

4. Check GitHub check-run annotations:

   ```bash
   gh api repos/netdata/netdata/check-runs/<CHECK_RUN_ID>/annotations --paginate
   ```

5. If annotations are empty, fetch PR issues through the Codacy API:

   ```bash
   .agents/skills/codacy-audit/scripts/pr-issues.sh <PR_NUMBER>
   ```

6. If `CODACY_TOKEN` is not configured, record that Codacy details are not
   locally available and re-check after the next push.
