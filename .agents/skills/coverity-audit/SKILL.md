---
name: coverity-audit
description: Triage Coverity Scan defects (https://scan.coverity.com) for this project — fetch defect lists, fetch per-defect details, and apply triage decisions (Bug / FalsePositive / Intentional with severity, action, and a comment). Use when the user asks to "review Coverity defects", "triage Coverity findings", "fetch Coverity outstanding", or anything mentioning Coverity Scan, CIDs, or scan.coverity.com.
---

# Coverity Scan triage skill

This skill drives the Coverity Scan unofficial JSON API (the public site has
no documented API — the scripts mimic what the browser does) for the project
configured in `.env`. Scripts auto-detect the repo root and write all
artifacts under `<repo-root>/.local/audits/coverity/`.

The skill captures **operational knowledge** (how the API works, where it
trips up, what the data means). It does NOT prescribe a review pipeline —
how you actually triage each defect (single model, multi-model, manual) is
adhoc; agree the approach with the user up front.

## MANDATORY — keep this skill alive

If you (the agent) discover a new pattern, gotcha, working flow, correction,
or any piece of knowledge while running this skill — update this `SKILL.md`
AND commit it BEFORE proceeding. Knowledge that isn't committed is lost.

Examples of things to capture:
- New view IDs encountered (and what each represents)
- New API endpoint or parameter behavior
- New failure mode (Cloudflare quirks, session expiry signals, rate limits)
- New FP guardrail you found via the codebase (idiom that Coverity mismodels)
- New severity / classification mapping detail learned the hard way

---

## MANDATORY — startup sequence when this skill is invoked

Do these steps in this order. Skipping any step costs the user their session.

### Step 1 — agree the triage approach with the user

Coverity reviews are usually 1-3 defects. Sometimes hundreds. Ask the user:

- "How many defects are we looking at — one specific CID, or a sweep?"
- "How do you want to review them — you read each one, you spawn one agent
  per defect, you want me to use multiple models for cross-checking, …?"

Do NOT default to a heavy multi-stage pipeline; that's only worth setting up
when there are tens of defects to crunch. For small batches a single agent
or the user reading the bundle directly is usually faster.

If the user does want a multi-model approach, a sensible (not prescribed)
shape is: cheap models surface ideas, the strongest coding model produces
the actual analysis + fix, the strongest reviewing model sanity-checks the
result. The user picks the actual CLIs/models — this skill does not assume
any specific tool.

### Step 2 — ask the user for a fresh cookie

Coverity Scan auth is cookie-based and tied to a live browser session. Give
the user the exact recipe:

> 1. Open https://scan.coverity.com/projects/<owner>-<repo>?tab=overview
>    (replace `<owner>-<repo>` with the project slug, e.g. `netdata-netdata`).
> 2. Click **"View Defects"** at the top right -- a new tab opens to
>    https://scan4.scan.coverity.com/# (or `scanN.scan.coverity.com` --
>    Coverity load-balances; use whichever URL you land on as `COVERITY_HOST`).
> 3. **Keep that tab open the whole time we work.** Closing it kills the
>    session immediately.
> 4. Press F12 to open DevTools, switch to the Network tab.
> 5. Click any defect in the list -- 3-4 requests appear in the Network tab.
> 6. Right-click any of those requests, select Copy > **Copy as cURL**.
> 7. Paste the entire curl command back to me.

If the curl the user pastes does NOT contain a `-b 'cookie=...'` line, ask
again -- they probably picked "Copy as fetch" or "Copy as Node.js fetch".

### Step 3 — save the cookie in `.env`

Extract the value of the `-b` argument from the user's curl and write/update
`<repo-root>/.env` with:

```bash
COVERITY_COOKIE='<paste-the-entire-cookie-blob-here>'
COVERITY_PROJECT_ID=<numeric projectId from the URL or table.json query>
COVERITY_VIEW_OUTSTANDING=<viewId of the Outstanding view>
COVERITY_HOST=https://scan4.scan.coverity.com
```

