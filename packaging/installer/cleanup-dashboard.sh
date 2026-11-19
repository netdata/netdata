#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Compare against the extracted release after its files have been installed.
set -eu
unset CDPATH
incoming=$1
prefix=$2
web_dest=${3%/}
case "$prefix" in /*) ;; *) prefix="$PWD/$prefix" ;; esac
case "/$web_dest/" in
    *'//'* | *'/../'* | *'/./'*) echo >&2 "Skipping dashboard cleanup: unsafe install destination"; exit 0 ;;
esac

[ -f "$incoming/index.html" ] && [ ! -L "$incoming/index.html" ] || exit 0
cd -P "$incoming"
incoming=$PWD
cd -P "$prefix"
prefix=$PWD
cd -P "./$web_dest"
# A relocated web directory may contain unrelated files. Check the physical
# location after chdir, and perform all deletions relative to the pinned cwd.
if [ "$PWD" != "${prefix%/}/$web_dest" ]; then
    echo >&2 "Skipping dashboard cleanup: redirected web directory"
    exit 0
fi

# mountinfo escapes whitespace and backslashes in paths. Keep only mounts
# below the prefix. A double slash separates complete, normalized paths.
mounts=//
platform=$(uname -s)
if [ "$platform" = Linux ]; then
    if [ ! -r /proc/self/mountinfo ]; then
        echo >&2 "Skipping dashboard cleanup: cannot read mount table"
        exit 0
    fi
    while read -r _ _ _ _ mount _; do
        case "$mount" in
            *\\*) mount=$(printf '%b/' "$mount"); mount=${mount%/} ;;
        esac
        case "$mount" in / | "$prefix") continue ;; esac
        case "$mount" in "${prefix%/}/"*) ;; *) continue ;; esac
        case "$PWD/" in "$mount/"*)
            echo >&2 "Skipping dashboard cleanup: mounted web directory"
            exit 0 ;;
        esac
        case "$mount/" in "$PWD/v3/"*) mounts="${mounts}${mount#/}//" ;; esac
    done < /proc/self/mountinfo
else
    web_device=$(stat -f %d .) || exit 1
    prefix_device=$(stat -f %d "$prefix") || exit 1
    [ "$web_device" = "$prefix_device" ] || exit 0
fi

remove_files() {
    printf >&2 'Dashboard cleanup: rm -f (%s stale files)\n' "$#"
    if rm -f "$@"; then return 0; else
        status=$?
        printf >&2 'Dashboard cleanup: rm failed in %s with status %d\n' "$PWD" "$status"
        return "$status"
    fi
}

# Enter each directory in a subshell. rm receives only immediate children of
# that working directory, never an absolute path or a path read from a listing.
clean_dir() (
    expected="$PWD/${1#./}"
    case "$mounts" in *"//${expected#/}//"*) exit 0 ;; esac
    [ ! -L "$1" ] || exit 0
    cd -P "$1" || exit 1
    [ "$PWD" = "$expected" ] || exit 0
    if [ "$platform" != Linux ]; then
        here=$(stat -f %d .) || exit 1
        [ "$here" = "$web_device" ] || exit 0
    fi
    source_dir=$2
    if [ -L "$source_dir" ]; then exit 0; fi
    if [ -d "$source_dir" ] && { [ ! -r "$source_dir" ] || [ ! -x "$source_dir" ]; }; then
        echo >&2 "Cannot read incoming dashboard directory: $source_dir"
        exit 1
    fi

    set --
    for file in ./* ./.[!.]* ./..?*; do
        case "$mounts" in *"//${PWD#/}/${file#./}//"*) continue ;; esac
        [ ! -L "$file" ] || continue
        source_file="$source_dir/${file#./}"
        if [ -d "$file" ]; then
            clean_dir "$file" "$source_file" || exit $?
        elif [ -f "$file" ] && [ ! -e "$source_file" ] && [ ! -L "$source_file" ]; then
            set -- "$@" "$file"
            if [ "$#" -eq 128 ]; then
                remove_files "$@" || exit $?
                set --
            fi
        fi
    done
    if [ "$#" -gt 0 ]; then remove_files "$@"; fi
)

[ -d ./v3 ] && [ ! -L ./v3 ] || exit 0
# A build directory or a custom static TMPDIR can be placed inside the web tree.
case "$incoming/" in "$PWD/v3/"*) echo >&2 "Skipping dashboard cleanup: overlapping source"; exit 0 ;; esac
case "$PWD/v3/" in "$incoming/"*) echo >&2 "Skipping dashboard cleanup: overlapping destination"; exit 0 ;; esac
clean_dir ./v3 "$incoming"
