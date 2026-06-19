# Recipe: find by version (regression spotter)

Use case: "Did crash X appear in v2.10? Was it fixed in
v2.10.0-100-nightly?"

## Quick path

Compare crash counts across explicit versions:

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --health crash \
    --versions 'v2.9.0,v2.10.0,v2.10.0-100-nightly,v2.10.0-130-nightly' \
    --since '14d ago' \
    --last 1 \
    --facets 'AE_AGENT_VERSION,AE_FATAL_FUNCTION' \
    --histogram AE_AGENT_VERSION \
    --output /tmp/regression.json
```

`--last 1` because we only need the facet counts.
`--histogram AE_AGENT_VERSION` adds time-bucketed counts per
version, useful to spot when something disappeared.

Then read the response:

```bash
jq '.facets[]
    | select(.id=="AE_AGENT_VERSION")
    | .options
    | sort_by(-.count)
    | .[]
    | "\(.count)\t\(.id)"' /tmp/regression.json
```

## "When did this start?" (wide window)

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'specific_buggy_function' \
    --version all \
    --since '30d ago' \
    --last 1 \
    --facets 'AE_AGENT_VERSION' \
    --histogram AE_AGENT_VERSION \
    --output /tmp/origin.json
```

`--version all` is required -- the auto filter would mask
older versions. The histogram tells you when (in time) and
which versions started reporting.

The first appearance in a specific version + nightly date
gives you a commit window to bisect.

## "Was it fixed?" check

After a fix lands in nightly N:

```bash
.agents/skills/query-agent-events/scripts/get-events.sh \
    --function 'specific_buggy_function' \
    --versions 'v2.10.0-90-nightly,v2.10.0-100-nightly,v2.10.0-120-nightly' \
    --since '7d ago' \
    --last 1 \
    --facets 'AE_AGENT_VERSION' \
    --output /tmp/post-fix.json
```

Check if the count drops to ~zero in nightly N+1 and beyond.

## Triage flow

1. **Identify the candidate function / signal / cause** --
   start from `find-by-function.md` or `finding-crashes.md`.
2. **Wide-window query** with `--version all` to see the full
   version distribution.
3. **Read the histogram** -- when did this first appear?
   When (if ever) did it stop?
4. **Bisect commits** in the implicated version range.
5. **Verify the fix** with a post-fix query.

## Pitfalls

- **Old version events from unupdated agents are normal**.
  Don't conclude "the bug is back" from a single old-version
  record -- check the date.
- **`--versions auto` masks regressions** because it filters
  to recent versions. Use `--version all` for "when did this
  start?" investigations.
- **Nightly numbers are commits-since-tag**. Higher number =
  newer. Sorting nightly versions lexically is wrong; sort by
  the embedded number (the auto-detection in `_lib.sh` does
  this correctly).
- **Different agents on the same install**: distinct
  `AE_AGENT_ID` (Netdata machine GUID) but same `AE_HOST_ID`
  (OS machine-id). Group by `AE_AGENT_ID` to get unique
  installs, NOT by `AE_HOST_ID`.
