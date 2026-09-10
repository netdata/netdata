#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/repo"
printf '%s\n' "#!/usr/bin/env bash" \
  'set -euo pipefail' \
  'printf "%s\\n" "$*" >> "${CODACY_MOCK_LOG}"' \
  'case "${1:-}" in' \
  '  version) printf "%s\\n" "codacy-cli v2.3.4" ;;' \
  '  init) mkdir -p .codacy; : > .codacy/codacy.yaml ;;' \
  '  install) ;;' \
  '  analyze) while [ "$#" -gt 0 ]; do [ "$1" = "--output" ] && { shift; printf "%s\\n" "{\"version\":\"2.1.0\",\"runs\":[]}" > "$1"; exit 0; }; shift; done ;;' \
  'esac' > "$tmp/bin/codacy-cli"
chmod +x "$tmp/bin/codacy-cli"

output="$tmp/result.sarif"
PATH="$tmp/bin:$PATH" CODACY_MOCK_LOG="$tmp/calls" CODACY_CLI_V2_VERSION=2.3.4 \
  "$root/.agents/skills/triage-codacy/scripts/analyze-local.sh" \
  --directory "$tmp/repo" --output "$output" >/dev/null

jq -e '.version == "2.1.0" and (.runs | type) == "array"' "$output" >/dev/null
grep -Fx "init" "$tmp/calls" >/dev/null
grep -Fx "install" "$tmp/calls" >/dev/null
grep -F -- "analyze $tmp/repo --format sarif --output $output" "$tmp/calls" >/dev/null

relative_output="$tmp/relative.sarif"
(
  cd "$tmp"
  PATH="$tmp/bin:$PATH" CODACY_MOCK_LOG="$tmp/calls" CODACY_CLI_V2_VERSION=2.3.4 \
    "$root/.agents/skills/triage-codacy/scripts/analyze-local.sh" \
    --directory "$tmp/repo" --output "relative.sarif" >/dev/null
)
[ -s "$relative_output" ]

set +e
PATH="$tmp/bin:$PATH" CODACY_MOCK_LOG="$tmp/calls" CODACY_CLI_V2_VERSION=9.9.9 \
  "$root/.agents/skills/triage-codacy/scripts/analyze-local.sh" \
  --directory "$tmp/repo" --output "$tmp/version-mismatch.sarif" >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -eq 2 ]

PATH="$tmp/bin:$PATH" CODACY_MOCK_LOG="$tmp/calls" CODACY_CLI_V2_VERSION=2.3.4 \
  "$root/.agents/skills/triage-codacy/scripts/analyze-local.sh" \
  --directory "$tmp/repo" --output "$tmp/result-2.sarif" >/dev/null
if grep -Fx "init" "$tmp/calls" | wc -l | grep -qv '^1$'; then
  printf 'init should run only for an uninitialized project\n' >&2
  exit 1
fi

printf '%s\n' '#!/usr/bin/env bash' 'exit 0' > "$tmp/bin/codacy-analysis-cli"
chmod +x "$tmp/bin/codacy-analysis-cli"
set +e
PATH="$tmp/bin:$PATH" CODACY_MOCK_LOG="$tmp/calls" CODACY_CLI_V2_VERSION=2.3.4 \
  "$root/.agents/skills/triage-codacy/scripts/analyze-local.sh" \
  --directory "$tmp/repo" --output "$tmp/legacy.sarif" >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -eq 2 ]

printf 'PASS: triage-codacy uses Codacy CLI v2\n'