`.env` is gitignored. The cookie blob must include both `COVJSESSIONID-build`
and `XSRF-TOKEN`. The scripts extract `XSRF-TOKEN` automatically.

### Step 4 — start the keepalive (background)

```
Bash tool with run_in_background=true,
command="bash .agents/skills/coverity-audit/scripts/keepalive.sh"
```

The keepalive **exits non-zero the moment a ping fails** (session expired,
browser tab closed, cookie went bad). The orchestrator's background-task
notification fires immediately so you know to ask for a fresh cookie and
restart.

**Stop the keepalive at the end of the triage session** by killing its
background task.

### Step 5 — proceed with the actual triage work

Now (and only now) is it safe to fetch tables, fetch details, prepare
defect bundles, finalize verdicts.

---

## CRITICAL — server-side view state

Coverity's table API has a **stateful, server-side view cursor**. The
`/views/table.json` POST changes server-state (which page is "current"),
then the `/reports/table.json` GET reads whatever the current state is.

This has two consequences:

### 1. Pagination is two-step, not one-shot

Per page:
1. `POST /views/table.json {projectId, viewId, pageNum}` -- moves the cursor
2. `GET /reports/table.json?projectId=...&viewId=...` -- reads the page

`fetch-table.sh` handles both steps and the page loop.

### 2. **The user MUST NOT touch the Coverity UI while a fetch is running**

If the user clicks a different view, scrolls to a different page, sorts a
column, applies a filter — the server-side cursor moves under our scripts'
feet. The scripts will then fetch garbage (rows from whatever view the user
just opened, not the view the script asked for).

Tell the user explicitly: "I'm about to fetch the defect list. Don't click
in the Coverity tab until I say I'm done."

If the user accidentally interferes, re-run the fetch script — it's
idempotent on cached pages but not on already-corrupted ones, so delete the
output JSONs and re-fetch from page 1.

---

## Coverity views (queue scopes)

Coverity organizes defects into named **views**, each with a numeric
`viewId`. The skill operates per-view.

### How to find a view's ID

In the Coverity UI:
- Click the project's defects view, then the view selector dropdown.
- Each view has a URL like `https://scan4.scan.coverity.com/#viewId=NNNNN`.
- That `NNNNN` is the value to put in `.env` (or pass to `fetch-table.sh`).

### Common view types you'll encounter

The default project ships with these (the IDs are project-specific):

- **Outstanding** — the live queue: defects neither classified nor dismissed.
  Where day-to-day triage happens. Save its viewId as
  `COVERITY_VIEW_OUTSTANDING` (also used by the keepalive).
- **All in project** — every defect, including already-classified ones.
  Useful for batch rescans or full audits.
- **Dismissed** — defects classified as False Positive / Intentional and
  ignored. Useful for re-review when the underlying code changed.
- **Fixed** — defects Coverity now considers fixed. Verification queue.
- **Unclassified non-outstanding** — corner cases.

When a user asks you to operate on a non-default view, ask them to give you
its viewId from the UI. You can have multiple `COVERITY_VIEW_*` env vars.

### Switching views without confusing the cursor

The view-state cursor described above is **per-session**. If you want to
fetch view A then view B, do them sequentially (not concurrently): each
`fetch-table.sh` call sends its own POST that resets the cursor. There is
no need to "go back" -- just call it again with the next viewId.

---

## Coverity attribute reference

Triage values are sent as integer **attribute IDs**, not names:

### Classification (attribute 3)

| ID | Name           |
|----|----------------|
| 20 | Unclassified   |
| 21 | Pending        |
| 22 | False Positive |
| 23 | Intentional    |
| 24 | Bug            |

### Severity (attribute 1)

| ID | Name        |
|----|-------------|
| 10 | Unspecified |
| 11 | Major       |
| 12 | Moderate    |
| 13 | Minor       |

### Action (attribute 2)

| ID | Name              |
|----|-------------------|
|  1 | Undecided         |
|  2 | Fix Required      |
|  3 | Fix Submitted     |
|  4 | Modeling Required |
|  5 | Ignore            |

