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
   report shape. A parseable error object is not a report: JSON output must be the
   CLI's array of exactly-one-key `Issue`, `DuplicationClone`, `FileError`, or
   `FileMetrics` wrappers with their required payload fields. SARIF must use the
   2.1.0 schema and version, and every run must contain a valid tool driver and
   rules, results with valid rule and artifact references, exactly one successful
   invocation, and artifacts. The wrapper reports each JSON result class
   separately and fails when a `FileError` makes the analysis incomplete. A
   printed dump path without a successful exit is not a pass.

   The wrapper passes `--fail-if-incomplete` and accepts only runner status 0
   (complete within the issue threshold) or 102 (complete with findings). It
   rejects status 101, which means one or more tools failed, before trusting an
   otherwise valid-looking report.

   This is a closed 7.10.1 compatibility contract, not a generic SARIF reader.
   The Docker runner is pinned to the 7.10.1 Linux/amd64 manifest digest, and the
   local runner rejects any other reported CLI version before initializing the
   output path. Upgrade the image version and digest, accepted local version,
   validator, and mock matrix together after checking the new stable release's
   upstream model and real output. Runner diagnostics stay on stderr; inspect
   them before trusting a malformed dump.

   Codacy 7.10.1 has one known SARIF quirk: in a mixed-category run, a
   non-security result's rule index addresses the non-security subset even
   though the combined rule list starts with security rules. The wrapper accepts
   only an in-range index and treats the existing unique rule ID as authoritative.
   It cannot reconstruct the hidden partition from the serialized category,
   because the CLI itself treats that field as unreliable.

   When `--output` is explicit, use a writable regular-file path under an
   existing parent directory. The wrapper rejects directories, must truncate
   the destination before starting the analyzer, and requires a non-empty
   readable regular file after the analyzer returns. If initialization fails,
   it stops instead of trusting stale report content.

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
