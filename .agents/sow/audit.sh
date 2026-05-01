#!/usr/bin/env bash
# Read-only audit for a project-local SOW setup.
# Reports current state of cwd: is SOW initialized? what's in place? what's missing?
# Never modifies anything.

set -uo pipefail

# Color output if stdout is a tty
if [ -t 1 ]; then
  RED=$'\033[0;31m'
  GREEN=$'\033[0;32m'
  YELLOW=$'\033[1;33m'
  BLUE=$'\033[0;34m'
  GRAY=$'\033[0;90m'
  NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BLUE=""; GRAY=""; NC=""
fi

cwd=$(pwd)
echo "${BLUE}=== SOW audit (cwd=$cwd) ===${NC}"
echo

is_output_reference_skill() {
  local name="$1"
  [ -f ./AGENTS.md ] && awk -v name="$name" '
    BEGIN { in_output = 0; found = 0 }
    /^[[:space:]]*Output\/reference skills:[[:space:]]*$/ { in_output = 1; next }
    /^[[:space:]]*(Runtime input skills|Legacy runtime skills):[[:space:]]*$/ { if (in_output) in_output = 0 }
    /^#{1,6}[[:space:]]/ && $0 !~ /Project Skills/ { if (in_output) in_output = 0 }
    in_output && index($0, ".agents/skills/" name "/") { found = 1 }
    END { exit found ? 0 : 1 }
  ' ./AGENTS.md 2>/dev/null
}