### External reference (attribute 4)

Free-form string. The scripts always send `null`.

### Sensible (verdict → attributes) mappings

These are what `finalize-defect.sh` applies when given a verdict from the
suggested vocabulary below:

| Outcome           | classification (3) | action (2) |
|-------------------|--------------------|------------|
| Real bug, fixed   | 24 Bug             | 3 Fix Submitted |
| False positive    | 22 False Positive  | 5 Ignore       |
| Cosmetic / harmless | 23 Intentional   | 5 Ignore       |

### Coverity displayImpact → severity ID

`finalize-defect.sh` maps `defect-summary.json#displayImpact` to severity:

| displayImpact | severity ID | name        |
|---------------|-------------|-------------|
| `High`        | 11          | Major       |
| `Medium`      | 12          | Moderate    |
| `Low`         | 13          | Minor       |
| `null` / missing | 10       | Unspecified |

---

## Suggested verdict vocabulary

This is the taxonomy used historically; you can reuse it or replace it. The
`finalize-defect.sh` script understands these names directly:

### Real bugs (classification = 24 Bug, action = 3 Fix Submitted)

| Verdict                       | When to use                                                          |
|-------------------------------|----------------------------------------------------------------------|
| `TRUE_BUG_MEMORY_CORRUPTION`  | OOB read/write, UAF, double-free, type confusion, stack-escape UAF.  |
| `TRUE_BUG_CRASH`              | Reachable NULL deref / div-by-zero / assert / fatal. No corruption.  |
| `TRUE_BUG_RESOURCE_LEAK`      | fd / memory / lock / refcount leak that accumulates on a reachable path. |
| `TRUE_BUG_LOGIC`              | Wrong result, wrong metric, wrong stored/transmitted data. No crash. |
| `TRUE_BUG_UB`                 | Spec-UB (signed overflow, strict aliasing) compiles today but latent.|

### False positives (classification = 22 FP, action = 5 Ignore)

| Verdict                          | When to use                                                  |
|----------------------------------|--------------------------------------------------------------|
| `FALSE_POSITIVE_GUARD_EXISTS`    | The code has a check Coverity failed to track.               |
| `FALSE_POSITIVE_UNREACHABLE`     | The flagged path cannot be reached under any realistic state.|
| `FALSE_POSITIVE_TRUSTED_INPUT`   | Tainted source is actually trusted (root-owned local file).  |
| `FALSE_POSITIVE_TOOL_MODEL`      | Coverity's semantic model is wrong (e.g. doesn't know `mallocz` cannot return NULL). |
| `IMPOSSIBLE_CONDITIONS`          | Combination of states existing invariants prevent.           |

### Cosmetic (classification = 23 Intentional, action = 5 Ignore)

| Verdict     | When to use                                                  |
|-------------|--------------------------------------------------------------|
| `COSMETIC`  | Not a bug: unused value, dead branch, redundant expression.  |

### Bookkeeping (no Coverity update)

| Verdict       | When to use                                                            |
|---------------|------------------------------------------------------------------------|
| `CODE_GONE`   | The flagged file/function no longer exists. (`finalize-defect.sh` skips.) |
| `NEEDS_HUMAN` | Genuinely unsure after reasonable investigation. (`finalize-defect.sh` skips.) |

---

## Per-defect workdir convention

`prepare-defect.sh` creates a per-defect directory under
`.local/audits/coverity/triage/<scope>/cid-<N>/` with:

- `defect-summary.json` — the row from the table dump
- `defect-details.json` — the per-defect details (event trace, CWE, checker)
- `source-context.c` — ~150 lines of source around the main event
- `TODO.md` — per-defect notes; keep all per-defect artifacts inside

Whatever review approach the user picks, write its outputs here too (e.g.
analyzer reports, decider reasoning, commit message draft, build-verify
log). Don't pile per-defect stuff at the repo root.

---

## CID vs defectInstanceId

Two IDs for one defect, both flying around the API:

- **`cid`** is stable across scans. Use it for tracking, comments, your
  permanent records.
