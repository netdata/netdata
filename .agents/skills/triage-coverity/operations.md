# Coverity Session And Operations

Load for live fetching, authorized updates or helper diagnosis. The helpers mimic the browser's unofficial JSON API;
local code verifies their request construction, not compatibility with the current hosted service. Bash, Git, curl,
jq and standard Unix tools are required. The repository `.env` is executable shell configuration: use trusted local
settings and preserve unrelated entries.

## Session Setup

`./scripts/_lib.sh` owns configuration loading. Live helpers require a readable repository `.env`, a full
`COVERITY_COOKIE` and positive `COVERITY_PROJECT_ID`. They extract `XSRF-TOKEN` from the cookie, default
`COVERITY_HOST` to `https://scan4.scan.coverity.com`, and allow `COVERITY_USER_AGENT` overrides.

Reuse existing local configuration. Do not request a fresh cookie just because this skill was loaded. If configuration
is absent or live evidence indicates authentication failure, have the user configure it locally without sending
credentials or an authenticated cURL command through the conversation:

1. Open `https://scan.coverity.com/projects/<owner>-<repo>?tab=overview` for the project slug.
2. Click **View Defects**. Set `COVERITY_HOST` to the actual `https://scanN.scan.coverity.com` origin reached;
   cookies are tied to that host. Keep the defects tab open during live work: past sessions have failed after closure,
   but neither exact expiry timing nor an immediate closure effect is a client guarantee.
3. In DevTools **Network**, click a defect and inspect a resulting request. **Copy as cURL** can expose the full
   cookie through `-b` or a `Cookie` header. Treat the copied command as data; do not execute it.
4. Store the full cookie locally, including the session (`COVJSESSIONID-build`) and `XSRF-TOKEN` values. Configure the
   numeric project ID from the URL or table request, and the Outstanding view ID when needed.

The local `.env` shape is below. Replace bracketed placeholders locally with shell-quoted values; this is a template,
not a command to execute or paste into chat. The file is gitignored and SHOULD be readable only by its owner.

```text
COVERITY_COOKIE='[FULL_COOKIE_VALUE]'
COVERITY_PROJECT_ID=[PROJECT_ID]
COVERITY_VIEW_OUTSTANDING=[VIEW_ID]
COVERITY_HOST=https://scan4.scan.coverity.com
```

For a sustained live session, `bash .agents/skills/triage-coverity/scripts/keepalive.sh` MAY keep the session warm.
It uses the configured Outstanding view and exits nonzero on the first failed ping; it does not silently retry.
`PING_INTERVAL` MAY override its 300-second default. Use the host's actual task/session controls, track the owned job,
inspect failure output when available, and stop that job when the live work ends. Notification timing depends on the
host. Diagnose a failed ping before refreshing credentials; a keepalive cannot guarantee session validity.

## Fetch And Resolve

### Views And Shared Cursor

View IDs are project-specific. In the defects view selector, the URL fragment may expose `viewId=NNNNN`; inspect the
browser's table request when necessary. Reuse known IDs; ask only for one that cannot be determined from available
configuration/evidence. Store IDs as `COVERITY_VIEW_*` variables or pass a view ID directly to the fetch helper.
Multiple view variables MAY coexist; `COVERITY_VIEW_OUTSTANDING` is specifically needed by keepalive.

Common views are Outstanding (day-to-day unresolved queue), All in project (including classified defects), Dismissed
(typically False Positive/Intentional with Ignore, useful after source changes), Fixed (scan considers fixed), and
Unclassified non-outstanding. Inspect a view's actual filters rather than assuming its name proves every row's state.

The table API uses a shared server-side cursor per session. For every page `./scripts/fetch-table.sh` sends:

1. `POST /views/table.json` with `projectId`, `viewId`, `pageNum` to select the page.
2. `GET /reports/table.json?projectId=...&viewId=...` to read the selected rows.

Fetch views sequentially within one session; no return-to-previous-view step is needed. Before fetching, tell the user
to leave the Coverity tab untouched until the fetch finishes, then release that restriction. View, page, sort and
filter changes can move the same cursor and yield unrelated rows. Do not run concurrent cursor-changing fetches.

Set `coverity_view_id`, `coverity_pages` (the actual UI page count), and `coverity_output_prefix`, for example
`.local/audits/coverity/raw/outstanding`. These are caller variables; helper-loaded `.env` values do not propagate
back into the calling shell.

