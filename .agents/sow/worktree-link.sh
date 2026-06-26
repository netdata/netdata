#!/usr/bin/env bash
# Set up and migrate the local-only SOW working memory.
#
# SOW queues (.agents/sow/q/**), specs (.agents/sow/specs/**), .local/ and .env
# are per-developer working memory that is never committed. This script makes a
# developer manage them once per machine instead of once per git worktree:
#
#   - In the ORIGIN checkout (the main worktree): create .agents/sow/q with the
#     canonical queues and migrate any pre-existing top-level queue dirs
#     (active/pending/current/done/...) into q/. Self-heal specs from history if
#     this checkout dropped them when it crossed the untrack commit.
#
#   - In a LINKED git worktree: symlink q/, specs/, .local and .env to the
#     origin checkout so the working memory is shared. Any local working memory
#     already present in the worktree is merged into origin first (no data loss).
#     If origin is not yet on the new SOW system, fail with instructions.
#
# Idempotent and safe to re-run. Never touches the git index. Never deletes a
# unique file: on a name collision it keeps both copies and warns.

set -uo pipefail
shopt -s dotglob nullglob   # globs include dotfiles (.gitkeep) and expand to nothing when empty

if [ -t 2 ]; then
  RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'
  BLUE=$'\033[0;34m'; GRAY=$'\033[0;90m'; NC=$'\033[0m'
else
  RED=""; GREEN=""; YELLOW=""; BLUE=""; GRAY=""; NC=""
fi

# Print a command before running it (transparency for file operations).
run() {
  printf >&2 "${GRAY}%s >${NC} ${YELLOW}" "$(pwd)"
  printf >&2 "%q " "$@"
  printf >&2 "${NC}\n"
  "$@"
}

info() { printf >&2 "%s\n" "${BLUE}$*${NC}"; }
warn() { printf >&2 "%s\n" "${YELLOW}WARN:${NC} $*"; }
die()  { printf >&2 "%s\n" "${RED}ERROR:${NC} $*"; exit 1; }

SOW_DIR=".agents/sow"
NON_QUEUE=" q specs "                       # dirs under .agents/sow/ that are NOT queues
CANON_QUEUES="pending current done active"  # queues ensured to exist inside q/

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || die "not inside a git work tree"

top=$(git rev-parse --show-toplevel)
git_dir=$(git rev-parse --absolute-git-dir)
common_dir=$(cd "$(git rev-parse --git-common-dir)" && pwd)
cd "$top" || die "cannot cd to repo top: $top"

# An origin checkout is "on the new SOW system" once its .gitignore carries the
# precise queue rule. This is the marker the migration commit introduces.
origin_is_new() { grep -q '^/\.agents/sow/q$' "$1/.gitignore" 2>/dev/null; }

# True if git tracks any file under the given repo-relative path.
has_tracked_files() { [ -n "$(git ls-files -- "$1" 2>/dev/null | head -1)" ]; }

# Collision-safe merge of an untracked directory tree into a destination.
# $1 = source dir, $2 = destination dir. Label used to disambiguate collisions.
merge_into() {
  local src="${1%/}" dest="$2" label="$3"
  [ -d "$dest" ] || run mkdir -p "$dest"
  local f base target n
  for f in "$src"/*; do
    base=$(basename "$f")
    target="$dest/$base"
    if [ ! -e "$target" ]; then
      run mv "$f" "$target"
    elif [ -d "$f" ] && [ -d "$target" ]; then
      merge_into "$f" "$target" "$label"
      rmdir "$f" 2>/dev/null || true
    elif [ -f "$f" ] && [ -f "$target" ] && cmp -s "$f" "$target"; then
      run rm -f "$f"                       # identical duplicate, drop it
    else
      n=1
      while [ -e "$dest/$base.from-$label-$n" ]; do n=$((n + 1)); done
      warn "collision: '$f' differs from '$target' — keeping both"
      run mv "$f" "$dest/$base.from-$label-$n"
    fi
  done
}

# Move this checkout's pre-existing top-level queue dirs (active/pending/...)
# into a target q/. Target is this checkout's own q/ in origin mode, or origin's
# q/ when invoked from a worktree (so worktree-local SOWs land in the shared store).
# $1 = target q dir (absolute), $2 = collision label.
migrate_old_queues() {
  local targetq="$1" label="$2" sow="$top/$SOW_DIR" d name
  for d in "$sow"/*/; do
    name=$(basename "$d")
    case "$NON_QUEUE" in *" $name "*) continue ;; esac   # skip q, specs
    [ -L "${d%/}" ] && continue                          # skip symlinks
    if has_tracked_files "$SOW_DIR/$name"; then
      die "tracked files under $SOW_DIR/$name — run the migration commit (git rm --cached) first; aborting to protect git state"
    fi
    info "migrating $SOW_DIR/$name -> $targetq/$name"
    merge_into "${d%/}" "$targetq/$name" "$label"
    rmdir "${d%/}" 2>/dev/null || warn "$SOW_DIR/$name not empty after migration; left in place"
  done
}