- **`defectInstanceId`** is per-scan. The `defectdetails.json` endpoint
  takes a `defectInstanceId`, NOT a `cid`. Each `table.json` row carries the
  current `lastDefectInstanceId` for its CID.

When all you have is a CID (e.g. you're acting on an old list, or working
from external triage notes), use:

```
bash .agents/skills/coverity-audit/scripts/resolve-cid-to-diid.sh <cid>
# prints the current defectInstanceId, or "GONE" if Coverity no longer reports it
```

Internally it queries `/reports/defects.json?cid=N` and parses
`defectInstanceId` out of the returned `.url` field.

---

## Workflow

### Fetch a view's table

```
bash .agents/skills/coverity-audit/scripts/fetch-table.sh \
    "${COVERITY_VIEW_OUTSTANDING}" 7 .local/audits/coverity/raw/outstanding
```

Produces `.local/audits/coverity/raw/outstanding-page1.json` ... and a
combined flat array at `.local/audits/coverity/raw/outstanding-all.json`.
Pass the right page count (visible in the UI) for the view.

**Reminder**: the user must not touch the UI during a fetch.

### Fetch per-defect details

```
bash .agents/skills/coverity-audit/scripts/fetch-details.sh \
    .local/audits/coverity/raw/outstanding-all.json \
    .local/audits/coverity/details/outstanding
```

One file per CID at `<details>/cid-<N>.json`. Idempotent.

### Bundle a defect for review

```
bash .agents/skills/coverity-audit/scripts/prepare-defect.sh <CID> outstanding
```

Creates `.local/audits/coverity/triage/outstanding/cid-<N>/` with the bundle.
The actual review (single-model, multi-model, human) is **adhoc** — agree
the approach with the user.

### Apply a verdict

After review and (if needed) a fix commit:

```
bash .agents/skills/coverity-audit/scripts/finalize-defect.sh \
    <CID> <VERDICT> <scope> .local/audits/coverity/triage/<scope>/cid-<N>/comment.txt \
    [<commit-sha>]
```

`<scope>` is the same name `prepare-defect.sh` uses (e.g. `outstanding`,
`dismissed`, `fixed`, `unclassified`).

This:
- Skips silently for `NEEDS_HUMAN` and `CODE_GONE`.
- Reads `displayImpact` from the table dump to derive severity.
- Appends `Fix commit: <sha>` to the comment when a SHA is given.
- Posts JSON to `/sourcebrowser/updatedefecttriage.json`.

For scopes other than "outstanding" (re-triaging dismissed/fixed/etc.),
finalize-defect prints a warning -- the caller is asserting that the new
verdict disagrees with the existing classification. It still applies.

---

## ASCII-only comments — non-negotiable

Coverity Scan sits behind Cloudflare. The WAF rejects bodies containing
non-ASCII bytes (em-dashes, smart quotes, accented letters) with a 403
Cloudflare challenge that looks like an expired-session error but isn't.

- Use `--` instead of em-dash (U+2014).
- Use straight quotes `"` `'` instead of smart quotes.
- The scripts reject non-ASCII before the network round-trip.

---

## Project-specific FP guardrails (Netdata)

Most of the cost of a Coverity audit is rejecting false positives. Before
calling something a bug in this codebase, rule out these idioms — Coverity
mis-models all of them:

1. **`z`-suffix allocators never fail.** `mallocz`, `callocz`, `reallocz`,
   `strdupz`, `strndupz`, `mallocz_flex` call `fatal()` on OOM. They cannot
   return NULL. Any "possible NULL deref after `mallocz`" warning is FP.
2. **`freez(NULL)` is safe.** Same for `string_freez`, etc.
3. **`DOUBLE_LINKED_LIST_*` macros** (`libnetdata/linked-lists.h`) manage
   prev/next with their own invariants. Raw pointer manipulation inside
   them is expected.
4. **`buffer_*` API** (`libnetdata/buffer/`) auto-grows the underlying
   `buffer->buffer[]` on write. Direct indexing of `buffer->buffer[N]` from
   callers is the risk, not the API.
