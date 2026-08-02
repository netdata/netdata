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

   This happens when the outer Codacy CLI uses the host Docker socket but writes
   `codacy-config*.json` only inside its private `/tmp`. The child-container bind
   is resolved by the host daemon; because the host path does not exist, Docker
   creates a directory there. The repository wrapper avoids this by setting
   Java's temp directory to a host-backed directory mounted at the same absolute
   path in the CLI container.

   Verify that contract before changing analyzer configuration:

   ```bash
   grep -nE 'JAVA_TOOL_OPTIONS|RUNNER_TMP.*RUNNER_TMP' \
     .agents/skills/codacy-audit/scripts/analyze-local.sh
   ```

   The wrapper now exits nonzero when JSON or SARIF is malformed or has the wrong
   top-level report shape. A parseable error object is not a report: JSON output
   must be the CLI's result array, while SARIF must declare version 2.1.0 and
   contain a `runs` array. A printed dump path without a successful exit is not a
   pass.

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