is_legacy_runtime_skill() {
  local name="$1"
  [ -f ./AGENTS.md ] && awk -v name="$name" '
    BEGIN { in_legacy = 0; found = 0 }
    /^[[:space:]]*Legacy runtime skills:[[:space:]]*$/ { in_legacy = 1; next }
    /^[[:space:]]*(Runtime input skills|Output\/reference skills):[[:space:]]*$/ { if (in_legacy) in_legacy = 0 }
    /^#{1,6}[[:space:]]/ && $0 !~ /Project Skills/ && $0 !~ /Legacy runtime skills/ { if (in_legacy) in_legacy = 0 }
    in_legacy && index($0, ".agents/skills/" name "/") { found = 1 }
    END { exit found ? 0 : 1 }
  ' ./AGENTS.md 2>/dev/null
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

# --- Marker check ---
echo "${BLUE}-- initialization marker --${NC}"
if [ -f ./AGENTS.md ]; then
  if grep -q "^Project SOW status: initialized$" ./AGENTS.md 2>/dev/null; then
    echo "  ${GREEN}OK${NC}  marker present in ./AGENTS.md"
    initialized=true
  else
    echo "  ${YELLOW}--${NC}  AGENTS.md exists but marker absent (partial state or not initialized)"
    initialized=false
  fi
else
  echo "  ${RED}--${NC}  ./AGENTS.md does not exist (create or normalize project instructions first, then SOW init)"
  initialized=false
fi
echo

# --- Canonical AGENTS.md sections ---
echo "${BLUE}-- canonical AGENTS.md sections --${NC}"
required_sections=(
  "## Goals"
  "## SOW System"
  "### Roles"
  "### Git Worktrees"
  "### Pre-Implementation Gate"
  "### Project Skills"
  "### Specs"
  "### Project-specific overrides"
)
sections_ok=0
sections_missing=0
if [ -f ./AGENTS.md ]; then
  for s in "${required_sections[@]}"; do
    if grep -qF "$s" ./AGENTS.md 2>/dev/null; then
      echo "  ${GREEN}OK${NC}  $s"
      sections_ok=$((sections_ok + 1))
    else
      echo "  ${RED}--${NC}  $s (missing)"
      sections_missing=$((sections_missing + 1))
    fi
  done
else
  echo "  ${GRAY}(AGENTS.md not present; skipping section check)${NC}"
fi
echo

# --- Cross-tool instruction bridges ---
echo "${BLUE}-- cross-tool instruction bridges --${NC}"
bridge_missing=0
if [ -f ./AGENTS.md ]; then
  if [ -L ./CLAUDE.md ] && [ "$(readlink ./CLAUDE.md 2>/dev/null)" = "AGENTS.md" ]; then
    echo "  ${GREEN}OK${NC}  CLAUDE.md -> AGENTS.md"
  else
    echo "  ${RED}--${NC}  CLAUDE.md -> AGENTS.md  (missing or not a relative symlink)"
    bridge_missing=$((bridge_missing + 1))
  fi
  if [ -L ./GEMINI.md ] && [ "$(readlink ./GEMINI.md 2>/dev/null)" = "AGENTS.md" ]; then
    echo "  ${GREEN}OK${NC}  GEMINI.md -> AGENTS.md"
  else
    echo "  ${RED}--${NC}  GEMINI.md -> AGENTS.md  (missing or not a relative symlink)"
    bridge_missing=$((bridge_missing + 1))
  fi
  if [ -d ./.agents/skills ]; then
    echo "  ${GREEN}OK${NC}  .agents/skills/"
  else
    echo "  ${RED}--${NC}  .agents/skills/  (missing; create even when no project skills exist)"
    bridge_missing=$((bridge_missing + 1))
  fi
  if [ -L ./.claude/skills ] && [ "$(readlink ./.claude/skills 2>/dev/null)" = "../.agents/skills" ]; then
    echo "  ${GREEN}OK${NC}  .claude/skills -> ../.agents/skills"
  else
    echo "  ${RED}--${NC}  .claude/skills -> ../.agents/skills  (missing or not a relative symlink)"
    bridge_missing=$((bridge_missing + 1))
  fi
else
  echo "  ${GRAY}(AGENTS.md not present; skipping bridge check)${NC}"
fi
echo

# --- .agents/sow/ directories ---
echo "${BLUE}-- SOW directories --${NC}"
sow_dirs=(specs pending current done)
sow_dir_ok=0
sow_dir_missing=0
empty_sow_dir_missing_keep=0
for d in "${sow_dirs[@]}"; do
  if [ -d ".agents/sow/$d" ]; then
    echo "  ${GREEN}OK${NC}  .agents/sow/$d/"
    sow_dir_ok=$((sow_dir_ok + 1))
    has_entries=$(find ".agents/sow/$d" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l | tr -d ' ')
    if [ "$has_entries" -eq 0 ] && [ ! -f ".agents/sow/$d/.gitkeep" ] && [ ! -f ".agents/sow/$d/.keep" ]; then
      echo "      ${YELLOW}*${NC}  empty directory has no .gitkeep or .keep placeholder"
      empty_sow_dir_missing_keep=$((empty_sow_dir_missing_keep + 1))
    fi
  else
    echo "  ${RED}--${NC}  .agents/sow/$d/  (missing)"
    sow_dir_missing=$((sow_dir_missing + 1))
  fi
done
echo

# --- Project-local framework files ---
echo "${BLUE}-- project-local SOW framework files --${NC}"
framework_missing=0
if [ -f ".agents/sow/SOW.template.md" ]; then
  echo "  ${GREEN}OK${NC}  .agents/sow/SOW.template.md"
  if grep -q "^## Pre-Implementation Gate$" ".agents/sow/SOW.template.md" 2>/dev/null; then
    echo "      ${GREEN}OK${NC}  template includes Pre-Implementation Gate"
    sow_template_pre_impl_missing=0
  else
    echo "      ${RED}--${NC}  template missing ## Pre-Implementation Gate"
    sow_template_pre_impl_missing=1
  fi
else
  echo "  ${RED}--${NC}  .agents/sow/SOW.template.md  (missing)"
  framework_missing=$((framework_missing + 1))
  sow_template_pre_impl_missing=1
fi
if [ -f ".agents/sow/audit.sh" ]; then
  echo "  ${GREEN}OK${NC}  .agents/sow/audit.sh"
else
  echo "  ${RED}--${NC}  .agents/sow/audit.sh  (missing)"
  framework_missing=$((framework_missing + 1))
fi
echo

# --- SOW counts per status ---
echo "${BLUE}-- SOW counts per status --${NC}"
for d in pending current done; do
  if [ -d ".agents/sow/$d" ]; then
    n=$(find ".agents/sow/$d" -mindepth 1 -maxdepth 1 -name 'SOW-*.md' -type f 2>/dev/null | wc -l | tr -d ' ')
    if [ "$n" -gt 0 ]; then
      echo "  $d: $n"
      find ".agents/sow/$d" -mindepth 1 -maxdepth 1 -name 'SOW-*.md' -type f -printf '    %f\n' 2>/dev/null | sort
    else
      echo "  $d: ${GRAY}(empty)${NC}"
    fi
  fi
done
echo

# --- SOW status/directory consistency ---
echo "${BLUE}-- SOW status/directory consistency --${NC}"
sow_status_mismatch=0
sow_status_missing=0
sow_status_checked=0
for d in pending current done; do
  [ -d ".agents/sow/$d" ] || continue
  while IFS= read -r f; do
    [ -z "$f" ] && continue
    sow_status_checked=$((sow_status_checked + 1))
    status=$(read_sow_status "$f")
    if [ -z "$status" ]; then
      echo "  ${RED}--${NC}  $f  (missing Status: line)"
      sow_status_missing=$((sow_status_missing + 1))
      continue
    fi
    ok=false
    case "$d:$status" in
      pending:open|current:in-progress|current:paused|done:completed|done:closed)
        ok=true
        ;;
    esac
    if $ok; then
      echo "  ${GREEN}OK${NC}  $f  ($status)"
    else
      echo "  ${RED}--${NC}  $f  (Status: $status does not match $d/)"
      sow_status_mismatch=$((sow_status_mismatch + 1))
    fi
  done < <(find ".agents/sow/$d" -mindepth 1 -maxdepth 1 -name 'SOW-*.md' -type f 2>/dev/null | sort)