5. **`STRING` is refcounted and interned** (`libnetdata/string/`).
   `string_strdupz` increments refcount on an existing intern;
   `string_dup` acquires a new reference. Mixing them is a refcount bug.
6. **`ARAL` is a slab allocator** (`libnetdata/aral/`). Objects are reused
   — UAF looks different here: the same address is re-issued. A pointer
   that "still works" after `aral_freez` may be a reused object.
7. **`DICTIONARY`** has `dictionary_acquired_item_get` / `_release` with
   refcounts. Raw access bypasses the refcount.
8. **Custom locks** (`spinlock_lock`, `rw_spinlock_*`) — not pthread.
   Coverity MISSING_LOCK warnings often misunderstand them.
9. **Platform**: glibc + musl. Watch glibc-only assumptions
   (e.g. `strerror_r` signature).
10. **Compilers**: gcc + clang both must compile.
11. **Process model**: long-running daemon, spawns plugin subprocesses over
    a line-based stdin/stdout protocol. Plugin stdin is typically trusted;
    streaming peers are UNTRUSTED remote peers over TCP.

---

## Input trust boundaries (Netdata)

Used to decide whether a tainted-data path is `FALSE_POSITIVE_TRUSTED_INPUT`:

| Source                       | Trust                          | Module                             |
|------------------------------|--------------------------------|------------------------------------|
| Local `/proc`, `/sys`        | Trusted (root-owned)           | `collectors/proc.plugin/`, `cgroups.plugin/` |
| Plugin stdin (our plugins)   | Trusted                        | `plugins.d/`, `collectors/*.plugin/` |
| Streaming peer (remote agent)| **UNTRUSTED**                  | `streaming/`, `stream-*`            |
| HTTP request (dashboard API) | Semi-trusted (usually localhost) | `web/api/`, `web/server/`         |
| MCP request                  | Semi-trusted (localhost)       | `web/mcp/`                          |
| Cloud (aclk)                 | Trusted (TLS + token to Netdata Cloud) | `aclk/`                    |
| Config files                 | Trusted (root-owned)           | `daemon/config/`, `health/`         |

---

## Failure modes — quick diagnosis

| Symptom                                              | Likely cause                                                |
|------------------------------------------------------|-------------------------------------------------------------|
| HTTP 401 / 403 / 302, response is HTML               | Session expired. Recapture cookie from browser.             |
| HTTP 403 with Cloudflare challenge HTML              | Either non-ASCII in comment, or browser tab closed.         |
| keepalive.sh exits non-zero                          | Same as above. Ask user for a fresh cookie.                 |
| HTTP 200 but `defectStatus` empty                    | XSRF token stale. Recapture cookie.                         |
| `Could not parse session from .env`                  | Wrong cookie format. Paste the FULL `-b 'k=v; ...'` string. |
| Fetched rows look wrong (different view)             | User clicked in the UI mid-fetch. Delete output JSONs and re-fetch. |
| `.url` field missing in `/reports/defects.json` reply | Coverity no longer reports this CID -- treat as `CODE_GONE`. |
| Cookie expires in mid-run despite keepalive          | Browser tab was closed. Reopen the tab and recapture cookie. |

---

## Recurring tips

- The `lastDefectInstanceId` field, NOT `cid`, is what `defectdetails.json` wants.
- Coverity caches results aggressively; if a CID disappears from "Outstanding"
  immediately after finalize, a fresh fetch can take a few seconds to reflect.
- Idempotence: `fetch-details.sh` skips files already present, so partial runs
  are safe to re-invoke.
- `prepare-defect.sh` uses Coverity's `displayFile` and the main-event line
  to extract source context. If Coverity's line numbers are stale (after a
  refactor), the context may not center on the current code -- use it as a
  hint, not an authoritative location.
- Coverity load-balances across `scanN.scan.coverity.com` hosts. Whatever
  hostname your browser landed on must be the one in `COVERITY_HOST`;
  cookies are per-host.
