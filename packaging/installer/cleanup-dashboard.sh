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

# Linux bind mounts can share a device number with their parent.
mount_id() {
    if [ "$platform" = Linux ]; then
        while read -r key value; do
            if [ "$key" = mnt_id: ]; then printf '%s\n' "$value"; return 0; fi
        done < /proc/self/fdinfo/3
        return 1
    fi
    stat -f %d .
}

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
    [ ! -L "$1" ] || exit 0
    cd -P "$1" || exit 1
    [ "$PWD" = "$expected" ] || exit 0
    here=$(mount_id 3<.) || exit 1
    [ "$here" = "$web_mount" ] || exit 0
    source_dir=$2
    if [ -L "$source_dir" ]; then exit 0; fi
    if [ -d "$source_dir" ] && { [ ! -r "$source_dir" ] || [ ! -x "$source_dir" ]; }; then
        echo >&2 "Cannot read incoming dashboard directory: $source_dir"
        exit 1
    fi

    set --
    for file in ./* ./.[!.]* ./..?*; do
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
platform=$(uname -s)
if ! web_mount=$(mount_id 3<.); then
    echo >&2 "Skipping dashboard cleanup: cannot identify the installed filesystem"
    exit 0
fi
clean_dir ./v3 "$incoming"