done
if [ "$sow_status_checked" -eq 0 ]; then
  echo "  ${GRAY}(no SOW files found)${NC}"
fi
echo

# --- Current SOW pre-implementation gates ---
echo "${BLUE}-- current SOW pre-implementation gates --${NC}"
current_sow_pre_impl_missing=0
current_sow_pre_impl_checked=0
if [ -d ".agents/sow/current" ]; then
  while IFS= read -r f; do
    [ -z "$f" ] && continue
    current_sow_pre_impl_checked=$((current_sow_pre_impl_checked + 1))
    if grep -q "^## Pre-Implementation Gate$" "$f" 2>/dev/null; then
      echo "  ${GREEN}OK${NC}  $f"
    else
      echo "  ${RED}--${NC}  $f  (missing ## Pre-Implementation Gate before implementation continues)"
      current_sow_pre_impl_missing=$((current_sow_pre_impl_missing + 1))
    fi
  done < <(find ".agents/sow/current" -mindepth 1 -maxdepth 1 -name 'SOW-*.md' -type f 2>/dev/null | sort)
fi
if [ "$current_sow_pre_impl_checked" -eq 0 ]; then
  echo "  ${GRAY}(no current SOW files found)${NC}"
fi
echo

# --- Project skills ---
echo "${BLUE}-- runtime project skills --${NC}"
project_skills_ok=0
project_skills_total=0
project_output_reference_total=0
if [ -d .agents/skills ]; then
  while IFS= read -r d; do
    [ -z "$d" ] && continue
    name=$(basename "$d")
    if is_output_reference_skill "$name"; then
      project_output_reference_total=$((project_output_reference_total + 1))
      echo "  ${GREEN}OK${NC}  $name  (listed as output/reference; excluded from default runtime guidance)"
      continue
    fi
    project_skills_total=$((project_skills_total + 1))
    if [ -f "$d/SKILL.md" ]; then
      lines=$(wc -l <"$d/SKILL.md" 2>/dev/null | tr -d ' ')
      echo "  ${GREEN}OK${NC}  $name  ($lines lines)"
      project_skills_ok=$((project_skills_ok + 1))
    else
      echo "  ${RED}--${NC}  $name  (no SKILL.md)"
    fi
  done < <(find .agents/skills -mindepth 1 -maxdepth 1 -type d -name 'project-*' 2>/dev/null | sort)
  if [ "$project_skills_total" -eq 0 ] && [ "$project_output_reference_total" -eq 0 ]; then
    echo "  ${YELLOW}(no .agents/skills/project-*/ found)${NC}"
  elif [ "$project_skills_total" -eq 0 ]; then
    echo "  ${YELLOW}(no runtime input project-* skills found; only output/reference exceptions)${NC}"
  fi
