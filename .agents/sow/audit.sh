#!/usr/bin/env bash
# Read-only audit for the project-local SOW system.
# Never modifies files.

set -uo pipefail

if [ -t 1 ]; then
  RED=$'\033[0;31m'
  GREEN=$'\033[0;32m'
  YELLOW=$'\033[1;33m'
  BLUE=$'\033[0;34m'
  GRAY=$'\033[0;90m'
  NC=$'\033[0m'
else
  RED=""
  GREEN=""
  YELLOW=""
  BLUE=""
  GRAY=""
  NC=""
fi

failures=0
warnings=0

ok() {
  echo "  ${GREEN}OK${NC}  $*"
}

fail() {
  echo "  ${RED}--${NC}  $*"
  failures=$((failures + 1))
}

warn() {
  echo "  ${YELLOW}--${NC}  $*"
  warnings=$((warnings + 1))
}

section() {
  echo
  echo "${BLUE}-- $* --${NC}"
}

read_sow_status() {
  awk '
    function clean(s, a) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
      gsub(/`/, "", s)
      gsub(/\*\*/, "", s)
      sub(/^Status:[[:space:]]*/, "", s)
      sub(/^status:[[:space:]]*/, "", s)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
      split(s, a, /[[:space:]|—]+/)
      print a[1]
      exit
    }
    /^Status:[[:space:]]*/ { clean($0) }
    /^status:[[:space:]]*/ { clean($0) }
    /^\*\*Status:\*\*[[:space:]]*/ { clean($0) }
    /^## Status[[:space:]]*$/ { in_status = 1; next }
    in_status && NF { clean($0) }
  ' "$1" 2>/dev/null
}

echo "${BLUE}=== SOW audit (cwd=$(pwd)) ===${NC}"

section "initialization marker"
if [ -f AGENTS.md ]; then
  if grep -q "^Project SOW status: initialized$" AGENTS.md; then
    ok "marker present in AGENTS.md"
  else
    fail "AGENTS.md exists but Project SOW status marker is missing"
  fi
else
  fail "AGENTS.md is missing"
fi

section "canonical AGENTS.md sections"
required_sections=(
  "## Goals"
  "## SOW System"
  "### Roles"
  "### Git Worktrees"
  "### Sensitive Data In Durable Artifacts"
  "### Durable AI-Facing Artifact Formatting"
  "### Open-Source Reference Evidence"
  "### Pre-Implementation Gate"
  "### SOW Completion And Merge"
  "### Enforcement"
  "### Regressions"
  "### Project Skills"
  "### Specs"
  "### Project-specific overrides"
)

for heading in "${required_sections[@]}"; do
  if grep -qF "$heading" AGENTS.md 2>/dev/null; then
    ok "$heading"
  else
    fail "$heading is missing"
  fi
done

if grep -qF "CRITICAL: Never write raw sensitive data to durable artifacts." AGENTS.md 2>/dev/null; then
  ok "CRITICAL sensitive-data warning"
else
  fail "CRITICAL sensitive-data warning is missing from AGENTS.md"
fi

section "SOW layout"
for path in .agents/sow/active .agents/sow/specs; do
  if [ -d "$path" ]; then
    ok "$path exists"
  else
    fail "$path is missing"
  fi
done

for path in .agents/sow/SOW.template.md .agents/sow/audit.sh .agents/sow/scan-sensitive.sh .agents/sow/specs/README.md; do
  if [ -f "$path" ]; then
    ok "$path exists"
  else
    fail "$path is missing"
  fi
done

for legacy_dir in .agents/sow/pending .agents/sow/current .agents/sow/done; do
  if [ -e "$legacy_dir" ]; then
    fail "$legacy_dir must not exist in the active-only SOW model"
  else
    ok "$legacy_dir absent"
  fi
done

section "active SOW files"
active_count=0
if [ -d .agents/sow/active ]; then
  while IFS= read -r sow; do
    [ -n "$sow" ] || continue
    active_count=$((active_count + 1))
    status=$(read_sow_status "$sow")

    case "$status" in
      planning|ready|in-progress|paused|completed)
        ok "$sow status=$status"
        ;;
      "")
        fail "$sow has no Status"
        ;;
      *)
        fail "$sow has invalid Status: $status"
        ;;
    esac

    for needle in \
      "## Pre-Implementation Gate" \
      "Sensitive data handling plan:" \
      "Sensitive data gate:" \
      "## Validation" \
      "## Artifact Maintenance Gate"
    do
      if grep -qF "$needle" "$sow"; then
        ok "$sow contains $needle"
      else
        fail "$sow is missing $needle"
      fi
    done
  done < <(find .agents/sow/active -maxdepth 1 -type f -name 'SOW-*.md' 2>/dev/null | sort)
fi

if [ "$active_count" -eq 0 ]; then
  ok "no active branch-local SOW files"
else
  warn "$active_count active branch-local SOW file(s); they must be deleted before merge"
fi

section "spec index"
if [ -f .agents/sow/specs/README.md ]; then
  while IFS= read -r spec; do
    [ -n "$spec" ] || continue
    rel=${spec#.agents/sow/specs/}
    if grep -qF "]($rel)" .agents/sow/specs/README.md; then
      ok "$rel is listed in specs/README.md"
    else
      fail "$rel is missing from specs/README.md"
    fi
  done < <(find .agents/sow/specs -maxdepth 1 -type f -name '*.md' ! -name README.md 2>/dev/null | sort)

  while IFS= read -r link; do
    [ -n "$link" ] || continue
    if [ -f ".agents/sow/specs/$link" ]; then
      ok "spec index link resolves: $link"
    else
      fail "spec index link is broken: $link"
    fi
  done < <(grep -oE '\]\([A-Za-z0-9._-]+\.md\)' .agents/sow/specs/README.md 2>/dev/null | sed 's/^](//; s/)$//' | sort -u)
else
  fail ".agents/sow/specs/README.md is missing"
fi

section "spec references"
if command -v rg >/dev/null 2>&1; then
  while IFS= read -r ref; do
    [ -n "$ref" ] || continue
    if [ -f "$ref" ]; then
      ok "spec reference resolves: $ref"
    else
      fail "spec reference is broken: $ref"
    fi
  done < <(
    rg --no-filename -o '\.agents/sow/specs/[A-Za-z0-9._-]+\.md' \
      AGENTS.md .agents/skills .agents/sow/specs docs src \
      -g '*.md' -g 'SKILL.md' -g '*.sh' -g '*.yml' \
      2>/dev/null | sort -u
  )
else
  warn "ripgrep not available; skipped spec reference audit"
fi

section "legacy SOW references"
if command -v rg >/dev/null 2>&1; then
  legacy_refs=$(rg --line-number 'SOW-[0-9]{4}\b' \
    AGENTS.md .agents .github docs src \
    -g '*.md' -g 'SKILL.md' -g '*.sh' -g '*.yml' \
    -g '!TODO*.md' \
    -g '!**/TODO*.md' \
    -g '!**/.agents/sow/active/SOW-*.md' \
    2>/dev/null || true)

  if [ -n "$legacy_refs" ]; then
    printf '%s\n' "$legacy_refs"
    fail "legacy SOW-NNNN references remain in durable files"
  else
    ok "no legacy SOW-NNNN references in durable files"
  fi
else
  warn "ripgrep not available; skipped legacy SOW reference audit"
fi

section "sensitive data"
scan_files=()

for path in AGENTS.md CLAUDE.md GEMINI.md .agents/ENV.md .agents/sow/SOW.template.md .agents/sow/specs/README.md; do
  [ -f "$path" ] && scan_files+=("$path")
done

if [ -d .agents/sow/specs ]; then
  while IFS= read -r file; do
    scan_files+=("$file")
  done < <(find .agents/sow/specs -type f -name '*.md' 2>/dev/null | sort)
fi

if [ -d .agents/sow/active ]; then
  while IFS= read -r file; do
    scan_files+=("$file")
  done < <(find .agents/sow/active -type f -name '*.md' 2>/dev/null | sort)
fi

if [ -d .agents/skills ]; then
  while IFS= read -r file; do
    scan_files+=("$file")
  done < <(find .agents/skills -type f 2>/dev/null | sort)
fi

if [ -d .agents/skill-verification ]; then
  while IFS= read -r file; do
    scan_files+=("$file")
  done < <(find .agents/skill-verification -type f 2>/dev/null | sort)
fi

if [ "${#scan_files[@]}" -eq 0 ]; then
  warn "no files selected for sensitive-data scan"
elif bash .agents/sow/scan-sensitive.sh "${scan_files[@]}"; then
  ok "sensitive-data scan passed (${#scan_files[@]} files)"
else
  fail "sensitive-data scan found potential leaks"
fi

section "summary"
if [ "$failures" -eq 0 ]; then
  if [ "$warnings" -eq 0 ]; then
    echo "${GREEN}PASS${NC}  no SOW audit failures or warnings"
  else
    echo "${YELLOW}PASS${NC}  no SOW audit failures; warnings=$warnings"
  fi
  exit 0
fi

echo "${RED}FAIL${NC}  failures=$failures warnings=$warnings"
exit 1
