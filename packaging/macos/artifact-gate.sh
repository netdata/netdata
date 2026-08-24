#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Verify that a staged macOS install tree is fit for a self-contained package.
#
# A package built from this tree must run on a clean macOS machine: no
# Homebrew, no developer toolchain, nothing outside the OS and the payload
# itself. This gate walks the tree and fails on anything that would break
# that promise:
#
#   loads    - a Mach-O references a library outside /usr/lib and
#              /System/Library that is not part of the payload
#   arch     - a Mach-O is not arm64-only (x86_64 slice or fat binary)
#   minos    - a Mach-O targets a macOS version newer than the support floor
#   rpath    - a Mach-O carries an LC_RPATH pointing outside the payload
#   shebang  - an executable script's interpreter does not exist on a clean
#              macOS machine and is not part of the payload
#   symlink  - a symlink is broken or points outside the payload
#
# Usage: artifact-gate.sh [options] <tree>
#
#   <tree>                     root of the staged install (a DESTDIR)
#   --prefix <path>            install prefix the payload lands at, e.g.
#                              /opt/netdata; absolute references under it are
#                              resolved inside <tree> (default: none)
#   --deployment-target <ver>  maximum allowed minos, default 14.0
#
# Exit status: 0 when the tree is clean, 1 on violations, 2 on usage errors.
#
# Runs with the stock macOS toolchain (bash 3.2, otool, lipo, file).

set -u

DEPLOYMENT_TARGET="14.0"
PREFIX=""
TREE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --prefix)
      PREFIX="${2:?--prefix needs a value}"
      shift 2
      ;;
    --deployment-target)
      DEPLOYMENT_TARGET="${2:?--deployment-target needs a value}"
      shift 2
      ;;
    -h|--help)
      sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    -*)
      echo "Unknown option: $1" >&2
      exit 2
      ;;
    *)
      if [ -n "${TREE}" ]; then
        echo "Only one tree may be given" >&2
        exit 2
      fi
      TREE="$1"
      shift
      ;;
  esac
done

if [ -z "${TREE}" ] || [ ! -d "${TREE}" ]; then
  echo "Usage: $0 [--prefix <path>] [--deployment-target <ver>] <tree>" >&2
  exit 2
fi

# Physical path, so symlink containment checks compare like with like.
TREE="$(cd "${TREE}" && pwd -P)" || exit 2
# Strip a trailing slash from the prefix so string prefix tests are exact.
PREFIX="${PREFIX%/}"

VIOLATIONS_FILE="$(mktemp "${TMPDIR:-/tmp}/artifact-gate.XXXXXX")" || exit 2
trap 'rm -f "${VIOLATIONS_FILE}"' EXIT

violation() { # class, path, detail
  printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "${VIOLATIONS_FILE}"
}

# Interpreters that exist and work on a clean macOS install. /usr/bin/python3
# is deliberately absent: it is a Command Line Tools stub that pops a GUI
# install dialog, not an interpreter.
system_interpreter_ok() { # command basename
  case "$1" in
    sh|bash|zsh|csh|tcsh|ksh|perl|osascript) return 0 ;;
    *) return 1 ;;
  esac
}