else
  echo "  ${YELLOW}(.agents/skills/ does not exist)${NC}"
fi
echo

# --- Non-project skill directories ---
echo "${BLUE}-- non-project skill directories --${NC}"
non_project_skills_total=0
non_project_skills_unclassified=0
if [ -d .agents/skills ]; then
  while IFS= read -r d; do
    [ -z "$d" ] && continue
    non_project_skills_total=$((non_project_skills_total + 1))
    name=$(basename "$d")
    if is_output_reference_skill "$name"; then
      echo "  ${GREEN}OK${NC}  $name  (listed as output/reference; not auto-loaded as runtime SOW skill)"
    elif is_legacy_runtime_skill "$name"; then
      echo "  ${GREEN}OK${NC}  $name  (listed as legacy runtime skill; project-* alignment deferred)"
    elif [ -f "$d/SKILL.md" ]; then
      echo "  ${YELLOW}*${NC}  $name  (not auto-loaded as runtime SOW skill)"
      non_project_skills_unclassified=$((non_project_skills_unclassified + 1))
    else
      echo "  ${RED}--${NC}  $name  (not project-* and no SKILL.md)"
      non_project_skills_unclassified=$((non_project_skills_unclassified + 1))
    fi
  done < <(find .agents/skills -mindepth 1 -maxdepth 1 -type d ! -name 'project-*' 2>/dev/null | sort)
  if [ "$non_project_skills_total" -eq 0 ]; then
    echo "  ${GREEN}OK${NC}  none"
  elif [ "$non_project_skills_unclassified" -gt 0 ]; then
    echo "  ${GRAY}Classify each warning as output/reference, obsolete, or rename/wrap it as project-* if it is runtime input.${NC}"
  fi
else
  echo "  ${GRAY}(.agents/skills/ does not exist)${NC}"
fi
echo

# --- TODO files at project root ---
echo "${BLUE}-- TODO files at project root --${NC}"
todo_count=0
todo_tracked=false
while IFS= read -r f; do
  [ -z "$f" ] && continue
  todo_count=$((todo_count + 1))
  echo "  ${YELLOW}*${NC}  $f  (not yet migrated)"
done < <(find . -maxdepth 1 -name 'TODO-*.md' -o -maxdepth 1 -name 'TODO.md' 2>/dev/null | sort)
if [ "$todo_count" -gt 0 ]; then
  if grep -RiqE "root TODO|TODO file|TODO migration|TODO classification|orphan TODO" .agents/sow/pending .agents/sow/current 2>/dev/null; then
    todo_tracked=true
    echo "  ${GREEN}OK${NC}  root TODO classification/migration is tracked by a pending/current SOW"
  fi
else
  if [ -d .agents/sow/.todo-backup ]; then
    bk=$(find .agents/sow/.todo-backup -maxdepth 1 -name 'TODO*.md' 2>/dev/null | wc -l | tr -d ' ')
    if [ "$bk" -gt 0 ]; then
      echo "  ${GREEN}OK${NC}  no orphan TODO files (${bk} backed up at .agents/sow/.todo-backup/)"
    else
      echo "  ${GREEN}OK${NC}  no TODO files at project root"
    fi
  else
    echo "  ${GREEN}OK${NC}  no TODO files at project root"
  fi
fi
todo_untracked_count=$todo_count
if $todo_tracked; then
  todo_untracked_count=0
fi
echo

# --- Backup of pre-SOW AGENTS.md ---
echo "${BLUE}-- pre-SOW AGENTS.md backup --${NC}"
if [ -f ./AGENTS.md.pre-sow.bak ]; then
  echo "  ${GREEN}OK${NC}  AGENTS.md.pre-sow.bak present (preserves original)"
else
  if $initialized; then
    echo "  ${GRAY}(no backup file; either init was clean or backup name differs)${NC}"
  else
    echo "  ${GRAY}(not yet initialized)${NC}"
  fi
fi
echo

# --- Final verdict ---
echo "${BLUE}-- verdict --${NC}"
skill_classification_warnings=${non_project_skills_unclassified:-0}