# ORIGIN: if specs were dropped because this checkout crossed the untrack commit,
# restore them from history without touching the index.
selfheal_specs() {
  local specs="$top/$SOW_DIR/specs" del parent
  run mkdir -p "$specs"
  [ -n "$(find "$specs" -type f -name '*.md' 2>/dev/null | head -1)" ] && return 0
  del=$(git log -1 --diff-filter=D --format=%H -- "$SOW_DIR/specs" 2>/dev/null)
  [ -n "$del" ] || return 0
  parent="${del}^"
  git rev-parse "$parent" >/dev/null 2>&1 || return 0
  info "specs missing on disk; restoring from history ($parent)"
  if git archive "$parent" -- "$SOW_DIR/specs" 2>/dev/null | tar -x -C "$top" 2>/dev/null; then
    info "specs restored from history"
  else
    warn "could not restore specs from history (recover manually with: git archive $parent -- $SOW_DIR/specs | tar -x)"
  fi
}

# WORKTREE: merge any local content at $rel into origin, then symlink to origin.
link_one() {
  local rel="$1" target="$2" here="$top/$1"
  [ -L "$here" ] && return 0                              # already linked
  if [ -e "$here" ]; then
    if [ -d "$here" ]; then
      has_tracked_files "$rel" && die "tracked files under $rel in this worktree — run the migration commit first; aborting"
      [ -d "$target" ] || run mkdir -p "$target"
      info "merging worktree $rel into origin before linking"
      merge_into "$here" "$target" "$(basename "$top")"
      rmdir "$here" 2>/dev/null || { warn "$rel not empty after merge; left in place, not linked"; return 0; }
    else
      warn "$rel is a real file in this worktree; not replacing with a link"
      return 0
    fi
  fi
  [ -e "$target" ] || { warn "origin has no $rel; nothing to link"; return 0; }
  run ln -s "$target" "$here"
}

link_worktree() {
  local origin="$1"
  [ -d "$origin/$SOW_DIR" ] || die "cannot find origin working tree at '$origin' (bare repo?)"
  if ! origin_is_new "$origin"; then
    die "origin checkout '$origin' is not on the new SOW system (no /.agents/sow/q in its .gitignore).
Update the origin checkout first, then re-run this script:
  (cd '$origin' && git pull --rebase)        # bring in the SOW migration
  (cd '$origin' && $SOW_DIR/worktree-link.sh)
Aborting — no files were moved, no data lost."
  fi
  # Judgment call C: build origin's q/ if the migration commit landed but the
  # origin setup has not run yet.
  [ -d "$origin/$SOW_DIR/q" ] || { warn "origin has no q/ yet; building it"; ( cd "$origin" && "$origin/$SOW_DIR/worktree-link.sh" ); }

  # Case 4: this worktree may still carry old-style top-level queue dirs; push
  # their contents into origin's shared q/ before linking (no data loss).
  migrate_old_queues "$origin/$SOW_DIR/q" "$(basename "$top")"

  link_one "$SOW_DIR/q"     "$origin/$SOW_DIR/q"
  link_one "$SOW_DIR/specs" "$origin/$SOW_DIR/specs"
  link_one ".local"         "$origin/.local"
  link_one ".env"           "$origin/.env"
}

if [ "$git_dir" = "$common_dir" ]; then
  info "origin checkout: $top"
  origin_is_new "$top" || warn ".gitignore lacks '/.agents/sow/q' — finish the migration commit so worktrees can detect this origin as migrated"
  run mkdir -p "$top/$SOW_DIR/q"
  migrate_old_queues "$top/$SOW_DIR/q" "migrated"
  for q in $CANON_QUEUES; do run mkdir -p "$top/$SOW_DIR/q/$q"; done
  selfheal_specs
  run mkdir -p "$top/.local"
  info "${GREEN}SOW working memory ready under $SOW_DIR/q (origin).${NC}"
else
  origin=$(dirname "$common_dir")
  info "linked worktree: $top  ->  origin: $origin"
  link_worktree "$origin"
  info "${GREEN}worktree linked to origin SOW working memory.${NC}"
fi