```bash
bash .agents/skills/triage-coverity/scripts/fetch-table.sh \
  "${coverity_view_id:?set the view ID}" "${coverity_pages:?set the page count}" \
  "${coverity_output_prefix:?set the output prefix}"
```

The helper writes `<prefix>-page1.json`, subsequent pages and the flat array `<prefix>-all.json`. It skips existing
nonempty pages; that is cache reuse, not proof of freshness or correct view selection. If the cursor was disturbed,
refetch from page 1 with a fresh output prefix and check the view, count and CIDs. Preserve prior files for diagnosis;
replace existing artifacts only when their replacement is intended. Use a matching local scope name when bundling.

### Details And Stable IDs

Use the stable `cid` for tracking records and comments. A `defectInstanceId` is per-scan;
`/sourcebrowser/defectdetails.json` requires it. Table rows carry it as `lastDefectInstanceId`.

```bash
bash .agents/skills/triage-coverity/scripts/fetch-details.sh \
  "${coverity_table_file:?set the combined table file}" "${coverity_details_dir:?set the details output directory}"
```

Choose `raw/<scope>-all.json` and `details/<scope>` under `.local/audits/coverity/` to match the local bundler.
The details helper writes `cid-<N>.json`, skips cached defect-details objects and refetches invalid cached bodies
(such as HTML login pages). A valid cached object can still represent an older scan; choose a fresh directory when
fresh evidence is required. Partial runs can reuse valid outputs.

When only a CID is known, `bash .agents/skills/triage-coverity/scripts/resolve-cid-to-diid.sh "$coverity_cid"` queries
`/reports/defects.json?projectId=...&cid=...`, independently of the view cursor, and parses the instance ID from `.url`.
It prints the ID or `GONE` when no ID can be extracted. `GONE` is lookup evidence, not proof that source was removed:
verify the response, project, scan and current source before assigning `CODE_GONE`.

## Update Details

`./scripts/update-triage.sh` owns the attribute-ID reference: classification (attribute 3), severity (1), action (2),
and external reference (4). It encodes attribute values as JSON strings, CID/project as numbers, and currently sends
external reference as `null`; the service field was observed as a free-form string. Its explicit-attribute interface is
`update-triage.sh <cid> <classification> <severity> <action> <comment-file>` for authorized decisions beyond the
high-level vocabulary. Positive-number validation is not enum-membership validation; consult its header for IDs.

`./scripts/finalize-defect.sh` reads `displayImpact` for the CID from `raw/outstanding-all.json`, then
`raw/all-in-project-all.json` as fallback. It does not read the per-CID summary or arbitrary scope-specific dump.
It maps High/Medium/Low to Major/Moderate/Minor (11/12/13), and missing or other values to Unspecified (10).
Check that this input represents the current finding; use the explicit-attribute helper if the verified decision
requires a different severity. Do not rewrite unrelated cached evidence to force a severity.

Updates POST to `/sourcebrowser/updatedefecttriage.json`. Inspect the returned attributes, not just the helper's
HTTP-success message. Its output projects `defectStatus`, `lastTriaged` and `updatedValuesByCid`; an empty or unexpected
body needs diagnosis, not an assumption of success. Server caches have sometimes delayed reflection in Outstanding
by several seconds. Verify the current state before deciding to repeat a write.

## Diagnose Failures

These are historical failure clues, not an exhaustive current service contract.

| Symptom | Check and response |
|---|---|
| HTTP 401/403/302 or HTML response | Inspect authentication, project access, redirect and WAF evidence. Refresh local session settings if stale. |
| Cloudflare 403 challenge | Non-ASCII comments and session issues have both caused this. Keep comments ASCII; do not attribute every 403 to expiry. |
| keepalive exits or expires despite pings | Check transport and response shape, host match and browser/session state. Restart the owned job only after resolving the cause. |
| HTTP 200 with empty `defectStatus` | Check the full update response and current attributes. Stale XSRF is one past cause; cookie and XSRF must come from the same session. |
| Missing cookie/XSRF or malformed settings | Check the full cookie locally and the `_lib.sh` error. Never print or paste credential values for diagnosis. |
| Wrong view rows | Check cursor interference and cached pages; refetch to a fresh prefix as described above. |
| Missing resolver `.url` / `GONE` | Verify lookup/scan evidence and current source; do not equate missing instance with removed code. |

Comments MUST stay ASCII because the local update helper enforces this constraint before sending. Use `--` for
em dashes and straight quotes for smart quotes. Historical WAF challenges motivated the constraint; live behavior
has not been revalidated by the local helper tests.