sow_status_errors=$((sow_status_mismatch + sow_status_missing))
pre_impl_errors=$((sow_template_pre_impl_missing + current_sow_pre_impl_missing))

if $initialized && [ "$sections_missing" -eq 0 ] && [ "$bridge_missing" -eq 0 ] && [ "$sow_dir_missing" -eq 0 ] && [ "$empty_sow_dir_missing_keep" -eq 0 ] && [ "$framework_missing" -eq 0 ] && [ "$sow_status_errors" -eq 0 ] && [ "$pre_impl_errors" -eq 0 ] && [ "$todo_untracked_count" -eq 0 ] && [ "$skill_classification_warnings" -eq 0 ]; then
  echo "  ${GREEN}=== SOW initialization complete and clean. ===${NC}"
  exit 0
elif $initialized && [ "$sections_missing" -eq 0 ] && [ "$bridge_missing" -eq 0 ] && [ "$sow_dir_missing" -eq 0 ] && [ "$empty_sow_dir_missing_keep" -eq 0 ] && [ "$framework_missing" -eq 0 ] && [ "$sow_status_errors" -eq 0 ] && [ "$pre_impl_errors" -eq 0 ] && [ "$todo_untracked_count" -eq 0 ]; then
  echo "  ${YELLOW}=== SOW initialization structurally complete with skill classification warning(s):${NC}"
  echo "    ${YELLOW}- ${skill_classification_warnings} non-project skill director(y/ies) need classification in AGENTS.md${NC}"
  echo "    ${YELLOW}- Runtime input skills should be renamed/wrapped as .agents/skills/project-*/${NC}"
  echo "    ${YELLOW}- Output/reference skills should be listed separately and kept out of the generic runtime hook${NC}"
  exit 0
elif $initialized; then
  echo "  ${YELLOW}=== SOW marker present but partial state detected:${NC}"
  [ "$sections_missing" -gt 0 ] && echo "    ${YELLOW}- ${sections_missing} canonical AGENTS.md section(s) missing${NC}"
  [ "$bridge_missing" -gt 0 ] && echo "    ${YELLOW}- ${bridge_missing} cross-tool instruction bridge(s) missing${NC}"
  [ "$sow_dir_missing" -gt 0 ] && echo "    ${YELLOW}- ${sow_dir_missing} SOW directory(ies) missing${NC}"
  [ "$empty_sow_dir_missing_keep" -gt 0 ] && echo "    ${YELLOW}- ${empty_sow_dir_missing_keep} empty SOW directory(ies) missing .gitkeep/.keep${NC}"
  [ "$framework_missing" -gt 0 ] && echo "    ${YELLOW}- ${framework_missing} project-local framework file(s) missing${NC}"
  [ "$sow_status_mismatch" -gt 0 ] && echo "    ${YELLOW}- ${sow_status_mismatch} SOW status/directory mismatch(es)${NC}"
  [ "$sow_status_missing" -gt 0 ] && echo "    ${YELLOW}- ${sow_status_missing} SOW file(s) missing Status line${NC}"
  [ "$sow_template_pre_impl_missing" -gt 0 ] && echo "    ${YELLOW}- project-local SOW template missing Pre-Implementation Gate${NC}"
  [ "$current_sow_pre_impl_missing" -gt 0 ] && echo "    ${YELLOW}- ${current_sow_pre_impl_missing} current SOW(s) missing Pre-Implementation Gate${NC}"
  [ "$todo_untracked_count" -gt 0 ] && echo "    ${YELLOW}- ${todo_untracked_count} untracked orphan TODO file(s) at project root${NC}"
  [ "$skill_classification_warnings" -gt 0 ] && echo "    ${YELLOW}- ${skill_classification_warnings} non-project skill director(y/ies) need classification${NC}"
  echo "  ${YELLOW}    Repair non-destructively using the project-local AGENTS.md and .agents/sow/SOW.template.md.${NC}"
  exit 0
else
  echo "  ${YELLOW}=== SOW NOT initialized. Install a project-local SOW framework before using SOWs here. ===${NC}"
  if [ ! -f ./AGENTS.md ]; then
    echo "  ${YELLOW}    AGENTS.md missing; create or normalize project instructions first.${NC}"
  fi
  exit 0
fi
