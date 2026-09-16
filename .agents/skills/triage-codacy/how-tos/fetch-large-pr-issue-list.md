# Fetch a large Codacy PR issue list

Use this when `pr-issues.sh <PR>` fails locally with:

```text
jq: Argument list too long
```

## Why it happens

`codacyaudit_pr_issues` can return a large JSON array. Passing that array to
`jq --argjson data "$issues_array"` sends the whole payload through the shell's
argument vector and can exceed the OS limit before `jq` starts.

## Correct pattern

Write the JSON array to a temporary file and load it through `jq --slurpfile`:

```bash
issues_tmp="$(mktemp "${TMPDIR:-/tmp}/codacy-pr-issues-XXXXXX")"
trap 'rm -f "$issues_tmp"' EXIT
printf '%s' "$issues_array" > "$issues_tmp"

jq -n \
  --arg pr "$PR" \
  --arg fetched_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson total "$total" \
  --slurpfile data "$issues_tmp" \
  '{pr: ($pr | tonumber), fetched_at: $fetched_at, total: $total, data: $data[0]}'
```

This keeps token bytes out of stdout and avoids shell argument-size limits.