# Map an absolute path under the install prefix to its location in the tree.
# Prints the mapped path; returns 1 when the path is not under the prefix.
map_into_tree() { # absolute path
  if [ -n "${PREFIX}" ]; then
    case "$1" in
      "${PREFIX}"/*) printf '%s/%s\n' "${TREE}" "${1#"${PREFIX}"/}"; return 0 ;;
    esac
  fi
  return 1
}

# Numeric major.minor comparison; succeeds when $1 <= $2.
version_le() {
  local a_major a_minor b_major b_minor
  a_major="${1%%.*}"; a_minor="$(echo "$1." | cut -d. -f2)"
  b_major="${2%%.*}"; b_minor="$(echo "$2." | cut -d. -f2)"
  [ "${a_major:-0}" -lt "${b_major:-0}" ] && return 0
  [ "${a_major:-0}" -gt "${b_major:-0}" ] && return 1
  [ "${a_minor:-0}" -le "${b_minor:-0}" ]
}

check_macho() { # file
  local f="$1" archs entry minos rpath load_cmds mapped

  archs="$(lipo -archs "${f}" 2>/dev/null)"
  if [ "${archs}" != "arm64" ]; then
    violation arch "${f}" "architectures: ${archs:-unreadable} (want exactly arm64)"
  fi

  load_cmds="$(otool -l "${f}" 2>/dev/null)"

  # Dependent libraries: LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB / LC_REEXPORT_DYLIB.
  # The name lines look like:  name /usr/lib/libSystem.B.dylib (offset 24)
  for entry in $(printf '%s\n' "${load_cmds}" \
                 | awk '/cmd LC_(LOAD_DYLIB|LOAD_WEAK_DYLIB|REEXPORT_DYLIB)/ {want=1}
                        want && $1 == "name" {print $2; want=0}'); do
    case "${entry}" in
      /usr/lib/*|/System/Library/*)
        : ;;
      @rpath/*|@loader_path/*|@executable_path/*)
        violation loads "${f}" "@-relative load entry: ${entry}"
        ;;
      *)
        if mapped="$(map_into_tree "${entry}")" && [ -e "${mapped}" ]; then
          : # payload-internal absolute install name
        else
          violation loads "${f}" "external load entry: ${entry}"
        fi
        ;;
    esac
  done

  # Run-path search paths must stay inside the payload.
  for rpath in $(printf '%s\n' "${load_cmds}" \
                 | awk '/cmd LC_RPATH/ {want=1}
                        want && $1 == "path" {print $2; want=0}'); do
    if mapped="$(map_into_tree "${rpath}")" && [ -d "${mapped}" ]; then
      :
    else
      violation rpath "${f}" "LC_RPATH outside payload: ${rpath}"
    fi
  done

  # Deployment target: LC_BUILD_VERSION carries minos; older binaries carry
  # LC_VERSION_MIN_MACOSX with a version field. Missing both fails closed.
  minos="$(printf '%s\n' "${load_cmds}" \
           | awk '/cmd LC_BUILD_VERSION/ {want=1}
                  want && $1 == "minos" {print $2; exit}')"
  if [ -z "${minos}" ]; then
    minos="$(printf '%s\n' "${load_cmds}" \
             | awk '/cmd LC_VERSION_MIN_MACOSX/ {want=1}
                    want && $1 == "version" {print $2; exit}')"
  fi
  if [ -z "${minos}" ]; then
    violation minos "${f}" "no LC_BUILD_VERSION or LC_VERSION_MIN_MACOSX recorded"
  elif ! version_le "${minos}" "${DEPLOYMENT_TARGET}"; then
    violation minos "${f}" "targets macOS ${minos} (floor is ${DEPLOYMENT_TARGET})"
  fi
}

check_shebang() { # file
  local f="$1" line interp arg1 mapped

  line="$(head -c 512 "${f}" | head -n 1)"
  case "${line}" in
    '#!'*) : ;;
    *) return 0 ;; # executable but not a script; not this check's business
  esac

  # "#!/path/to/interp arg" -> interp + first arg
  line="${line#\#!}"
  # shellcheck disable=SC2086
  set -- ${line}
  interp="${1:-}"
  arg1="${2:-}"

  case "${interp}" in
    /usr/bin/env)
      if ! system_interpreter_ok "${arg1}"; then
        violation shebang "${f}" "interpreter not on a clean macOS: /usr/bin/env ${arg1}"
      fi
      ;;
    *)
      if mapped="$(map_into_tree "${interp}")" && [ -f "${mapped}" ]; then
        : # payload-internal interpreter
      elif system_interpreter_ok "$(basename "${interp}")" && [ -x "${interp}" ] \
           && case "${interp}" in /bin/*|/usr/bin/*) true ;; *) false ;; esac; then
        : # OS-provided interpreter
      else
        violation shebang "${f}" "interpreter not on a clean macOS: ${interp}"
      fi
      ;;
  esac
}

check_symlink() { # link
  local l="$1" target resolved mapped

  target="$(readlink "${l}")"

  case "${target}" in
    /*)
      if mapped="$(map_into_tree "${target}")" && [ -e "${mapped}" ]; then
        return 0
      fi
      violation symlink "${l}" "absolute target outside payload: ${target}"
      ;;
    *)
      resolved="$(cd "$(dirname "${l}")" 2>/dev/null && cd "$(dirname "${target}")" 2>/dev/null && pwd -P)"
      if [ -z "${resolved}" ] || [ ! -e "${resolved}/$(basename "${target}")" ]; then
        violation symlink "${l}" "broken target: ${target}"
        return 0
      fi
      case "${resolved}/" in
        "${TREE}"/*) : ;;
        *) violation symlink "${l}" "target escapes payload: ${target}" ;;
      esac
      ;;
  esac
}

scanned_machos=0
scanned_scripts=0
scanned_symlinks=0

while IFS= read -r f; do
  if [ -L "${f}" ]; then
    scanned_symlinks=$((scanned_symlinks + 1))
    check_symlink "${f}"
    continue
  fi
  [ -f "${f}" ] || continue

  case "$(file -b "${f}")" in
    Mach-O*)
      scanned_machos=$((scanned_machos + 1))
      check_macho "${f}"
      ;;
    *)
      if [ -x "${f}" ]; then
        scanned_scripts=$((scanned_scripts + 1))
        check_shebang "${f}"
      fi
      ;;
  esac
done < <(find "${TREE}" \( -type f -o -type l \) | sort)

total=0
echo "artifact-gate: scanned ${scanned_machos} Mach-O files, ${scanned_scripts} executables, ${scanned_symlinks} symlinks under ${TREE}"
for class in loads arch minos rpath shebang symlink; do
  count="$(awk -F'\t' -v c="${class}" '$1 == c' "${VIOLATIONS_FILE}" | wc -l | tr -d ' ')"
  total=$((total + count))
  if [ "${count}" -gt 0 ]; then
    echo ""
    echo "== ${class}: ${count} violation(s)"
    awk -F'\t' -v c="${class}" '$1 == c {printf "   %s: %s\n", $2, $3}' "${VIOLATIONS_FILE}"
  fi
done

echo ""
if [ "${total}" -gt 0 ]; then
  echo "artifact-gate: FAIL - ${total} violation(s)"
  exit 1
fi
echo "artifact-gate: PASS"
exit 0
